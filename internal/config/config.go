// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

var (
	ErrMissingDatabaseURL = errors.New("DATABASE_URL is required")
	ErrMissingJWTConfig   = errors.New("SUPABASE_JWT_SECRET or SUPABASE_JWKS_URL is required")
	ErrProductionAuth     = errors.New("production requires SUPABASE_JWKS_URL, SUPABASE_JWT_ISSUER, and SUPABASE_JWT_AUDIENCE")
)

// Config holds the application configuration.
type Config struct {
	Port                string
	DatabaseURL         string
	SupabaseJWTSecret   string
	SupabaseJWKSURL     string
	SupabaseJWTIssuer   string
	SupabaseJWTAudience string
	// Optional: enables deleting the Supabase auth user on account deletion.
	// Without them, DELETE /account removes application data only.
	SupabaseURL            string
	SupabaseServiceRoleKey string
	AllowedOrigins         []string
	Environment            string
	LogLevel               string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, ErrMissingDatabaseURL
	}

	jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
	jwksURL := os.Getenv("SUPABASE_JWKS_URL")
	if jwtSecret == "" && jwksURL == "" {
		return nil, ErrMissingJWTConfig
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

	issuer, audience := os.Getenv("SUPABASE_JWT_ISSUER"), os.Getenv("SUPABASE_JWT_AUDIENCE")
	if env == "production" {
		jwksParsed, jwksErr := url.ParseRequestURI(jwksURL)
		issuerParsed, issuerErr := url.ParseRequestURI(issuer)
		if jwksURL == "" || issuer == "" || audience == "" || jwksErr != nil || issuerErr != nil || jwksParsed.Scheme != "https" || issuerParsed.Scheme != "https" || jwksParsed.Host == "" || issuerParsed.Host == "" {
			return nil, ErrProductionAuth
		}
	}

	return &Config{
		Port:                   port,
		DatabaseURL:            dbURL,
		SupabaseJWTSecret:      jwtSecret,
		SupabaseJWKSURL:        jwksURL,
		SupabaseJWTIssuer:      issuer,
		SupabaseJWTAudience:    audience,
		SupabaseURL:            os.Getenv("SUPABASE_URL"),
		SupabaseServiceRoleKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		AllowedOrigins:         origins,
		Environment:            env,
		LogLevel:               logLevel,
	}, nil
}
