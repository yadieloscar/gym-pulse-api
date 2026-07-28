package dao

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestSupabaseSecurityMigrationProtectsEveryPublicApplicationTable(t *testing.T) {
	migrationFiles, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrationFiles) == 0 {
		t.Fatal("no up migrations found")
	}

	var migrations strings.Builder
	for _, filename := range migrationFiles {
		body, readErr := os.ReadFile(filename)
		if readErr != nil {
			t.Fatal(readErr)
		}
		migrations.Write(body)
		migrations.WriteByte('\n')
	}

	contents := migrations.String()
	createTable := regexp.MustCompile(`(?im)^\s*CREATE TABLE(?: IF NOT EXISTS)?\s+([a-zA-Z0-9_.]+)\s*\(`)
	var publicTables []string
	for _, match := range createTable.FindAllStringSubmatch(contents, -1) {
		table := strings.ToLower(match[1])
		if strings.Contains(table, ".") {
			if !strings.HasPrefix(table, "public.") {
				continue
			}
		} else {
			table = "public." + table
		}
		if !slices.Contains(publicTables, table) {
			publicTables = append(publicTables, table)
		}
	}

	if len(publicTables) != 24 {
		t.Fatalf("found %d public application tables, want 24: %v", len(publicTables), publicTables)
	}
	for _, table := range publicTables {
		statement := "ALTER TABLE " + table + " ENABLE ROW LEVEL SECURITY;"
		if !strings.Contains(contents, statement) {
			t.Errorf("%s is created without an RLS migration", table)
		}
	}
	if strings.Contains(strings.ToUpper(contents), "FORCE ROW LEVEL SECURITY") {
		t.Fatal("application migrations must not FORCE RLS; the direct API database role relies on owner bypass")
	}
}

func TestSupabaseSecurityMigrationRevokesDataAPIAccess(t *testing.T) {
	body, err := os.ReadFile("../../migrations/016_secure_supabase_data_api.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	contents := string(body)

	for _, role := range []string{"anon", "authenticated", "service_role"} {
		if !strings.Contains(contents, "'"+role+"'") {
			t.Errorf("migration does not account for %s", role)
		}
	}
	for _, objectType := range []string{"TABLES", "SEQUENCES", "FUNCTIONS"} {
		if !strings.Contains(contents, "REVOKE ALL PRIVILEGES ON ALL "+objectType+" IN SCHEMA public") {
			t.Errorf("migration does not revoke existing %s privileges", strings.ToLower(objectType))
		}
		if !strings.Contains(contents, "REVOKE ALL PRIVILEGES ON "+objectType+" FROM") {
			t.Errorf("migration does not revoke default %s privileges", strings.ToLower(objectType))
		}
	}
	if !strings.Contains(contents, "REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC") {
		t.Error("migration leaves PostgreSQL's default PUBLIC function execution enabled")
	}
}
