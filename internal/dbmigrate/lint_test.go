package dbmigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// strictFromVersion is the first migration held to the re-runnable rules
// below. 001–042 were applied before the rules existed and are never edited;
// golang-migrate does not re-run an applied version, so rewriting them would
// change nothing in a live database and only risk drift with fresh ones.
const strictFromVersion = 43

var migrationName = regexp.MustCompile(`^(\d{3})_[a-z0-9_]+\.(up|down)\.sql$`)

type migrationFile struct {
	name    string
	version int
	up      bool
	// code is the file with comments, string literals, and dollar-quoted
	// bodies (DO blocks, functions) blanked out, so rules only see top-level
	// statements. A DO block is expected to guard its own statements.
	code string
}

func loadMigrationFiles(t *testing.T) []migrationFile {
	t.Helper()
	paths, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no migrations found")
	}
	var files []migrationFile
	for _, path := range paths {
		name := filepath.Base(path)
		match := migrationName.FindStringSubmatch(name)
		if match == nil {
			t.Errorf("%s: name must be NNN_snake_case.up.sql or NNN_snake_case.down.sql", name)
			continue
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, migrationFile{
			name:    name,
			version: version,
			up:      match[2] == "up",
			code:    strings.ToUpper(topLevelSQL(string(body))),
		})
	}
	return files
}

