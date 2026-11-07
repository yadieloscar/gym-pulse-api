package dao

import (
	"os"
	"strings"
	"testing"
)

func TestExportDAOReadsOneOwnedReadOnlySnapshot(t *testing.T) {
	body, err := os.ReadFile("export_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, invariant := range []string{
		"pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}",
		"defer tx.Rollback(ctx)",
		"WHERE id=$1", // user_profiles keyed by user id
		"WHERE user_id=$1 AND status <> 'discarded'",
		"WHERE ws.user_id=$1 AND ws.status <> 'discarded'",
		"WHERE dl.user_id=$1",
		"WHERE t.user_id=$1",
		"WHERE fe.user_id=$1",
		"uf.user_id = fe.user_id",
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("export DAO missing %q", invariant)
		}
	}
	// Every query runs inside the snapshot transaction, never on the pool.
	if strings.Contains(source, "r.pool.Query") || strings.Contains(source, "r.pool.Exec") {
		t.Error("export queries must use the snapshot transaction")
	}
	for _, write := range []string{"INSERT ", "UPDATE ", "DELETE "} {
		if strings.Contains(source, write) {
			t.Errorf("export DAO must not write (%s)", strings.TrimSpace(write))
		}
	}
	if got := strings.Count(source, "$1"); got < 15 {
		t.Errorf("expected every section to bind the user id, found %d binds", got)
	}
}
