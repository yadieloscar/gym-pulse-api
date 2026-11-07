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

func newLogOnceFixture(t *testing.T) (context.Context, *pgxpool.Pool, *logDAO, uuid.UUID) {
	t.Helper()
	url := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("integration tests require GYMPULSE_TEST_DATABASE_URL pointing to a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
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
		cleanupCtx := context.WithoutCancel(t.Context())
		if err := NewAccountDAO(pool).DeleteUserData(cleanupCtx, userID); err != nil {
			t.Errorf("cleaning fixture data: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM auth.users WHERE id=$1`, userID); err != nil {
			t.Errorf("cleaning fixture identity: %v", err)
		}
	})
	return ctx, pool, &logDAO{pool: pool}, userID
}

func countDayLogs(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM day_logs WHERE user_id=$1`, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func onceLog() *model.DayLog {
	return &model.DayLog{Date: "2026-10-12", TypeID: "push", SubtypeID: "hypertrophy"}
}

func TestLogDAOCreateOnceReplaysWithoutASecondLog(t *testing.T) {
	ctx, pool, logs, userID := newLogOnceFixture(t)

	first := onceLog()
	if replayed, err := logs.CreateOnce(ctx, userID, first, "finish-1", "hash-a"); err != nil || replayed {
		t.Fatalf("first create: replayed=%v err=%v", replayed, err)
	}
	second := onceLog()
	replayed, err := logs.CreateOnce(ctx, userID, second, "finish-1", "hash-a")
	if err != nil || !replayed {
		t.Fatalf("replay: replayed=%v err=%v", replayed, err)
	}
	if second.ID != first.ID || !second.LoggedAt.Equal(first.LoggedAt) {
		t.Errorf("replay returned %s at %s, want %s at %s", second.ID, second.LoggedAt, first.ID, first.LoggedAt)
	}
	if n := countDayLogs(t, ctx, pool, userID); n != 1 {
		t.Errorf("expected 1 day log after replay, got %d", n)
	}

	_, err = logs.CreateOnce(ctx, userID, onceLog(), "finish-1", "hash-b")
	var conflict *model.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected idempotency conflict for a changed payload, got %v", err)
	}
	if n := countDayLogs(t, ctx, pool, userID); n != 1 {
		t.Errorf("expected 1 day log after conflict, got %d", n)
	}
}

func TestLogDAOCreateOnceConcurrentDeliveriesCreateOneLog(t *testing.T) {
	ctx, pool, logs, userID := newLogOnceFixture(t)

	var wg sync.WaitGroup
	ids := make([]uuid.UUID, 4)
	errs := make([]error, 4)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dl := onceLog()
			_, errs[i] = logs.CreateOnce(ctx, userID, dl, "finish-race", "hash-a")
			ids[i] = dl.ID
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
		if ids[i] != ids[0] {
			t.Errorf("delivery %d returned %s, want %s", i, ids[i], ids[0])
		}
	}
	if n := countDayLogs(t, ctx, pool, userID); n != 1 {
		t.Errorf("expected 1 day log after concurrent deliveries, got %d", n)
	}
}
