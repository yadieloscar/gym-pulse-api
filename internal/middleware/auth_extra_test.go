package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestAuthMiddleware_JWKS_MalformedAndUnsupportedKeys(t *testing.T) {
	// Mix in a key with kty=oct (skipped), an EC key with bad x coordinate
	// (decodeCoordinate fails -> continue), and an RSA key with bad N.
	jwks := JWKS{
		Keys: []JWKSKey{
			{Kty: "oct", Kid: "oct-key"},
			{Kty: "EC", Kid: "ec-bad-x", X: "!!!not-base64!!!", Y: "abc"},
			{Kty: "EC", Kid: "ec-bad-y", X: "abc", Y: "!!!not-base64!!!"},
			{Kty: "RSA", Kid: "rsa-bad-n", N: "!!!", E: "AQAB"},
			{Kty: "RSA", Kid: "rsa-bad-e", N: "abc", E: "!!!"},
		},
	}
	body, _ := json.Marshal(jwks)

	installHTTPResponse(t, http.StatusOK, string(body))

	mw := AuthMiddleware("", "https://malformed-jwks.example.test")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Token with kid referencing a key that exists in JWKS but was skipped
	// during parsing -> "key not found".
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": uuid.New().String(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "ec-bad-x"
	tokStr, err := tok.SignedString(rsaKey)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tokStr)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddlewareChallengeAndCodeOn401(t *testing.T) {
	secret := "test-secret-key"
	handler := AuthMiddleware(secret, "")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name          string
		authorization string
		wantChallenge string
	}{
		{name: "missing header", authorization: "", wantChallenge: `Bearer realm="gym-pulse"`},
		{name: "wrong scheme", authorization: "Basic abc", wantChallenge: `Bearer realm="gym-pulse"`},
		{name: "garbage token", authorization: "Bearer not-a-jwt", wantChallenge: `Bearer realm="gym-pulse", error="invalid_token"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != tc.wantChallenge {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, tc.wantChallenge)
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != "AUTHENTICATION_REQUIRED" {
				t.Fatalf("code = %q, want AUTHENTICATION_REQUIRED", body["code"])
			}
			if body["error"] == "" {
				t.Fatal("error message must not be empty")
			}
		})
	}
}

func TestAuthMiddlewareToleratesClockSkew(t *testing.T) {
	secret := "test-secret-key"
	sub := uuid.New().String()
	handler := AuthMiddleware(secret, "")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	sign := func(claims jwt.MapClaims) string {
		claims["sub"] = sub
		str, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return str
	}
	now := time.Now()

	cases := []struct {
		name   string
		claims jwt.MapClaims
		want   int
	}{
		{name: "expired inside leeway", claims: jwt.MapClaims{"exp": now.Add(-10 * time.Second).Unix()}, want: http.StatusOK},
		{name: "expired beyond leeway", claims: jwt.MapClaims{"exp": now.Add(-2 * time.Minute).Unix()}, want: http.StatusUnauthorized},
		{name: "not yet valid inside leeway", claims: jwt.MapClaims{"exp": now.Add(time.Hour).Unix(), "nbf": now.Add(10 * time.Second).Unix()}, want: http.StatusOK},
		{name: "not yet valid beyond leeway", claims: jwt.MapClaims{"exp": now.Add(time.Hour).Unix(), "nbf": now.Add(2 * time.Minute).Unix()}, want: http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			req.Header.Set("Authorization", "Bearer "+sign(tc.claims))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
