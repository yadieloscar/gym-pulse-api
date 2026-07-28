package config

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// setEnv sets an env var for the test and restores it after.
func setEnv(t *testing.T, key, val string) {
	t.Helper()
	t.Setenv(key, val)
}

// clearAll unsets all relevant env vars.
func clearAll(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DATABASE_URL", "DATABASE_LOCK_URL", "SUPABASE_JWT_SECRET", "PORT", "ENVIRONMENT", "LOG_LEVEL", "ALLOWED_ORIGINS", "SUPABASE_JWKS_URL", "SUPABASE_JWT_ISSUER", "SUPABASE_JWT_AUDIENCE", "SUPABASE_URL", "SUPABASE_SERVICE_ROLE_KEY", "SUPABASE_AVATAR_BUCKET", "DATABASE_MAX_CONNS", "DATABASE_MIN_CONNS", "DATABASE_LOCK_MAX_CONNS", "DATABASE_MAX_CONN_LIFETIME", "DATABASE_MAX_CONN_IDLE_TIME", "DATABASE_HEALTH_CHECK_PERIOD"} {
		t.Setenv(k, "")
	}
}

func setValidReleaseEnv(t *testing.T, environment string) {
	t.Helper()
	setEnv(t, "DATABASE_URL", "postgres://app.example.com:6543/postgres")
	setEnv(t, "DATABASE_LOCK_URL", "postgres://app.example.com:5432/postgres")
	setEnv(t, "ENVIRONMENT", environment)
	setEnv(t, "ALLOWED_ORIGINS", "https://app.example.com")
	setEnv(t, "SUPABASE_URL", "https://project.supabase.co")
	setEnv(t, "SUPABASE_SERVICE_ROLE_KEY", "server-only-key")
	setEnv(t, "SUPABASE_JWT_AUDIENCE", "authenticated")
}

func setValidDevelopmentEnv(t *testing.T) {
	t.Helper()
	setEnv(t, "DATABASE_URL", "postgres://x")
	setEnv(t, "SUPABASE_JWT_SECRET", "secret")
	setEnv(t, "ENVIRONMENT", "development")
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	clearAll(t)
	setEnv(t, "ENVIRONMENT", "development")
	_, err := Load()
	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Errorf("expected ErrMissingDatabaseURL, got %v", err)
	}
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	clearAll(t)
	setEnv(t, "DATABASE_URL", "postgres://x")
	setEnv(t, "ENVIRONMENT", "development")
	_, err := Load()
	if !errors.Is(err, ErrMissingJWTConfig) {
		t.Errorf("expected ErrMissingJWTConfig, got %v", err)
	}
}

