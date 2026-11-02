//go:build integration

package dao

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type generationFixture struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	ledger ProgramGenerationDAO
	userID uuid.UUID
}

func newGenerationFixture(t *testing.T) *generationFixture {
	t.Helper()
	url := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("integration tests require GYMPULSE_TEST_DATABASE_URL pointing to a disposable migrated database")
	}
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	userID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO auth.users (id) VALUES ($1)`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM auth.users WHERE id = $1`, userID)
	})
	return &generationFixture{ctx: ctx, pool: pool, ledger: NewProgramGenerationDAO(pool, 2*time.Minute), userID: userID}
}

func (f *generationFixture) setCreatedAt(t *testing.T, id uuid.UUID, interval string) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, `UPDATE program_generations SET created_at = now() - $2::interval WHERE id = $1`, id, interval); err != nil {
		t.Fatal(err)
	}
}

func TestProgramGenerationLedgerEnforcesTheDailyLimit(t *testing.T) {
	f := newGenerationFixture(t)
	const limit = 5
	for i := 1; i <= limit; i++ {
		reservation, err := f.ledger.Reserve(f.ctx, f.userID, "2026-10-31", limit, "m")
		if err != nil {
			t.Fatalf("reservation %d: %v", i, err)
		}
		if reservation.Used != i {
			t.Fatalf("reservation %d reports used=%d", i, reservation.Used)
		}
		if err := f.ledger.Complete(f.ctx, reservation.ID, model.GenerationSucceeded); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.ledger.Reserve(f.ctx, f.userID, "2026-10-31", limit, "m")
	var limitErr *model.GenerationLimitError
	if !errors.As(err, &limitErr) || limitErr.Limit != limit {
		t.Fatalf("sixth reservation must hit the limit, got %v", err)
	}
	used, err := f.ledger.CountToday(f.ctx, f.userID, "2026-10-31")
	if err != nil || used != limit {
		t.Fatalf("count: %d %v", used, err)
	}

	// Another day and another athlete are independent.
	if _, err := f.ledger.Reserve(f.ctx, f.userID, "2026-11-01", limit, "m"); err != nil {
		t.Fatalf("next day must be free: %v", err)
	}
	other := newGenerationFixture(t)
	if _, err := other.ledger.Reserve(other.ctx, other.userID, "2026-10-31", limit, "m"); err != nil {
		t.Fatalf("another athlete must be unaffected: %v", err)
	}
}

func TestProgramGenerationLedgerNeverChargesFailures(t *testing.T) {
	f := newGenerationFixture(t)
	const limit = 2
	failed, err := f.ledger.Reserve(f.ctx, f.userID, "2026-10-31", limit, "m")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ledger.Complete(f.ctx, failed.ID, model.GenerationFailed); err != nil {
		t.Fatal(err)
	}
	stale, err := f.ledger.Reserve(f.ctx, f.userID, "2026-10-31", limit, "m")
	if err != nil {
		t.Fatal(err)
	}
	f.setCreatedAt(t, stale.ID, "3 minutes")
	fresh, err := f.ledger.Reserve(f.ctx, f.userID, "2026-10-31", limit, "m")
	if err != nil {
		t.Fatalf("stale pending must not count: %v", err)
	}
	if fresh.Used != 1 {
		t.Fatalf("only the fresh pending row counts, got used=%d", fresh.Used)
	}
	// A fresh pending row counts; one more succeeds and then the limit holds.
	if _, err := f.ledger.Reserve(f.ctx, f.userID, "2026-10-31", limit, "m"); err != nil {
		t.Fatalf("second live slot must be available: %v", err)
	}
	_, err = f.ledger.Reserve(f.ctx, f.userID, "2026-10-31", limit, "m")
	var limitErr *model.GenerationLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("two live rows must exhaust a limit of two, got %v", err)
	}
	// Completing a stale row as failed is a no-op for an already-counted
	// succeeded row: Complete only moves pending rows.
	if err := f.ledger.Complete(f.ctx, fresh.ID, model.GenerationSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := f.ledger.Complete(f.ctx, fresh.ID, model.GenerationFailed); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := f.pool.QueryRow(f.ctx, `SELECT status FROM program_generations WHERE id = $1`, fresh.ID).Scan(&status); err != nil || status != model.GenerationSucceeded {
		t.Fatalf("a succeeded row must stay succeeded: %s %v", status, err)
	}
}

func TestProgramGenerationLedgerSerializesConcurrentReservations(t *testing.T) {
	f := newGenerationFixture(t)
	const limit, attempts = 5, 10
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.ledger.Reserve(f.ctx, f.userID, "2026-10-31", limit, "m")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	granted, limited := 0, 0
	for err := range results {
		var limitErr *model.GenerationLimitError
		switch {
		case err == nil:
			granted++
		case errors.As(err, &limitErr):
			limited++
		default:
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	if granted != limit || limited != attempts-limit {
		t.Fatalf("want exactly %d granted and %d limited, got %d/%d", limit, attempts-limit, granted, limited)
	}
}
