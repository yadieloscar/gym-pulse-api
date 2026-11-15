//go:build integration

package dao

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func kneeKitInput(date string, items ...string) *model.KneeKitDay {
	if items == nil {
		items = []string{}
	}
	return &model.KneeKitDay{Date: date, CompletedItems: items}
}

func kneeKitCount(t *testing.T, f *mobilityFixture, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM knee_kit_days WHERE user_id=$1`, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestKneeKitDayCreateReplayReplaceConflictAndRange(t *testing.T) {
	f := newMobilityFixture(t)
	days := NewKneeKitDAO(f.pool)
	pain := 3

	input := kneeKitInput("2026-11-02", "isometric-hold", "morning-check")
	input.MorningPain = &pain
	first, outcome, err := days.SaveDay(f.ctx, f.userID, input, 0, "knee-1", "hash-1")
	if err != nil || outcome != model.KneeKitDayCreated {
		t.Fatalf("create: outcome=%v err=%v", outcome, err)
	}
	if first.ID == uuid.Nil || first.Revision != 1 || first.Date != "2026-11-02" || first.CreatedAt.IsZero() || !first.UpdatedAt.Equal(first.CreatedAt) ||
		!slices.Equal(first.CompletedItems, []string{"isometric-hold", "morning-check"}) || first.MorningPain == nil || *first.MorningPain != 3 ||
		first.ActivityPain != nil || first.SharpPain || first.Limping {
		t.Fatalf("stored day wrong: %+v", first)
	}

	// A verbatim retry replays the original result even though the stored
	// revision no longer matches the retry's expected_revision of 0.
	replay, outcome, err := days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-02", "isometric-hold", "morning-check"), 0, "knee-1", "hash-1")
	if err != nil || outcome != model.KneeKitDayCreated || replay.ID != first.ID || replay.Revision != 1 || !replay.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("replay: %+v outcome=%v err=%v", replay, outcome, err)
	}
	var conflict *model.ConflictError
	if _, _, err = days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-02"), 1, "knee-1", "hash-other"); !errors.As(err, &conflict) || !strings.Contains(conflict.Message, "idempotency") {
		t.Fatalf("same key with a different payload must be an idempotency conflict: %v", err)
	}

	// A stale revision conflicts and carries the stored day.
	_, _, err = days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-02"), 0, "knee-stale", "hash-stale")
	if !errors.As(err, &conflict) || conflict.Message != "resource revision conflict" || conflict.Expected != 0 || conflict.Actual != 1 {
		t.Fatalf("stale revision must conflict: %v", err)
	}
	if stored, ok := conflict.Authoritative.(*model.KneeKitDay); !ok || stored.ID != first.ID || stored.Revision != 1 {
		t.Fatalf("the conflict must carry the stored day: %#v", conflict.Authoritative)
	}
	// A nonzero revision against a missing day conflicts without a resource.
	_, _, err = days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-01"), 2, "knee-missing", "hash-missing")
	if !errors.As(err, &conflict) || conflict.Actual != 0 || conflict.Authoritative != nil {
		t.Fatalf("missing-row revision conflict: %v", err)
	}
	if kneeKitCount(t, f, f.userID) != 1 {
		t.Fatal("replays and conflicts must not write")
	}

	// Full replacement: unticking everything and clearing pain still stores
	// the day (there is no delete-by-empty) and bumps the revision.
	cleared := kneeKitInput("2026-11-02")
	cleared.SharpPain, cleared.Limping = true, true
	replaced, outcome, err := days.SaveDay(f.ctx, f.userID, cleared, 1, "knee-2", "hash-2")
	if err != nil || outcome != model.KneeKitDayReplaced {
		t.Fatalf("replace: outcome=%v err=%v", outcome, err)
	}
	if replaced.ID != first.ID || replaced.Revision != 2 || replaced.CompletedItems == nil || len(replaced.CompletedItems) != 0 ||
		replaced.MorningPain != nil || !replaced.SharpPain || !replaced.Limping || !replaced.CreatedAt.Equal(first.CreatedAt) || replaced.UpdatedAt.Before(first.UpdatedAt) {
		t.Fatalf("replaced day wrong: %+v", replaced)
	}
	// The earlier key still replays its original response.
	if again, _, err := days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-02"), 0, "knee-1", "hash-1"); err != nil || again.Revision != 1 || len(again.CompletedItems) != 2 {
		t.Fatalf("an old key must replay its original result: %+v err=%v", again, err)
	}

	// Another athlete may reuse the key and the date independently.
	other := f.newUser(t)
	if _, outcome, err := days.SaveDay(f.ctx, other, kneeKitInput("2026-11-02", "wall-sit"), 0, "knee-1", "hash-foreign"); err != nil || outcome != model.KneeKitDayCreated {
		t.Fatalf("foreign create: outcome=%v err=%v", outcome, err)
	}
	// A foreign revision is invisible: the owner's own day decides.
	if _, _, err := days.SaveDay(f.ctx, other, kneeKitInput("2026-11-02"), 2, "knee-foreign-rev", "h"); !errors.As(err, &conflict) || conflict.Actual != 1 {
		t.Fatalf("revisions are per owner: %v", err)
	}

	if _, _, err := days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-10-30", "morning-check"), 0, "knee-3", "hash-3"); err != nil {
		t.Fatal(err)
	}
	listed, err := days.ListDays(f.ctx, f.userID, "2026-10-01", "2026-11-30")
	if err != nil || len(listed) != 2 || listed[0].Date != "2026-10-30" || listed[1].Date != "2026-11-02" || listed[1].Revision != 2 {
		t.Fatalf("range must be the owner's days oldest first: %+v err=%v", listed, err)
	}
	single, err := days.ListDays(f.ctx, f.userID, "2026-11-02", "2026-11-02")
	if err != nil || len(single) != 1 || single[0].ID != first.ID || single[0].CompletedItems == nil {
		t.Fatalf("inclusive single-day range: %+v err=%v", single, err)
	}
	empty, err := days.ListDays(f.ctx, f.userID, "2025-01-01", "2025-01-31")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("an empty range must be a non-nil empty slice: %#v err=%v", empty, err)
	}
}

func TestKneeKitDayConcurrentFirstSavesCreateOnce(t *testing.T) {
	f := newMobilityFixture(t)
	days := NewKneeKitDAO(f.pool)
	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	outcomes := make([]model.KneeKitDayOutcome, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Distinct keys, all believing no record exists.
			_, outcomes[i], errs[i] = days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-03", "morning-check"), 0, "race-"+string(rune('a'+i)), "h")
		}()
	}
	wg.Wait()
	created := 0
	for i := range workers {
		var conflict *model.ConflictError
		switch {
		case errs[i] == nil && outcomes[i] == model.KneeKitDayCreated:
			created++
		case errors.As(errs[i], &conflict) && conflict.Actual == 1:
		default:
			t.Fatalf("worker %d: outcome=%v err=%v", i, outcomes[i], errs[i])
		}
	}
	if created != 1 || kneeKitCount(t, f, f.userID) != 1 {
		t.Fatalf("exactly one create expected, got %d created and %d rows", created, kneeKitCount(t, f, f.userID))
	}
}

func TestKneeKitDayConstraintsRejectInvalidRows(t *testing.T) {
	f := newMobilityFixture(t)
	insert := func(date string, items any, morning, activity any) error {
		_, err := f.pool.Exec(f.ctx, `INSERT INTO knee_kit_days (id, user_id, date, completed_items, morning_pain, activity_pain)
			VALUES ($1, $2, $3, $4, $5, $6)`, uuid.New(), f.userID, date, items, morning, activity)
		return err
	}
	thirteen := make([]string, 13)
	for i := range thirteen {
		thirteen[i] = "item-" + string(rune('a'+i))
	}
	for name, row := range map[string]struct {
		items             any
		morning, activity any
	}{
		"thirteen items":     {items: thirteen},
		"uppercase slug":     {items: []string{"Morning-Check"}},
		"underscore slug":    {items: []string{"morning_check"}},
		"trailing hyphen":    {items: []string{"morning-"}},
		"empty item":         {items: []string{""}},
		"empty among others": {items: []string{"morning-check", ""}},
		"comma in item":      {items: []string{"morning-check,"}},
		"long item":          {items: []string{strings.Repeat("a", 65)}},
		"null item":          {items: []*string{nil}},
		"null array":         {items: nil},
		"morning pain 11":    {items: []string{}, morning: 11},
		"morning pain -1":    {items: []string{}, morning: -1},
		"activity pain 11":   {items: []string{}, activity: 11},
	} {
		if err := insert("2026-11-03", row.items, row.morning, row.activity); err == nil {
			t.Errorf("%s must be rejected by the database", name)
		}
	}
	// The bounds themselves are accepted.
	twelve := thirteen[:12]
	twelve[0] = strings.Repeat("a", 64)
	if err := insert("2026-11-03", twelve, 0, 10); err != nil {
		t.Fatalf("boundary row must be accepted: %v", err)
	}
	if err := insert("2026-11-03", []string{}, nil, nil); err == nil {
		t.Fatal("a second row for the same athlete and date must violate the unique index")
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE knee_kit_days SET revision=0 WHERE user_id=$1`, f.userID); err == nil {
		t.Fatal("revision must stay positive")
	}
}