func TestLoad_RequiresExplicitEnvironment(t *testing.T) {
	for _, value := range []string{"", " \t "} {
		t.Run("value="+value, func(t *testing.T) {
			clearAll(t)
			setEnv(t, "DATABASE_URL", "postgres://x")
			setEnv(t, "SUPABASE_JWT_SECRET", "secret")
			setEnv(t, "ENVIRONMENT", value)

			if _, err := Load(); !errors.Is(err, ErrInvalidEnvironment) {
				t.Fatalf("expected ErrInvalidEnvironment for %q, got %v", value, err)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearAll(t)
	setValidDevelopmentEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected explicit development env, got %q", cfg.Environment)
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
	if cfg.DatabaseMaxConns != 10 || cfg.DatabaseMinConns != 1 {
		t.Errorf("unexpected database pool defaults: max=%d min=%d", cfg.DatabaseMaxConns, cfg.DatabaseMinConns)
	}
	if cfg.DatabaseLockURL != cfg.DatabaseURL {
		t.Errorf("development lock URL = %q, want DATABASE_URL %q", cfg.DatabaseLockURL, cfg.DatabaseURL)
	}
	if cfg.DatabaseLockMaxConns != 4 {
		t.Errorf("unexpected database lock pool default: max=%d", cfg.DatabaseLockMaxConns)
	}
	if cfg.DatabaseMaxConnLifetime != 30*time.Minute || cfg.DatabaseMaxConnIdleTime != 5*time.Minute || cfg.DatabaseHealthCheckPeriod != time.Minute {
		t.Errorf("unexpected database duration defaults: lifetime=%s idle=%s health=%s", cfg.DatabaseMaxConnLifetime, cfg.DatabaseMaxConnIdleTime, cfg.DatabaseHealthCheckPeriod)
	}
}

func TestLoad_DevelopmentAcceptsJWKSWithoutLegacySecret(t *testing.T) {
	clearAll(t)
	setEnv(t, "DATABASE_URL", "postgres://x")
	setEnv(t, "ENVIRONMENT", "development")
	setEnv(t, "SUPABASE_JWKS_URL", "https://project.example/auth/v1/.well-known/jwks.json")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Overrides(t *testing.T) {
	clearAll(t)
	setValidReleaseEnv(t, "production")
	setEnv(t, "PORT", "9090")
	setEnv(t, "LOG_LEVEL", "debug")
	setEnv(t, "ALLOWED_ORIGINS", "https://a.com, https://b.com ,, https://c.com")
	setEnv(t, "SUPABASE_JWKS_URL", "https://project.supabase.co/auth/v1/.well-known/jwks.json")
	setEnv(t, "SUPABASE_JWT_ISSUER", "https://project.supabase.co/auth/v1")
	setEnv(t, "DATABASE_MAX_CONNS", "24")
	setEnv(t, "DATABASE_MIN_CONNS", "3")
	setEnv(t, "DATABASE_LOCK_MAX_CONNS", "6")
	setEnv(t, "DATABASE_MAX_CONN_LIFETIME", "45m")
	setEnv(t, "DATABASE_MAX_CONN_IDLE_TIME", "7m")
	setEnv(t, "DATABASE_HEALTH_CHECK_PERIOD", "30s")

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
	if cfg.SupabaseJWKSURL != "https://project.supabase.co/auth/v1/.well-known/jwks.json" {
		t.Errorf("jwks: %q", cfg.SupabaseJWKSURL)
	}
	if cfg.SupabaseJWTIssuer != "https://project.supabase.co/auth/v1" {
		t.Errorf("issuer: %q", cfg.SupabaseJWTIssuer)
	}
	if cfg.DatabaseLockURL != "postgres://app.example.com:5432/postgres" {
		t.Errorf("lock URL: %q", cfg.DatabaseLockURL)
	}
	if len(cfg.AllowedOrigins) != 3 {
		t.Fatalf("expected 3 origins, got %v", cfg.AllowedOrigins)
	}
	if cfg.AllowedOrigins[0] != "https://a.com" || cfg.AllowedOrigins[2] != "https://c.com" {
		t.Errorf("origins not trimmed: %v", cfg.AllowedOrigins)
	}
	if cfg.DatabaseMaxConns != 24 || cfg.DatabaseMinConns != 3 {
		t.Errorf("database pool overrides: max=%d min=%d", cfg.DatabaseMaxConns, cfg.DatabaseMinConns)
	}
	if cfg.DatabaseLockMaxConns != 6 {
		t.Errorf("database lock pool override: max=%d", cfg.DatabaseLockMaxConns)
	}
	if cfg.DatabaseMaxConnLifetime != 45*time.Minute || cfg.DatabaseMaxConnIdleTime != 7*time.Minute || cfg.DatabaseHealthCheckPeriod != 30*time.Second {
		t.Errorf("database duration overrides: lifetime=%s idle=%s health=%s", cfg.DatabaseMaxConnLifetime, cfg.DatabaseMaxConnIdleTime, cfg.DatabaseHealthCheckPeriod)
	}
}

func TestLoad_ProductionDerivesAsymmetricClaimValidation(t *testing.T) {
	clearAll(t)
	setValidReleaseEnv(t, "production")
	setEnv(t, "SUPABASE_JWT_SECRET", "legacy-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SupabaseJWTIssuer != "https://project.supabase.co/auth/v1" {
		t.Fatalf("derived issuer = %q", cfg.SupabaseJWTIssuer)
	}
	if cfg.SupabaseJWKSURL != "https://project.supabase.co/auth/v1/.well-known/jwks.json" {
		t.Fatalf("derived JWKS URL = %q", cfg.SupabaseJWKSURL)
	}
}

func TestLoad_ProductionNormalizesCriticalValues(t *testing.T) {
	clearAll(t)
	setValidReleaseEnv(t, "production")
	setEnv(t, "SUPABASE_URL", " https://project.supabase.co/ ")
	setEnv(t, "SUPABASE_SERVICE_ROLE_KEY", " server-only-key ")
	setEnv(t, "SUPABASE_JWT_ISSUER", " https://project.supabase.co/auth/v1/ ")
	setEnv(t, "SUPABASE_JWKS_URL", " https://project.supabase.co/auth/v1/.well-known/jwks.json/ ")
	setEnv(t, "SUPABASE_JWT_AUDIENCE", " authenticated ")
	setEnv(t, "DATABASE_LOCK_URL", " postgres://app.example.com:5432/postgres ")
	setEnv(t, "SUPABASE_AVATAR_BUCKET", " avatars ")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SupabaseURL != "https://project.supabase.co" ||
		cfg.SupabaseServiceRoleKey != "server-only-key" ||
		cfg.SupabaseJWTIssuer != "https://project.supabase.co/auth/v1" ||
		cfg.SupabaseJWKSURL != "https://project.supabase.co/auth/v1/.well-known/jwks.json" ||
		cfg.SupabaseJWTAudience != "authenticated" ||
		cfg.DatabaseLockURL != "postgres://app.example.com:5432/postgres" ||
		cfg.SupabaseAvatarBucket != "avatars" {
		t.Fatalf("release values were not normalized: %+v", cfg)
	}
}

func TestLoad_ProductionRejectsWhitespaceOnlyCriticalValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		want error
	}{
		{name: "Supabase URL", key: "SUPABASE_URL", want: ErrProductionAdmin},
		{name: "service role", key: "SUPABASE_SERVICE_ROLE_KEY", want: ErrProductionAdmin},
		{name: "audience", key: "SUPABASE_JWT_AUDIENCE", want: ErrProductionAuth},
		{name: "lock URL", key: "DATABASE_LOCK_URL", want: ErrProductionDatabaseLock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAll(t)
			setValidReleaseEnv(t, "production")
			setEnv(t, tc.key, " \t ")

			if _, err := Load(); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v for whitespace-only %s, got %v", tc.want, tc.key, err)
			}
		})
	}
}

func TestLoad_NormalizesKnownEnvironmentAndRejectsTypos(t *testing.T) {
	clearAll(t)
	setEnv(t, "DATABASE_URL", "postgres://x")
	setEnv(t, "SUPABASE_JWT_SECRET", "secret")
	setEnv(t, "ENVIRONMENT", " TeSt ")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "test" {
		t.Fatalf("environment = %q, want test", cfg.Environment)
	}

	setEnv(t, "ENVIRONMENT", "prodution")
	if _, err := Load(); !errors.Is(err, ErrInvalidEnvironment) {
		t.Fatalf("expected ErrInvalidEnvironment, got %v", err)
	}
}

func TestLoad_StagingUsesProductionSecurityRequirements(t *testing.T) {
	clearAll(t)
	setValidReleaseEnv(t, "staging")
	setEnv(t, "SUPABASE_JWT_AUDIENCE", "")

	if _, err := Load(); !errors.Is(err, ErrProductionAuth) {
		t.Fatalf("expected staging to enforce release auth configuration, got %v", err)
	}
}

func TestLoad_ProductionRequiresSupabaseAdminConfiguration(t *testing.T) {
	clearAll(t)
	setEnv(t, "DATABASE_URL", "postgres://x")
	setEnv(t, "ENVIRONMENT", "production")
	setEnv(t, "SUPABASE_JWT_AUDIENCE", "authenticated")

	if _, err := Load(); !errors.Is(err, ErrProductionAdmin) {
		t.Fatalf("expected ErrProductionAdmin, got %v", err)
	}

	setEnv(t, "SUPABASE_URL", "http://project.example.com")
	setEnv(t, "SUPABASE_SERVICE_ROLE_KEY", "server-only-key")
	if _, err := Load(); !errors.Is(err, ErrProductionAdmin) {
		t.Fatalf("expected ErrProductionAdmin for insecure URL, got %v", err)
	}
}

func TestLoad_ProductionRequiresExplicitHTTPSOrigins(t *testing.T) {
	clearAll(t)
	setEnv(t, "DATABASE_URL", "postgres://x")
	setEnv(t, "ENVIRONMENT", "production")
	setEnv(t, "SUPABASE_JWT_AUDIENCE", "authenticated")
	setEnv(t, "SUPABASE_URL", "https://project.example.com")
	setEnv(t, "SUPABASE_SERVICE_ROLE_KEY", "server-only-key")
	setEnv(t, "DATABASE_LOCK_URL", "postgres://project.example.com:5432/postgres")

	if _, err := Load(); !errors.Is(err, ErrProductionCORS) {
		t.Fatalf("expected ErrProductionCORS, got %v", err)
	}

	setEnv(t, "ALLOWED_ORIGINS", "http://app.example.com")
	if _, err := Load(); !errors.Is(err, ErrProductionCORS) {
		t.Fatalf("expected ErrProductionCORS for insecure origin, got %v", err)
	}

	for _, origin := range []string{
		"https://user@app.example.com",
		"https://app.example.com?redirect=other",
		"https://app.example.com/#fragment",
	} {
		setEnv(t, "ALLOWED_ORIGINS", origin)
		if _, err := Load(); !errors.Is(err, ErrProductionCORS) {
			t.Fatalf("expected ErrProductionCORS for malformed origin %q, got %v", origin, err)
		}
	}
}

func TestLoad_ProductionRejectsMismatchedSupabaseProjectIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, key, value string
	}{
		{
			name:  "issuer",
			key:   "SUPABASE_JWT_ISSUER",
			value: "https://other.supabase.co/auth/v1",
		},
		{
			name:  "JWKS",
			key:   "SUPABASE_JWKS_URL",
			value: "https://other.supabase.co/auth/v1/.well-known/jwks.json",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAll(t)
			setValidReleaseEnv(t, "production")
			setEnv(t, tc.key, tc.value)

			if _, err := Load(); !errors.Is(err, ErrSupabaseProjectMismatch) {
				t.Fatalf("expected ErrSupabaseProjectMismatch, got %v", err)
			}
		})
	}
}

