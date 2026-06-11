package config

import (
	"errors"
	"testing"
)

// setEnv sets an env var for the test and restores it after.
func setEnv(t *testing.T, key, val string) {
	t.Helper()
	t.Setenv(key, val)
}

// clearAll unsets all relevant env vars.
func clearAll(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DATABASE_URL", "SUPABASE_JWT_SECRET", "PORT", "ENVIRONMENT", "LOG_LEVEL", "ALLOWED_ORIGINS", "SUPABASE_JWKS_URL"} {
		t.Setenv(k, "")
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	clearAll(t)
	_, err := Load()
	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Errorf("expected ErrMissingDatabaseURL, got %v", err)
	}
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	clearAll(t)
	setEnv(t, "DATABASE_URL", "postgres://x")
	_, err := Load()
	if !errors.Is(err, ErrMissingJWTSecret) {
		t.Errorf("expected ErrMissingJWTSecret, got %v", err)
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearAll(t)
	setEnv(t, "DATABASE_URL", "postgres://x")
	setEnv(t, "SUPABASE_JWT_SECRET", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected default env, got %q", cfg.Environment)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level, got %q", cfg.LogLevel)
	}
	if len(cfg.AllowedOrigins) != 0 {
		t.Errorf("expected no origins, got %v", cfg.AllowedOrigins)
	}
	if cfg.SupabaseJWKSURL != "" {
		t.Errorf("expected empty JWKS URL")
	}
}

func TestLoad_Overrides(t *testing.T) {
	clearAll(t)
	setEnv(t, "DATABASE_URL", "postgres://x")
	setEnv(t, "SUPABASE_JWT_SECRET", "secret")
	setEnv(t, "PORT", "9090")
	setEnv(t, "ENVIRONMENT", "production")
	setEnv(t, "LOG_LEVEL", "debug")
	setEnv(t, "ALLOWED_ORIGINS", "https://a.com, https://b.com ,, https://c.com")
	setEnv(t, "SUPABASE_JWKS_URL", "https://jwks.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9090" {
		t.Errorf("port: %q", cfg.Port)
	}
	if cfg.Environment != "production" {
		t.Errorf("env: %q", cfg.Environment)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log level: %q", cfg.LogLevel)
	}
	if cfg.SupabaseJWKSURL != "https://jwks.example.com" {
		t.Errorf("jwks: %q", cfg.SupabaseJWKSURL)
	}
	if len(cfg.AllowedOrigins) != 3 {
		t.Fatalf("expected 3 origins, got %v", cfg.AllowedOrigins)
	}
	if cfg.AllowedOrigins[0] != "https://a.com" || cfg.AllowedOrigins[2] != "https://c.com" {
		t.Errorf("origins not trimmed: %v", cfg.AllowedOrigins)
	}
}