func TestKneeKitDaysAreRemovedWithTheAccount(t *testing.T) {
	f := newMobilityFixture(t)
	days := NewKneeKitDAO(f.pool)
	if _, _, err := days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-03", "wall-sit"), 0, "acct", "h"); err != nil {
		t.Fatal(err)
	}
	if err := NewAccountDAO(f.pool).DeleteUserData(f.ctx, f.userID); err != nil {
		t.Fatal(err)
	}
	if kneeKitCount(t, f, f.userID) != 0 {
		t.Fatal("account deletion must remove knee kit days")
	}

	cascaded := f.newUser(t)
	if _, _, err := days.SaveDay(f.ctx, cascaded, kneeKitInput("2026-11-03"), 0, "cascade", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM auth.users WHERE id=$1`, cascaded); err != nil {
		t.Fatal(err)
	}
	if kneeKitCount(t, f, cascaded) != 0 {
		t.Fatal("deleting the auth identity must cascade to knee kit days")
	}
}

func TestKneeKitDaysNeverChangeStatsOrParticipation(t *testing.T) {
	f := newMobilityFixture(t)
	if _, _, err := NewSportActivityDAO(f.pool).Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-11-02",
		SportID: "running", SportName: "Running", DurationMinutes: 30, Source: model.ActivitySourceManual}, "UTC", "sport-1", "sport-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO day_participation (user_id, date, scheduled_opportunity, participated, finalized_at, timezone, local_date)
		VALUES ($1, '2026-11-01', true, false, now(), 'UTC', '2026-11-01')`, f.userID); err != nil {
		t.Fatal(err)
	}
	before := f.statsSnapshot(t)

	// Knee days on the participated date, the missed scheduled date, and rest dates.
	days := NewKneeKitDAO(f.pool)
	pain := 6
	for _, date := range []string{"2026-11-02", "2026-11-01", "2026-11-03", "2026-10-28"} {
		input := kneeKitInput(date, "isometric-hold", "morning-check")
		input.ActivityPain, input.SharpPain = &pain, true
		if _, _, err := days.SaveDay(f.ctx, f.userID, input, 0, "knee-"+date, "h"); err != nil {
			t.Fatalf("%s: %v", date, err)
		}
	}
	if after := f.statsSnapshot(t); before != after {
		t.Fatalf("knee kit days changed stats or participation:\nbefore=%s\nafter=%s", before, after)
	}
	var rows int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM day_participation WHERE user_id=$1 AND date IN ('2026-11-03', '2026-10-28')`, f.userID).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("a rest date with only a knee kit day must have no participation row, got %d err=%v", rows, err)
	}
}

// The deployed API connects as the least-privilege runtime role, which has no
// owner bypass. Saves and reads must work through its grant and RLS policy.
func TestKneeKitWorksAsRuntimeRole(t *testing.T) {
	f := newMobilityFixture(t)
	var canAssume bool
	if err := f.pool.QueryRow(f.ctx, `SELECT pg_has_role(current_user, 'gym_pulse_api', 'MEMBER')`).Scan(&canAssume); err != nil {
		t.Fatal(err)
	}
	if !canAssume {
		t.Skip("test database user cannot assume gym_pulse_api")
	}
	config := f.pool.Config().Copy()
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET ROLE gym_pulse_api`)
		return err
	}
	runtime, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)

	days := NewKneeKitDAO(runtime)
	if _, outcome, err := days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-03", "wall-sit"), 0, "runtime-1", "h1"); err != nil || outcome != model.KneeKitDayCreated {
		t.Fatalf("create as runtime role: outcome=%v err=%v", outcome, err)
	}
	if _, outcome, err := days.SaveDay(f.ctx, f.userID, kneeKitInput("2026-11-03"), 1, "runtime-2", "h2"); err != nil || outcome != model.KneeKitDayReplaced {
		t.Fatalf("replace as runtime role: outcome=%v err=%v", outcome, err)
	}
	if listed, err := days.ListDays(f.ctx, f.userID, "2026-11-01", "2026-11-30"); err != nil || len(listed) != 1 || listed[0].Revision != 2 {
		t.Fatalf("list as runtime role: %+v err=%v", listed, err)
	}
}
