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
	"io"
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

type cachedKey struct {
	key any
	alg string
}

type AuthConfig struct {
	JWTSecret  string
	JWKSURL    string
	Issuer     string
	Audience   string
	HTTPClient *http.Client
}

type keyCache struct {
	keys        map[string]cachedKey
	missing     map[string]time.Time
	lastFetch   time.Time
	lastAttempt time.Time
	client      *http.Client
	now         func() time.Time
	mutex       sync.RWMutex
}

const (
	jwksCacheTTL        = time.Hour
	jwksRefreshCooldown = time.Minute
	jwksHTTPTimeout     = 5 * time.Second
	jwksMaxBodyBytes    = 1 << 20
)

func (c *keyCache) getPublicKey(ctx context.Context, jwksURL string, kid string) (cachedKey, error) {
	// Try read lock first
	c.mutex.RLock()
	key, ok := c.keys[kid]
	now := c.now()
	isFresh := now.Sub(c.lastFetch) < jwksCacheTTL
	missingUntil := c.missing[kid]
	c.mutex.RUnlock()

	if ok && isFresh {
		return validateCachedAlgorithm(key, kid)
	}
	if !missingUntil.IsZero() && now.Before(missingUntil) {
		return cachedKey{}, fmt.Errorf("%w: %s", errJWKSKeyNotFound, kid)
	}

	// Write lock for fetch
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Double check cache
	now = c.now()
	if key, ok = c.keys[kid]; ok && now.Sub(c.lastFetch) < jwksCacheTTL {
		return validateCachedAlgorithm(key, kid)
	}
	if until := c.missing[kid]; !until.IsZero() && now.Before(until) {
		return cachedKey{}, fmt.Errorf("%w: %s", errJWKSKeyNotFound, kid)
	}
	if !c.lastAttempt.IsZero() && now.Sub(c.lastAttempt) < jwksRefreshCooldown {
		c.missing[kid] = c.lastAttempt.Add(jwksRefreshCooldown)
		return cachedKey{}, fmt.Errorf("%w: %s", errJWKSKeyNotFound, kid)
	}
	c.lastAttempt = now

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return cachedKey{}, fmt.Errorf("building JWKS request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return cachedKey{}, fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return cachedKey{}, fmt.Errorf("%w: %d", errJWKSStatus, resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(io.LimitReader(resp.Body, jwksMaxBodyBytes)).Decode(&jwks); err != nil {
		return cachedKey{}, fmt.Errorf("decoding JWKS: %w", err)
	}

	newKeys := make(map[string]cachedKey)
	for _, k := range jwks.Keys {
		var pubKey any
		switch k.Kty {
		case "EC":
			if k.Alg != "ES256" || k.Crv != "P-256" {
				continue
			}
			xVal, err := decodeCoordinate(k.X)
			if err != nil {
				continue
			}
			yVal, err := decodeCoordinate(k.Y)
			if err != nil {
				continue
			}
			encoded := make([]byte, 65)
			encoded[0] = 4
			xVal.FillBytes(encoded[1:33])
			yVal.FillBytes(encoded[33:])
			parsed, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), encoded)
			if err != nil {
				continue
			}
			pubKey = parsed
		case "RSA":
			if k.Alg != "RS256" {
				continue
			}
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
			if eVal < 3 || len(nBytes) == 0 {
				continue
			}
			pubKey = &rsa.PublicKey{
				N: new(big.Int).SetBytes(nBytes),
				E: eVal,
			}
		default:
			continue
		}
		if k.Kid == "" {
			continue
		}
		newKeys[k.Kid] = cachedKey{key: pubKey, alg: k.Alg}
	}

	c.keys = newKeys
	c.lastFetch = now
	c.missing = make(map[string]time.Time)

	key, ok = c.keys[kid]
	if !ok {
		c.missing[kid] = now.Add(jwksRefreshCooldown)
		return cachedKey{}, fmt.Errorf("%w: %s", errJWKSKeyNotFound, kid)
	}
	return validateCachedAlgorithm(key, kid)
}

func validateCachedAlgorithm(key cachedKey, kid string) (cachedKey, error) {
	if key.key == nil || key.alg == "" {
		return cachedKey{}, fmt.Errorf("%w: %s", errJWKSKeyNotFound, kid)
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
	return AuthMiddlewareWithConfig(AuthConfig{JWTSecret: jwtSecret, JWKSURL: jwksURL})
}

func AuthMiddlewareWithConfig(cfg AuthConfig) func(http.Handler) http.Handler {
	baseClient := cfg.HTTPClient
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	clone := *baseClient
	client := &clone
	if client.Timeout == 0 {
		client.Timeout = jwksHTTPTimeout
	}
	jwksCache := &keyCache{keys: make(map[string]cachedKey), missing: make(map[string]time.Time), client: client, now: time.Now}
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
			options := []jwt.ParserOption{jwt.WithExpirationRequired()}
			if cfg.Issuer != "" {
				options = append(options, jwt.WithIssuer(cfg.Issuer))
			}
			if cfg.Audience != "" {
				options = append(options, jwt.WithAudience(cfg.Audience))
			}
			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) { //nolint:contextcheck
				// 1. Asymmetric JWKS validation if configured
				if cfg.JWKSURL != "" {
					kid, ok := t.Header["kid"].(string)
					if !ok || kid == "" {
						return nil, errMissingKidHeader
					}

					alg, _ := t.Header["alg"].(string)
					if alg != "RS256" && alg != "ES256" {
						return nil, fmt.Errorf("%w: %v", errUnexpectedSigningAlg, t.Header["alg"])
					}
					cached, err := jwksCache.getPublicKey(r.Context(), cfg.JWKSURL, kid)
					if err != nil {
						return nil, err
					}
					key := cached
					if key.alg != alg {
						return nil, fmt.Errorf("%w: token %s, key %s", errUnexpectedSigningAlg, alg, key.alg)
					}
					if (alg == "RS256" && t.Method != jwt.SigningMethodRS256) || (alg == "ES256" && t.Method != jwt.SigningMethodES256) {
						return nil, fmt.Errorf("%w: %s", errUnexpectedSigningAlg, alg)
					}
					return key.key, nil
				}

				// 2. Symmetric HMAC validation fallback
				if t.Method != jwt.SigningMethodHS256 {
					return nil, fmt.Errorf("%w: %v", errUnexpectedSignMethod, t.Header["alg"])
				}
				return []byte(cfg.JWTSecret), nil
			}, options...)
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
			if _, err := uuid.Parse(userID); err != nil {
				writeAuthError(w, "invalid user id in token")
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
