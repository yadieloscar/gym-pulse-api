// Package middleware provides HTTP middleware for the gym-pulse API.
package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

// UserIDKey is the context key for the authenticated user's ID.
const UserIDKey contextKey = "user_id"

var (
	errUserIDNotInContext   = errors.New("user_id not found in context")
	errUnexpectedSignMethod = errors.New("unexpected signing method")
)

// AuthMiddleware validates the Bearer JWT and injects the user ID into the request context.
func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
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

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
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

// GetUserID extracts the user ID from the request context.
func GetUserID(ctx context.Context) (uuid.UUID, error) {
	userIDStr, ok := ctx.Value(UserIDKey).(string)
	if !ok {
		return uuid.Nil, errUserIDNotInContext
	}
	return uuid.Parse(userIDStr)
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		http.Error(w, message, http.StatusUnauthorized)
	}
}