func TestLoad_ProductionRequiresSessionCapableLockURL(t *testing.T) {
	t.Run("explicit lock URL required", func(t *testing.T) {
		clearAll(t)
		setValidReleaseEnv(t, "production")
		setEnv(t, "DATABASE_LOCK_URL", "")

		if _, err := Load(); !errors.Is(err, ErrProductionDatabaseLock) {
			t.Fatalf("expected ErrProductionDatabaseLock, got %v", err)
		}
	})

	t.Run("Supavisor transaction port rejected", func(t *testing.T) {
		clearAll(t)
		setValidReleaseEnv(t, "production")
		setEnv(t, "DATABASE_LOCK_URL", "postgres://project.pooler.supabase.com:6543/postgres")

		if _, err := Load(); !errors.Is(err, ErrProductionDatabaseLock) {
			t.Fatalf("expected ErrProductionDatabaseLock, got %v", err)
		}
	})

	t.Run("Supavisor session port accepted", func(t *testing.T) {
		clearAll(t)
		setValidReleaseEnv(t, "production")
		setEnv(t, "DATABASE_LOCK_URL", "postgres://project.pooler.supabase.com:5432/postgres")

		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DatabaseLockURL != "postgres://project.pooler.supabase.com:5432/postgres" {
			t.Fatalf("lock URL = %q", cfg.DatabaseLockURL)
		}
	})
}

