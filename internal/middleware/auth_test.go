package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestAuthMiddleware_Symmetric(t *testing.T) {
	secret := "test-secret-key"
	userID := uuid.New().String()

	// Helper to generate tokens
	generateToken := func(sub string, expiresAt time.Time) string {
		claims := jwt.MapClaims{
			"sub": sub,
			"exp": expiresAt.Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		str, _ := token.SignedString([]byte(secret))
		return str
	}

	// Create a simple handler that verifies context injection
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := MustGetUserID(r.Context())
		if uid.String() != userID {
			t.Errorf("expected user ID %s in context, got %s", userID, uid)
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := AuthMiddleware(secret, "")
	handler := middleware(nextHandler)

	t.Run("missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}

		var resp map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["error"] != "missing authorization header" {
			t.Errorf("expected error message 'missing authorization header', got '%s'", resp["error"])
		}
	})

	t.Run("invalid authorization format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Basic credentials")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("invalid token signature", func(t *testing.T) {
		claims := jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		badToken, _ := token.SignedString([]byte("wrong-secret"))

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+badToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		tokenStr := generateToken(userID, time.Now().Add(-time.Hour))
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("missing user id in token", func(t *testing.T) {
		tokenStr := generateToken("", time.Now().Add(time.Hour))
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		tokenStr := generateToken(userID, time.Now().Add(time.Hour))
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})
}

func TestAuthMiddleware_AsymmetricJWKS(t *testing.T) {
	// Generate keys for signature testing
	rsaPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	ecPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate EC key: %v", err)
	}

	// Prepare RSA key params
	rsaNStr := base64.RawURLEncoding.EncodeToString(rsaPrivKey.N.Bytes())
	rsaEBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(rsaEBytes, uint32(rsaPrivKey.E))
	for len(rsaEBytes) > 0 && rsaEBytes[0] == 0 {
		rsaEBytes = rsaEBytes[1:]
	}
	rsaEStr := base64.RawURLEncoding.EncodeToString(rsaEBytes)

	// Prepare EC key params
	ecXStr := base64.RawURLEncoding.EncodeToString(ecPrivKey.X.Bytes())
	ecYStr := base64.RawURLEncoding.EncodeToString(ecPrivKey.Y.Bytes())

	jwks := JWKS{
		Keys: []JWKSKey{
			{
				Kty: "RSA",
				Kid: "rsa-key-id",
				N:   rsaNStr,
				E:   rsaEStr,
				Alg: "RS256",
			},
			{
				Kty: "EC",
				Kid: "ec-key-id",
				X:   ecXStr,
				Y:   ecYStr,
				Alg: "ES256",
			},
		},
	}

	jwksJSON, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("failed to marshal JWKS: %v", err)
	}

	// Start local JWKS server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	userID := uuid.New().String()
	middleware := AuthMiddleware("", srv.URL)
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := MustGetUserID(r.Context())
		if uid.String() != userID {
			t.Errorf("expected user ID %s, got %s", userID, uid)
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware(nextHandler)

	t.Run("RSA RS256 token validation success", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "rsa-key-id"
		tokenStr, err := token.SignedString(rsaPrivKey)
		if err != nil {
			t.Fatalf("failed to sign RSA token: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("EC ES256 token validation success", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "ec-key-id"
		tokenStr, err := token.SignedString(ecPrivKey)
		if err != nil {
			t.Fatalf("failed to sign EC token: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("JWKS missing kid header", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		tokenStr, _ := token.SignedString(rsaPrivKey)

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("JWKS invalid signing method alg", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "rsa-key-id"
		tokenStr, _ := token.SignedString([]byte("some-secret"))

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("JWKS key not found", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "unknown-key-id"
		tokenStr, _ := token.SignedString(rsaPrivKey)

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("JWKS caching works", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "rsa-key-id"
		tokenStr, _ := token.SignedString(rsaPrivKey)

		// Call 1: fetches and caches
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}

		// Call 2: hits cache
		req2 := httptest.NewRequest("GET", "/api/v1/test", nil)
		req2.Header.Set("Authorization", "Bearer "+tokenStr)
		rec2 := httptest.NewRecorder()
		handler.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec2.Code)
		}
	})

	t.Run("JWKS server status error", func(t *testing.T) {
		errMiddleware := AuthMiddleware("", srv.URL+"/non-existent")
		errHandler := errMiddleware(nextHandler)

		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "bad-key-id"
		tokenStr, _ := token.SignedString(rsaPrivKey)

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		errHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("JWKS fetch error (invalid URL)", func(t *testing.T) {
		errMiddleware := AuthMiddleware("", "http://invalid-domain-name-that-does-not-exist.local")
		errHandler := errMiddleware(nextHandler)

		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "bad-key-id"
		tokenStr, _ := token.SignedString(rsaPrivKey)

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		errHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("JWKS decode error (invalid json)", func(t *testing.T) {
		badJSONSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("invalid json"))
		}))
		defer badJSONSrv.Close()

		errMiddleware := AuthMiddleware("", badJSONSrv.URL)
		errHandler := errMiddleware(nextHandler)

		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": userID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "bad-key-id"
		tokenStr, _ := token.SignedString(rsaPrivKey)

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		errHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})
}

func TestMustGetUserID_Panic(t *testing.T) {
	t.Run("panic when not in context", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic, got nil")
			}
		}()
		MustGetUserID(context.Background())
	})

	t.Run("panic when invalid UUID format", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic, got nil")
			}
		}()
		ctx := context.WithValue(context.Background(), UserIDKey, "invalid-uuid")
		MustGetUserID(ctx)
	})
}
