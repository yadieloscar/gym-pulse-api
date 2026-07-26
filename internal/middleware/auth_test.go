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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestJWKSUnknownKidUsesNegativeCache(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: middlewareRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"keys":[]}`))}, nil
	})}
	cache := &keyCache{keys: map[string]cachedKey{}, missing: map[string]time.Time{}, client: client, now: time.Now}
	for range 2 {
		if _, err := cache.getPublicKey(context.Background(), "https://jwks.example.test", "unknown"); err == nil {
			t.Fatal("unknown kid was accepted")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("JWKS fetch count = %d, want 1", calls.Load())
	}
}

func TestJWKSCacheRefreshesAtSupabaseEdgeTTL(t *testing.T) {
	var calls atomic.Int32
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	body := `{"keys":[{"kty":"RSA","kid":"current","n":"AQ","e":"Aw","alg":"RS256"}]}`
	client := &http.Client{Transport: middlewareRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	cache := &keyCache{
		keys:    map[string]cachedKey{},
		missing: map[string]time.Time{},
		client:  client,
		now:     func() time.Time { return now },
	}

	if _, err := cache.getPublicKey(context.Background(), "https://jwks.example.test", "current"); err != nil {
		t.Fatalf("initial JWKS fetch: %v", err)
	}
	now = now.Add(jwksCacheTTL - time.Second)
	if _, err := cache.getPublicKey(context.Background(), "https://jwks.example.test", "current"); err != nil {
		t.Fatalf("cached JWKS lookup: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("fresh JWKS fetch count = %d, want 1", calls.Load())
	}

	now = now.Add(time.Second)
	if _, err := cache.getPublicKey(context.Background(), "https://jwks.example.test", "current"); err != nil {
		t.Fatalf("expired JWKS refresh: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expired JWKS fetch count = %d, want 2", calls.Load())
	}
}

func TestAuthMiddlewareValidatesIssuerAudienceAndUUIDSubject(t *testing.T) {
	secret := "test-secret"
	handler := AuthMiddlewareWithConfig(AuthConfig{JWTSecret: secret, Issuer: "https://issuer.example/auth/v1", Audience: "authenticated"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	sign := func(claims jwt.MapClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		value, _ := token.SignedString([]byte(secret))
		return value
	}
	base := jwt.MapClaims{"sub": uuid.NewString(), "exp": time.Now().Add(time.Hour).Unix(), "iss": "https://issuer.example/auth/v1", "aud": "authenticated"}
	request := func(claims jwt.MapClaims) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+sign(claims))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := request(base); got != http.StatusOK {
		t.Fatalf("valid claims status = %d", got)
	}
	badSubject := base
	badSubject["sub"] = "not-a-uuid"
	if got := request(badSubject); got != http.StatusUnauthorized {
		t.Fatalf("invalid sub status = %d", got)
	}
	badIssuer := jwt.MapClaims{"sub": uuid.NewString(), "exp": time.Now().Add(time.Hour).Unix(), "iss": "https://wrong.example", "aud": "authenticated"}
	if got := request(badIssuer); got != http.StatusUnauthorized {
		t.Fatalf("invalid issuer status = %d", got)
	}
}

type middlewareRoundTripFunc func(*http.Request) (*http.Response, error)

func (f middlewareRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func installHTTPResponse(t *testing.T, status int, body string) {
	t.Helper()
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: middlewareRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = previous })
}

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
				Crv: "P-256",
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

	installHTTPResponse(t, http.StatusOK, string(jwksJSON))

	userID := uuid.New().String()
	middleware := AuthMiddleware("", "https://jwks.example.test")
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

	t.Run("RSA key rejects RS512 token", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodRS512, jwt.MapClaims{"sub": userID, "exp": time.Now().Add(time.Hour).Unix()})
		token.Header["kid"] = "rsa-key-id"
		tokenStr, _ := token.SignedString(rsaPrivKey)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
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
		installHTTPResponse(t, http.StatusInternalServerError, "")
		errMiddleware := AuthMiddleware("", "https://jwks-error.example.test")
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
		installHTTPResponse(t, http.StatusOK, "invalid json")
		errMiddleware := AuthMiddleware("", "https://bad-jwks.example.test")
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
