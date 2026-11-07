//go:build integration

package dao

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type mobilityFixture struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	sessions MobilitySessionDAO
	userID   uuid.UUID
}

func newMobilityFixture(t *testing.T) *mobilityFixture {
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
	f := &mobilityFixture{ctx: ctx, pool: pool, sessions: NewMobilitySessionDAO(pool)}
	f.userID = f.newUser(t)
	return f
}

func (f *mobilityFixture) newUser(t *testing.T) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO auth.users (id) VALUES ($1)`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.WithoutCancel(f.ctx), `DELETE FROM auth.users WHERE id = $1`, userID)
	})
	return userID
}

func mobilityInput(date, key string) *model.MobilitySession {
	return &model.MobilitySession{Date: date, RoutineID: "hips-and-hamstrings", RoutineName: "Hips and hamstrings",
		Context: model.MobilityContextCooldown, DurationSeconds: 540, MovesCompleted: 7, MovesTotal: 8,
		ContentVersion: "1", OperationKey: key}
}

func (f *mobilityFixture) count(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM mobility_sessions WHERE user_id=$1`, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMobilitySessionCreateReplayConflictListGetDelete(t *testing.T) {
	f := newMobilityFixture(t)

	first, outcome, err := f.sessions.Create(f.ctx, f.userID, mobilityInput("2026-11-02", "mob-1"), "hash-1")
	if err != nil || outcome != model.MobilitySessionCreated {
		t.Fatalf("create: outcome=%v err=%v", outcome, err)
	}
	if first.ID == uuid.Nil || first.CreatedAt.IsZero() || first.Date != "2026-11-02" || first.OperationKey != "mob-1" || first.Context != model.MobilityContextCooldown {
		t.Fatalf("stored entry wrong: %+v", first)
	}

	replay, outcome, err := f.sessions.Create(f.ctx, f.userID, mobilityInput("2026-11-02", "mob-1"), "hash-1")
	if err != nil || outcome != model.MobilitySessionReplayed || replay.ID != first.ID || !replay.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("replay: %+v outcome=%v err=%v", replay, outcome, err)
	}
	_, _, err = f.sessions.Create(f.ctx, f.userID, mobilityInput("2026-11-01", "mob-1"), "hash-other")
	var conflict *model.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("same key with a different body must conflict: %v", err)
	}
	if f.count(t, f.userID) != 1 {
		t.Fatal("replay and conflict must not write")
	}

	// Another athlete may reuse the key independently.
	other := f.newUser(t)
	foreign, outcome, err := f.sessions.Create(f.ctx, other, mobilityInput("2026-11-02", "mob-1"), "hash-other")
	if err != nil || outcome != model.MobilitySessionCreated {
		t.Fatalf("foreign create: %v", err)
	}

	second, _, err := f.sessions.Create(f.ctx, f.userID, mobilityInput("2026-11-03", "mob-2"), "hash-2")
	if err != nil {
		t.Fatal(err)
	}
	third, _, err := f.sessions.Create(f.ctx, f.userID, mobilityInput("2026-11-03", "mob-3"), "hash-3")
	if err != nil {
		t.Fatal(err)
	}

	listed, err := f.sessions.List(f.ctx, f.userID, "2026-11-01", "2026-11-03")
	if err != nil || len(listed) != 3 {
		t.Fatalf("list: %v err=%v", listed, err)
	}
	if listed[0].ID != third.ID || listed[1].ID != second.ID || listed[2].ID != first.ID {
		t.Fatalf("list must be newest first by date, created_at, id: %v", []uuid.UUID{listed[0].ID, listed[1].ID, listed[2].ID})
	}
	ranged, err := f.sessions.List(f.ctx, f.userID, "2026-11-02", "2026-11-02")
	if err != nil || len(ranged) != 1 || ranged[0].ID != first.ID {
		t.Fatalf("inclusive single-day range: %v err=%v", ranged, err)
	}
	empty, err := f.sessions.List(f.ctx, f.userID, "2025-01-01", "2025-01-31")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty range must be a non-nil empty slice: %#v err=%v", empty, err)
	}

	var notFound *model.NotFoundError
	if got, err := f.sessions.Get(f.ctx, f.userID, first.ID); err != nil || got.ID != first.ID {
		t.Fatalf("get: %v %v", got, err)
	}
	if _, err := f.sessions.Get(f.ctx, f.userID, foreign.ID); !errors.As(err, &notFound) {
		t.Fatalf("foreign get must be not found: %v", err)
	}
	if err := f.sessions.Delete(f.ctx, f.userID, foreign.ID); !errors.As(err, &notFound) {
		t.Fatalf("foreign delete must be not found: %v", err)
	}
	if f.count(t, other) != 1 {
		t.Fatal("foreign delete must not remove the other athlete's entry")
	}

	if err := f.sessions.Delete(f.ctx, f.userID, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.Delete(f.ctx, f.userID, first.ID); !errors.As(err, &notFound) {
		t.Fatalf("repeat delete must be not found: %v", err)
	}
	if _, err := f.sessions.Get(f.ctx, f.userID, first.ID); !errors.As(err, &notFound) {
		t.Fatalf("deleted get must be not found: %v", err)
	}
}

