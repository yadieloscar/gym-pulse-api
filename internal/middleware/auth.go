// Package middleware provides HTTP middleware for the gym-pulse API.
package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

// UserIDKey is the context key for the authenticated user's ID.
const UserIDKey contextKey = "user_id"

var (
	errUnexpectedSignMethod = errors.New("unexpected signing method")
	errJWKSStatus           = errors.New("unexpected JWKS status code")
	errJWKSKeyNotFound      = errors.New("key not found in JWKS")
	errMissingKidHeader     = errors.New("missing kid header in token")
	errUnexpectedSigningAlg = errors.New("unexpected signing method alg")
)

type JWKSKey struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	N   string `json:"n"`
	E   string `json:"e"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
}

type JWKS struct {
	Keys []JWKSKey `json:"keys"`
}

type keyCache struct {
	keys      map[string]any
	lastFetch time.Time
	mutex     sync.RWMutex
}

func (c *keyCache) getPublicKey(ctx context.Context, jwksURL string, kid string) (any, error) {
	// Try read lock first
	c.mutex.RLock()
	key, ok := c.keys[kid]
	isFresh := time.Since(c.lastFetch) < 1*time.Hour
	c.mutex.RUnlock()

	if ok && isFresh {
		return key, nil
	}

	// Write lock for fetch
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Double check cache
	if key, ok = c.keys[kid]; ok && time.Since(c.lastFetch) < 1*time.Hour {
		return key, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building JWKS request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", errJWKSStatus, resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decoding JWKS: %w", err)
	}

	newKeys := make(map[string]any)
	for _, k := range jwks.Keys {
		var pubKey any
		switch k.Kty {
		case "EC":
			xVal, err := decodeCoordinate(k.X)
			if err != nil {
				continue
			}
			yVal, err := decodeCoordinate(k.Y)
			if err != nil {
				continue
			}
			pubKey = &ecdsa.PublicKey{
				Curve: elliptic.P256(),
				X:     xVal,
				Y:     yVal,
			}
		case "RSA":
			nBytes, err := decodeCoordinateBytes(k.N)
			if err != nil {
				continue
			}
			eBytes, err := decodeCoordinateBytes(k.E)
			if err != nil {
				continue
			}
			var eVal int
			if len(eBytes) < 4 {
				padded := make([]byte, 4)
				copy(padded[4-len(eBytes):], eBytes)
				eVal = int(binary.BigEndian.Uint32(padded))
			} else {
				eVal = int(binary.BigEndian.Uint32(eBytes))
			}
			pubKey = &rsa.PublicKey{
				N: new(big.Int).SetBytes(nBytes),
				E: eVal,
			}
		default:
			continue
		}
		newKeys[k.Kid] = pubKey
	}

	c.keys = newKeys
	c.lastFetch = time.Now()

	key, ok = c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errJWKSKeyNotFound, kid)
	}
	return key, nil
}

func decodeCoordinate(s string) (*big.Int, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(data), nil
}

func decodeCoordinateBytes(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// AuthMiddleware validates the Bearer JWT and injects the user ID into the request context.
func AuthMiddleware(jwtSecret string, jwksURL string) func(http.Handler) http.Handler {
	jwksCache := &keyCache{keys: make(map[string]any)}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAuthError(w, "missing authorization header")
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenStr == authHeader {
				writeAuthError(w, "invalid authorization format")
				return
			}

			// jwt.Parse's keyFunc signature doesn't accept context, so we
			// rely on the captured r.Context() below. contextcheck can't
			// see through the closure — silence the false positive.
			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) { //nolint:contextcheck
				// 1. Asymmetric JWKS validation if configured
				if jwksURL != "" {
					kid, ok := t.Header["kid"].(string)
					if !ok || kid == "" {
						return nil, errMissingKidHeader
					}

					switch t.Method.(type) {
					case *jwt.SigningMethodECDSA:
						// Expected alg ES256
					case *jwt.SigningMethodRSA:
						// Expected alg RS256
					default:
						return nil, fmt.Errorf("%w: %v", errUnexpectedSigningAlg, t.Header["alg"])
					}

					return jwksCache.getPublicKey(r.Context(), jwksURL, kid)
				}

				// 2. Symmetric HMAC validation fallback
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("%w: %v", errUnexpectedSignMethod, t.Header["alg"])
				}
				return []byte(jwtSecret), nil
			})
			if err != nil || !token.Valid {
				writeAuthError(w, "invalid or expired token")
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				writeAuthError(w, "invalid token claims")
				return
			}

			userID, ok := claims["sub"].(string)
			if !ok || userID == "" {
				writeAuthError(w, "missing user id in token")
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MustGetUserID extracts the authenticated user's UUID from the context.
// Panics if the value is absent — only call from handlers inside the AuthMiddleware group.
func MustGetUserID(ctx context.Context) uuid.UUID {
	userIDStr, _ := ctx.Value(UserIDKey).(string)
	id, err := uuid.Parse(userIDStr)
	if err != nil {
		panic("MustGetUserID called outside authenticated route: " + err.Error())
	}
	return id
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		http.Error(w, message, http.StatusUnauthorized)
	}
}
