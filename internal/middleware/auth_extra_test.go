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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	mw := AuthMiddleware("", srv.URL)
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