func TestMobilitySessionConcurrentDuplicatesCreateOnce(t *testing.T) {
	f := newMobilityFixture(t)
	const workers = 8
	var wg sync.WaitGroup
	ids := make([]uuid.UUID, workers)
	outcomes := make([]model.MobilitySessionCreateOutcome, workers)
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, outcome, err := f.sessions.Create(f.ctx, f.userID, mobilityInput("2026-11-03", "race"), "race-hash")
			if err == nil {
				ids[i] = got.ID
			}
			outcomes[i], errs[i] = outcome, err
		}()
	}
	wg.Wait()
	created := 0
	for i := range workers {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Fatal("every retry must return the same entry")
		}
		if outcomes[i] == model.MobilitySessionCreated {
			created++
		}
	}
	if created != 1 || f.count(t, f.userID) != 1 {
		t.Fatalf("exactly one create expected, got %d created and %d rows", created, f.count(t, f.userID))
	}
}

func TestMobilitySessionConstraintsRejectInvalidRows(t *testing.T) {
	f := newMobilityFixture(t)
	for name, mutate := range map[string]func(*model.MobilitySession){
		"completed above total": func(s *model.MobilitySession) { s.MovesCompleted, s.MovesTotal = 9, 8 },
		"unknown context":       func(s *model.MobilitySession) { s.Context = "evening" },
		"bad slug":              func(s *model.MobilitySession) { s.RoutineID = "Hips_Open" },
		"duration":              func(s *model.MobilitySession) { s.DurationSeconds = 7201 },
	} {
		input := mobilityInput("2026-11-03", "bad-"+name)
		mutate(input)
		if _, _, err := f.sessions.Create(f.ctx, f.userID, input, "h"); err == nil {
			t.Errorf("%s must be rejected by the database", name)
		}
	}
}

func TestMobilitySessionsAreRemovedWithTheAccount(t *testing.T) {
	f := newMobilityFixture(t)
	if _, _, err := f.sessions.Create(f.ctx, f.userID, mobilityInput("2026-11-03", "acct"), "h"); err != nil {
		t.Fatal(err)
	}
	if err := NewAccountDAO(f.pool).DeleteUserData(f.ctx, f.userID); err != nil {
		t.Fatal(err)
	}
	if f.count(t, f.userID) != 0 {
		t.Fatal("account deletion must remove mobility sessions")
	}

	cascaded := f.newUser(t)
	if _, _, err := f.sessions.Create(f.ctx, cascaded, mobilityInput("2026-11-03", "cascade"), "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM auth.users WHERE id=$1`, cascaded); err != nil {
		t.Fatal(err)
	}
	if f.count(t, cascaded) != 0 {
		t.Fatal("deleting the auth identity must cascade to mobility sessions")
	}
}

// statsSnapshot captures every stats and participation read for one athlete.
func (f *mobilityFixture) statsSnapshot(t *testing.T) string {
	t.Helper()
	stats := NewStatsDAO(f.pool)
	weekStart := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	weekly, err := stats.GetWeeklyCount(f.ctx, f.userID, weekStart, weekStart.AddDate(0, 0, 13))
	if err != nil {
		t.Fatal(err)
	}
	total, err := stats.GetTotalWorkouts(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	distribution, err := stats.GetDistribution(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	streak, err := stats.GetDayStreak(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	volume, err := stats.GetWeeklyVolume(f.ctx, f.userID, weekStart)
	if err != nil {
		t.Fatal(err)
	}
	participation, err := NewParticipationDAO(f.pool).List(f.ctx, f.userID, "2026-10-26", "2026-11-08")
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"weekly": weekly, "total": total, "distribution": distribution,
		"streak": streak, "volume": volume, "participation": participation})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestMobilitySessionsNeverChangeStatsOrParticipation(t *testing.T) {
	f := newMobilityFixture(t)
	// A participated sport date and a scheduled-but-missed date.
	if _, _, err := NewSportActivityDAO(f.pool).Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-11-02",
		SportID: "running", SportName: "Running", DurationMinutes: 30, Source: model.ActivitySourceManual}, "UTC", "sport-1", "sport-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO day_participation (user_id, date, scheduled_opportunity, participated, finalized_at, timezone, local_date)
		VALUES ($1, '2026-11-01', true, false, now(), 'UTC', '2026-11-01')`, f.userID); err != nil {
		t.Fatal(err)
	}
	before := f.statsSnapshot(t)

	// Mobility on the participated date, the missed scheduled date, and a rest date.
	for i, entry := range []struct {
		date    string
		context model.MobilityContext
	}{
		{"2026-11-02", model.MobilityContextCooldown},
		{"2026-11-01", model.MobilityContextAnytime},
		{"2026-11-03", model.MobilityContextRestDay},
		{"2026-10-28", model.MobilityContextWarmup},
	} {
		input := mobilityInput(entry.date, "mob-"+entry.date)
		input.Context = entry.context
		if _, _, err := f.sessions.Create(f.ctx, f.userID, input, "h"); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	after := f.statsSnapshot(t)
	if before != after {
		t.Fatalf("mobility entries changed stats or participation:\nbefore=%s\nafter=%s", before, after)
	}
	var rows int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM day_participation WHERE user_id=$1 AND date IN ('2026-11-03', '2026-10-28')`, f.userID).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("a rest date with only mobility must have no participation row, got %d err=%v", rows, err)
	}
}
