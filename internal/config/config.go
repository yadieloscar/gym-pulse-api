// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMissingDatabaseURL      = errors.New("DATABASE_URL is required")
	ErrMissingJWTConfig        = errors.New("SUPABASE_JWT_SECRET or SUPABASE_JWKS_URL is required")
	ErrProductionAuth          = errors.New("staging and production require SUPABASE_JWT_AUDIENCE")
	ErrProductionAdmin         = errors.New("staging and production require a canonical HTTPS SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY")
	ErrSupabaseProjectMismatch = errors.New("supabase issuer and JWKS URLs must match SUPABASE_URL")
	ErrProductionDatabaseLock  = errors.New("staging and production require a direct or session-mode DATABASE_LOCK_URL")
	ErrProductionCORS          = errors.New("staging and production require explicit HTTPS ALLOWED_ORIGINS")
	ErrInvalidEnvironment      = errors.New("ENVIRONMENT is required and must be development, test, staging, or production")
	ErrInvalidDatabasePool     = errors.New("invalid database pool configuration")
)

// Config holds the application configuration.
type Config struct {
	Port                string
	DatabaseURL         string
	DatabaseLockURL     string
	SupabaseJWTSecret   string
	SupabaseJWKSURL     string
	SupabaseJWTIssuer   string
	SupabaseJWTAudience string
	// Optional in development/test. Staging and production require these so
	// account deletion can remove the auth identity and avatar object.
	SupabaseURL               string
	SupabaseServiceRoleKey    string
	SupabaseAvatarBucket      string
	AllowedOrigins            []string
	Environment               string
	LogLevel                  string
	DatabaseMaxConns          int32
	DatabaseMinConns          int32
	DatabaseLockMaxConns      int32
	DatabaseMaxConnLifetime   time.Duration
	DatabaseMaxConnIdleTime   time.Duration
	DatabaseHealthCheckPeriod time.Duration
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, ErrMissingDatabaseURL
	}

	jwtSecret := strings.TrimSpace(os.Getenv("SUPABASE_JWT_SECRET"))
	jwksURL := strings.TrimSpace(os.Getenv("SUPABASE_JWKS_URL"))
	issuer := strings.TrimSpace(os.Getenv("SUPABASE_JWT_ISSUER"))
	audience := strings.TrimSpace(os.Getenv("SUPABASE_JWT_AUDIENCE"))
	supabaseURL := strings.TrimSpace(os.Getenv("SUPABASE_URL"))
	serviceRoleKey := strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_ROLE_KEY"))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT")))
	if env == "" {
		return nil, ErrInvalidEnvironment
	}
	switch env {
	case "development", "test", "staging", "production":
	default:
		return nil, ErrInvalidEnvironment
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

	isReleaseEnvironment := env == "staging" || env == "production"
	lockURL := strings.TrimSpace(os.Getenv("DATABASE_LOCK_URL"))
	if isReleaseEnvironment {
		canonicalSupabaseURL, err := canonicalHTTPSBaseURL(supabaseURL)
		if err != nil || serviceRoleKey == "" {
			return nil, ErrProductionAdmin
		}
		supabaseURL = canonicalSupabaseURL

		expectedIssuer := supabaseURL + "/auth/v1"
		expectedJWKSURL := expectedIssuer + "/.well-known/jwks.json"
		if (issuer != "" && strings.TrimRight(issuer, "/") != expectedIssuer) ||
			(jwksURL != "" && strings.TrimRight(jwksURL, "/") != expectedJWKSURL) {
			return nil, ErrSupabaseProjectMismatch
		}
		issuer = expectedIssuer
		jwksURL = expectedJWKSURL
		if audience == "" {
			return nil, ErrProductionAuth
		}

		if err := validateProductionLockURL(lockURL); err != nil {
			return nil, err
		}

		if len(origins) == 0 {
			return nil, ErrProductionCORS
		}
		for _, origin := range origins {
			parsed, parseErr := url.ParseRequestURI(origin)
			if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
				return nil, ErrProductionCORS
			}
		}
	} else {
		if jwtSecret == "" && jwksURL == "" {
			return nil, ErrMissingJWTConfig
		}
		if lockURL == "" {
			lockURL = dbURL
		}
	}

	avatarBucket := strings.TrimSpace(os.Getenv("SUPABASE_AVATAR_BUCKET"))
	if avatarBucket == "" {
		avatarBucket = "avatars"
	}

	maxConns, err := positiveInt32Env("DATABASE_MAX_CONNS", 10)
	if err != nil {
		return nil, err
	}
	minConns, err := nonNegativeInt32Env("DATABASE_MIN_CONNS", 1)
	if err != nil {
		return nil, err
	}
	if minConns > maxConns {
		return nil, fmt.Errorf("%w: DATABASE_MIN_CONNS must not exceed DATABASE_MAX_CONNS", ErrInvalidDatabasePool)
	}
	lockMaxConns, err := positiveInt32Env("DATABASE_LOCK_MAX_CONNS", 4)
	if err != nil {
		return nil, err
	}
	maxConnLifetime, err := positiveDurationEnv("DATABASE_MAX_CONN_LIFETIME", 30*time.Minute)
	if err != nil {
		return nil, err
	}
	maxConnIdleTime, err := positiveDurationEnv("DATABASE_MAX_CONN_IDLE_TIME", 5*time.Minute)
	if err != nil {
		return nil, err
	}
	healthCheckPeriod, err := positiveDurationEnv("DATABASE_HEALTH_CHECK_PERIOD", time.Minute)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:                      port,
		DatabaseURL:               dbURL,
		DatabaseLockURL:           lockURL,
		SupabaseJWTSecret:         jwtSecret,
		SupabaseJWKSURL:           jwksURL,
		SupabaseJWTIssuer:         issuer,
		SupabaseJWTAudience:       audience,
		SupabaseURL:               supabaseURL,
		SupabaseServiceRoleKey:    serviceRoleKey,
		SupabaseAvatarBucket:      avatarBucket,
		AllowedOrigins:            origins,
		Environment:               env,
		LogLevel:                  logLevel,
		DatabaseMaxConns:          maxConns,
		DatabaseMinConns:          minConns,
		DatabaseLockMaxConns:      lockMaxConns,
		DatabaseMaxConnLifetime:   maxConnLifetime,
		DatabaseMaxConnIdleTime:   maxConnIdleTime,
		DatabaseHealthCheckPeriod: healthCheckPeriod,
	}, nil
}

func canonicalHTTPSBaseURL(raw string) (string, error) {
	canonical := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.ParseRequestURI(canonical)
	if err != nil ||
		parsed.Scheme != "https" ||
		parsed.Host == "" ||
		parsed.Path != "" ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" ||
		parsed.User != nil {
		return "", ErrProductionAdmin
	}
	return canonical, nil
}

func validateProductionLockURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return ErrProductionDatabaseLock
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil ||
		(parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Host == "" ||
		parsed.Port() == "6543" {
		return ErrProductionDatabaseLock
	}
	return nil
}

func positiveInt32Env(name string, fallback int32) (int32, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%w: %s must be a positive integer", ErrInvalidDatabasePool, name)
	}
	return int32(parsed), nil
}

func nonNegativeInt32Env(name string, fallback int32) (int32, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", ErrInvalidDatabasePool, name)
	}
	return int32(parsed), nil
}

func positiveDurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%w: %s must be a positive duration", ErrInvalidDatabasePool, name)
	}
	return parsed, nil
}