// topLevelSQL blanks out comments, quoted literals, and dollar-quoted bodies,
// keeping statement boundaries intact.
func topLevelSQL(sql string) string {
	var out strings.Builder
	for i := 0; i < len(sql); {
		switch {
		case strings.HasPrefix(sql[i:], "--"):
			end := strings.IndexByte(sql[i:], '\n')
			if end < 0 {
				return out.String()
			}
			i += end
		case strings.HasPrefix(sql[i:], "/*"):
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return out.String()
			}
			i += end + 4
		case sql[i] == '\'':
			// '' is an escaped quote inside a literal.
			j := i + 1
			for j < len(sql) {
				if sql[j] == '\'' {
					if j+1 < len(sql) && sql[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			out.WriteString("''")
			i = j + 1
		case sql[i] == '$':
			tag := dollarTag.FindString(sql[i:])
			if tag == "" {
				out.WriteByte(sql[i])
				i++
				continue
			}
			end := strings.Index(sql[i+len(tag):], tag)
			if end < 0 {
				return out.String()
			}
			out.WriteString(" $BODY$ ")
			i += len(tag) + end + len(tag)
		default:
			out.WriteByte(sql[i])
			i++
		}
	}
	return out.String()
}

var dollarTag = regexp.MustCompile(`^\$[A-Za-z_]*\$`)

func statements(code string) []string {
	var out []string
	for _, statement := range strings.Split(code, ";") {
		if fields := strings.Fields(statement); len(fields) > 0 {
			out = append(out, strings.Join(fields, " "))
		}
	}
	return out
}

func TestMigrationFilesArePairedAndUnique(t *testing.T) {
	files := loadMigrationFiles(t)
	type pair struct{ up, down []string }
	byVersion := map[int]*pair{}
	maxVersion := 0
	for _, f := range files {
		p := byVersion[f.version]
		if p == nil {
			p = &pair{}
			byVersion[f.version] = p
		}
		if f.up {
			p.up = append(p.up, f.name)
		} else {
			p.down = append(p.down, f.name)
		}
		maxVersion = max(maxVersion, f.version)
	}
	for version, p := range byVersion {
		if len(p.up) != 1 || len(p.down) != 1 {
			t.Errorf("version %03d needs exactly one up and one down file, has up=%v down=%v (renumber after fetching origin/main)", version, p.up, p.down)
			continue
		}
		if strings.TrimSuffix(p.up[0], ".up.sql") != strings.TrimSuffix(p.down[0], ".down.sql") {
			t.Errorf("version %03d up and down names differ: %s, %s", version, p.up[0], p.down[0])
		}
	}
	// Gaps before the strict range are historical; new versions are contiguous
	// so a skipped or stale number shows up in review.
	for version := strictFromVersion; version <= maxVersion; version++ {
		if byVersion[version] == nil {
			t.Errorf("version %03d is missing; new migrations must be numbered without gaps", version)
		}
	}
}

func TestEveryMigrationIsSingleTransaction(t *testing.T) {
	// golang-migrate sends each file as one multi-statement query, which
	// PostgreSQL runs as one implicit transaction: the whole file applies or
	// none of it does. Explicit transaction control or CONCURRENTLY breaks that.
	forbidden := regexp.MustCompile(`^(BEGIN|COMMIT|ROLLBACK|START TRANSACTION|END|SAVEPOINT)\b|\bCONCURRENTLY\b|^VACUUM\b`)
	for _, f := range loadMigrationFiles(t) {
		for _, statement := range statements(f.code) {
			if forbidden.MatchString(statement) {
				t.Errorf("%s: %q breaks the one-transaction-per-file guarantee", f.name, statement)
			}
		}
	}
}

// rerunRules make a strict migration safe to apply twice and safe against
// rows or objects that already exist (a partly restored database, a manual
// hotfix, a seed that ran on another branch).
var rerunRules = []struct {
	bad  *regexp.Regexp
	ok   *regexp.Regexp
	hint string
}{
	{regexp.MustCompile(`^INSERT INTO\b`), regexp.MustCompile(`\bON CONFLICT\b`), "INSERT needs ON CONFLICT (...) DO NOTHING or DO UPDATE"},
	{regexp.MustCompile(`^CREATE (UNLOGGED )?TABLE\b`), regexp.MustCompile(`^CREATE (UNLOGGED )?TABLE IF NOT EXISTS\b`), "use CREATE TABLE IF NOT EXISTS"},
	{regexp.MustCompile(`^CREATE (UNIQUE )?INDEX\b`), regexp.MustCompile(`^CREATE (UNIQUE )?INDEX IF NOT EXISTS\b`), "use CREATE INDEX IF NOT EXISTS with an explicit name"},
	{regexp.MustCompile(`^CREATE (SCHEMA|SEQUENCE|EXTENSION)\b`), regexp.MustCompile(`^CREATE (SCHEMA|SEQUENCE|EXTENSION) IF NOT EXISTS\b`), "use IF NOT EXISTS"},
	{regexp.MustCompile(`^CREATE (FUNCTION|PROCEDURE|VIEW|TRIGGER)\b`), nil, "use CREATE OR REPLACE"},
	{regexp.MustCompile(`^CREATE TYPE\b`), nil, "use a CHECK constraint, or create the type inside a DO block that checks pg_type"},
	{regexp.MustCompile(`\bADD COLUMN\b`), regexp.MustCompile(`\bADD COLUMN IF NOT EXISTS\b`), "use ADD COLUMN IF NOT EXISTS"},
	{regexp.MustCompile(`^DROP (TABLE|INDEX|SCHEMA|SEQUENCE|FUNCTION|PROCEDURE|VIEW|TYPE|TRIGGER|POLICY|EXTENSION)\b`), regexp.MustCompile(`^DROP \w+ IF EXISTS\b`), "use DROP ... IF EXISTS"},
	{regexp.MustCompile(`\bDROP (COLUMN|CONSTRAINT)\b`), regexp.MustCompile(`\bDROP (COLUMN|CONSTRAINT) IF EXISTS\b`), "use DROP COLUMN/CONSTRAINT IF EXISTS"},
	{regexp.MustCompile(`\bRENAME\b`), nil, "renames are not re-runnable; do it inside a DO block that checks the catalog first"},
}

var (
	createPolicy  = regexp.MustCompile(`^CREATE POLICY (\S+) ON (\S+)`)
	addConstraint = regexp.MustCompile(`\bADD CONSTRAINT (\S+)`)
)

func TestStrictMigrationsAreRerunnable(t *testing.T) {
	for _, f := range loadMigrationFiles(t) {
		if f.version < strictFromVersion {
			continue
		}
		all := statements(f.code)
		joined := strings.Join(all, ";\n")
		for _, statement := range all {
			for _, rule := range rerunRules {
				if rule.bad.MatchString(statement) && (rule.ok == nil || !rule.ok.MatchString(statement)) {
					t.Errorf("%s: %s: %q", f.name, rule.hint, statement)
				}
			}
			// PostgreSQL has no IF NOT EXISTS for policies or constraints, so the
			// file must drop the same name first.
			if m := createPolicy.FindStringSubmatch(statement); m != nil {
				if !strings.Contains(joined, fmt.Sprintf("DROP POLICY IF EXISTS %s ON %s", m[1], m[2])) {
					t.Errorf("%s: CREATE POLICY %s needs DROP POLICY IF EXISTS %s ON %s earlier in the file", f.name, m[1], m[1], m[2])
				}
			}
			for _, m := range addConstraint.FindAllStringSubmatch(statement, -1) {
				if !strings.Contains(joined, "DROP CONSTRAINT IF EXISTS "+m[1]) {
					t.Errorf("%s: ADD CONSTRAINT %s needs DROP CONSTRAINT IF EXISTS %s earlier in the file", f.name, m[1], m[1])
				}
			}
		}
		if f.up && regexp.MustCompile(`^CREATE (UNLOGGED )?TABLE`).MatchString(joined) && !strings.Contains(joined, "GYM_PULSE_PRIVATE.SECURE_TABLE(") {
			t.Errorf("%s creates a table without SELECT gym_pulse_private.secure_table('public.<table>')", f.name)
		}
	}
}

func TestTopLevelSQLIgnoresBodiesAndComments(t *testing.T) {
	got := strings.Join(statements(strings.ToUpper(topLevelSQL(`
-- CREATE TABLE in a comment
CREATE TABLE IF NOT EXISTS public.a (note text DEFAULT 'it''s; INSERT INTO x');
DO $$ BEGIN INSERT INTO b VALUES (1); END $$;
CREATE OR REPLACE FUNCTION f() RETURNS void LANGUAGE sql AS $fn$ DELETE FROM c; $fn$;
/* DROP TABLE d; */
`))), " | ")
	want := "CREATE TABLE IF NOT EXISTS PUBLIC.A (NOTE TEXT DEFAULT '') | DO $BODY$ | CREATE OR REPLACE FUNCTION F() RETURNS VOID LANGUAGE SQL AS $BODY$"
	if got != want {
		t.Fatalf("statements =\n%s\nwant\n%s", got, want)
	}
}
