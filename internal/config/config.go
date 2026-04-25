// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strings"
)

var (
	ErrMissingDatabaseURL = errors.New("DATABASE_URL is required")
	ErrMissingJWTSecret   = errors.New("SUPABASE_JWT_SECRET is required")
)

// Config holds the application configuration.
type Config struct {
	Port              string
	DatabaseURL       string
	SupabaseJWTSecret string
	AllowedOrigins    []string
	Environment       string
	LogLevel          string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, ErrMissingDatabaseURL
	}

	jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
	if jwtSecret == "" {
		return nil, ErrMissingJWTSecret
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	var origins []string
	if raw := os.Getenv("ALLOWED_ORIGINS"); raw != "" {
		for o := range strings.SplitSeq(raw, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}

	return &Config{
		Port:              port,
		DatabaseURL:       dbURL,
		SupabaseJWTSecret: jwtSecret,
		AllowedOrigins:    origins,
		Environment:       env,
		LogLevel:          logLevel,
	}, nil
}