func TestLoad_RejectsInvalidDatabasePool(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "zero max", key: "DATABASE_MAX_CONNS", value: "0"},
		{name: "negative min", key: "DATABASE_MIN_CONNS", value: "-1"},
		{name: "zero lock max", key: "DATABASE_LOCK_MAX_CONNS", value: "0"},
		{name: "invalid lifetime", key: "DATABASE_MAX_CONN_LIFETIME", value: "later"},
		{name: "zero idle", key: "DATABASE_MAX_CONN_IDLE_TIME", value: "0s"},
		{name: "negative health", key: "DATABASE_HEALTH_CHECK_PERIOD", value: "-1s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAll(t)
			setValidDevelopmentEnv(t)
			setEnv(t, tc.key, tc.value)

			if _, err := Load(); !errors.Is(err, ErrInvalidDatabasePool) {
				t.Fatalf("expected ErrInvalidDatabasePool, got %v", err)
			}
		})
	}

	t.Run("min exceeds max", func(t *testing.T) {
		clearAll(t)
		setValidDevelopmentEnv(t)
		setEnv(t, "DATABASE_MAX_CONNS", "2")
		setEnv(t, "DATABASE_MIN_CONNS", "3")

		if _, err := Load(); !errors.Is(err, ErrInvalidDatabasePool) {
			t.Fatalf("expected ErrInvalidDatabasePool, got %v", err)
		}
	})
}

func TestReleaseArtifactsRequireExplicitProductionEnvironment(t *testing.T) {
	read := func(relativePath string) string {
		t.Helper()
		body, err := os.ReadFile(relativePath)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	if dockerfile := read("../../Dockerfile"); !strings.Contains(dockerfile, "ENV ENVIRONMENT=production") {
		t.Fatal("production image does not default ENVIRONMENT to production")
	}
	if runbook := read("../../docs/RELEASE_RUNBOOK.md"); !strings.Contains(runbook, "`ENVIRONMENT=production`") {
		t.Fatal("release runbook does not require the effective production environment")
	}
	if readme := read("../../README.md"); !strings.Contains(readme, "| `ENVIRONMENT` | Yes |") {
		t.Fatal("README does not mark ENVIRONMENT as required")
	}
	if example := read("../../.env.example"); !strings.Contains(example, "ENVIRONMENT=development") {
		t.Fatal(".env.example does not select an explicit local environment")
	}
}
