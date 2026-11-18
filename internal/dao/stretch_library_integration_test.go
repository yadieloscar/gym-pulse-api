//go:build integration

package dao

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func stretchLibraryInput() ([]model.StretchMove, []model.StretchRoutine) {
	hold, reps, override := 45, 12, 30
	moves := []model.StretchMove{
		{ID: "my-couch", Name: "Couch stretch", Cue: "Hips forward", Areas: "Hips", Mode: model.StretchModeHold, Sided: true, HoldSeconds: &hold},
		{ID: "my-swings", Name: "Leg swings", Mode: model.StretchModeReps, Reps: &reps},
	}
	routines := []model.StretchRoutine{{ID: "my-evening", Name: "Evening", Items: []model.StretchRoutineItem{
		{MoveID: "my-couch"},
		{MoveID: "cat-cow", HoldSeconds: &override},
	}}}
	return moves, routines
}

func stretchLibraryRevision(t *testing.T, f *mobilityFixture, userID uuid.UUID) int64 {
	t.Helper()
	var revision int64
	err := f.pool.QueryRow(f.ctx, `SELECT revision FROM stretch_libraries WHERE user_id=$1`, userID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestStretchLibraryCreateReplayReplaceAndConflict(t *testing.T) {
	f := newMobilityFixture(t)
	libraries := NewStretchLibraryDAO(f.pool)
	moves, routines := stretchLibraryInput()

	empty, err := libraries.Get(f.ctx, f.userID)
	if err != nil || empty.Revision != 0 || empty.Moves == nil || len(empty.Moves) != 0 || empty.Routines == nil || len(empty.Routines) != 0 || empty.UpdatedAt != nil {
		t.Fatalf("Get before a save = %+v err=%v, want the empty library at revision 0", empty, err)
	}

	// A revision other than 0 before any save is stale, and nothing is stored.
	_, err = libraries.Save(f.ctx, f.userID, moves, routines, 2, "sl-early", "h-early")
	var conflict *model.ConflictError
	if !errors.As(err, &conflict) || conflict.Expected != 2 || conflict.Actual != 0 {
		t.Fatalf("save at revision 2 before any save = %v, want revision conflict 2 vs 0", err)
	}
	if current, ok := conflict.Authoritative.(*model.StretchLibrary); !ok || current.Revision != 0 || len(current.Moves) != 0 {
		t.Fatalf("conflict must carry the empty library, got %#v", conflict.Authoritative)
	}
	if stretchLibraryRevision(t, f, f.userID) != 0 {
		t.Fatal("a refused save stored a library")
	}

	created, err := libraries.Save(f.ctx, f.userID, moves, routines, 0, "sl-1", "h1")
	if err != nil || created.Revision != 1 || created.UpdatedAt == nil {
		t.Fatalf("create: %+v err=%v", created, err)
	}
	if len(created.Moves) != 2 || created.Moves[0].ID != "my-couch" || !created.Moves[0].Sided || created.Moves[0].HoldSeconds == nil || *created.Moves[0].HoldSeconds != 45 || created.Moves[0].Reps != nil ||
		created.Moves[1].Reps == nil || *created.Moves[1].Reps != 12 || created.Moves[1].HoldSeconds != nil {
		t.Fatalf("stored moves round trip: %+v", created.Moves)
	}
	if len(created.Routines) != 1 || len(created.Routines[0].Items) != 2 || created.Routines[0].Items[0].MoveID != "my-couch" ||
		created.Routines[0].Items[0].HoldSeconds != nil || created.Routines[0].Items[1].HoldSeconds == nil || *created.Routines[0].Items[1].HoldSeconds != 30 {
		t.Fatalf("stored routines round trip: %+v", created.Routines)
	}

	// The same key and body replays the stored answer and writes nothing.
	replay, err := libraries.Save(f.ctx, f.userID, moves, routines, 0, "sl-1", "h1")
	if err != nil || replay.Revision != 1 || replay.UpdatedAt == nil || !replay.UpdatedAt.Equal(*created.UpdatedAt) || len(replay.Moves) != 2 || len(replay.Routines) != 1 {
		t.Fatalf("replay: %+v err=%v", replay, err)
	}
	if got := stretchLibraryRevision(t, f, f.userID); got != 1 {
		t.Fatalf("revision after a replay = %d, want 1", got)
	}

	if _, err := libraries.Save(f.ctx, f.userID, moves, routines, 0, "sl-1", "other-hash"); !isIdempotencyConflict(err) {
		t.Fatalf("a different body under the same key = %v, want idempotency conflict", err)
	}

	// A stale revision is refused with the current library and changes nothing.
	_, err = libraries.Save(f.ctx, f.userID, []model.StretchMove{}, []model.StretchRoutine{}, 0, "sl-stale", "h-stale")
	conflict = nil
	if !errors.As(err, &conflict) || conflict.Expected != 0 || conflict.Actual != 1 {
		t.Fatalf("stale save = %v, want revision conflict 0 vs 1", err)
	}
	if current, ok := conflict.Authoritative.(*model.StretchLibrary); !ok || current.Revision != 1 || len(current.Moves) != 2 || len(current.Routines) != 1 {
		t.Fatalf("conflict must carry the current library, got %#v", conflict.Authoritative)
	}

	replaced, err := libraries.Save(f.ctx, f.userID, moves[:1], []model.StretchRoutine{}, 1, "sl-2", "h2")
	if err != nil || replaced.Revision != 2 || len(replaced.Moves) != 1 || replaced.Routines == nil || len(replaced.Routines) != 0 {
		t.Fatalf("replace: %+v err=%v", replaced, err)
	}
	if replaced.UpdatedAt.Before(*created.UpdatedAt) {
		t.Fatal("replace must advance updated_at")
	}

	read, err := libraries.Get(f.ctx, f.userID)
	if err != nil || read.Revision != 2 || len(read.Moves) != 1 || read.Moves[0].ID != "my-couch" || len(read.Routines) != 0 || read.UpdatedAt == nil {
		t.Fatalf("read: %+v err=%v", read, err)
	}
}

func TestStretchLibraryBelongsToOneAthlete(t *testing.T) {
	f := newMobilityFixture(t)
	libraries := NewStretchLibraryDAO(f.pool)
	moves, routines := stretchLibraryInput()
	if _, err := libraries.Save(f.ctx, f.userID, moves, routines, 0, "sl-owner", "h"); err != nil {
		t.Fatal(err)
	}

	other := f.newUser(t)
	if seen, err := libraries.Get(f.ctx, other); err != nil || seen.Revision != 0 || len(seen.Moves) != 0 || len(seen.Routines) != 0 {
		t.Fatalf("another athlete's read = %+v err=%v, want the empty library", seen, err)
	}
	// The other athlete's first save creates their own row at revision 1; the
	// same operation key is theirs alone and replays nothing of the owner's.
	theirs, err := libraries.Save(f.ctx, other, moves[1:], []model.StretchRoutine{}, 0, "sl-owner", "h-other")
	if err != nil || theirs.Revision != 1 || len(theirs.Moves) != 1 {
		t.Fatalf("another athlete's save: %+v err=%v", theirs, err)
	}
	owned, err := libraries.Get(f.ctx, f.userID)
	if err != nil || owned.Revision != 1 || len(owned.Moves) != 2 || len(owned.Routines) != 1 {
		t.Fatalf("the owner's library changed: %+v err=%v", owned, err)
	}
}

func TestStretchLibraryConcurrentFirstSavesCreateOnce(t *testing.T) {
	f := newMobilityFixture(t)
	libraries := NewStretchLibraryDAO(f.pool)
	moves, routines := stretchLibraryInput()
	const writers = 6
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = libraries.Save(f.ctx, f.userID, moves, routines, 0, "sl-race-"+uuid.NewString(), "h")
		}()
	}
	wg.Wait()
	created := 0
	for _, err := range errs {
		var conflict *model.ConflictError
		switch {
		case err == nil:
			created++
		case errors.As(err, &conflict):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != 1 || stretchLibraryRevision(t, f, f.userID) != 1 {
		t.Fatalf("created=%d revision=%d, want exactly one save at revision 1", created, stretchLibraryRevision(t, f, f.userID))
	}
}

func TestStretchLibraryTableRejectsContentThatIsNotAnObject(t *testing.T) {
	f := newMobilityFixture(t)
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO stretch_libraries (user_id, content) VALUES ($1, '[]'::jsonb)`, f.userID); err == nil {
		t.Fatal("the table must reject content that is not a JSON object")
	}
}

func TestStretchLibraryIsRemovedWithTheAccount(t *testing.T) {
	f := newMobilityFixture(t)
	libraries := NewStretchLibraryDAO(f.pool)
	moves, routines := stretchLibraryInput()
	if _, err := libraries.Save(f.ctx, f.userID, moves, routines, 0, "sl-acct", "h"); err != nil {
		t.Fatal(err)
	}
	if err := NewAccountDAO(f.pool).DeleteUserData(f.ctx, f.userID); err != nil {
		t.Fatal(err)
	}
	if stretchLibraryRevision(t, f, f.userID) != 0 {
		t.Fatal("account deletion must remove the stretch library")
	}

	cascaded := f.newUser(t)
	if _, err := libraries.Save(f.ctx, cascaded, moves, routines, 0, "sl-cascade", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM auth.users WHERE id=$1`, cascaded); err != nil {
		t.Fatal(err)
	}
	if stretchLibraryRevision(t, f, cascaded) != 0 {
		t.Fatal("deleting the auth identity must cascade to the stretch library")
	}
}

func TestStretchLibraryWorksAsRuntimeRole(t *testing.T) {
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

	libraries := NewStretchLibraryDAO(runtime)
	moves, routines := stretchLibraryInput()
	if saved, err := libraries.Save(f.ctx, f.userID, moves, routines, 0, "sl-runtime-1", "h1"); err != nil || saved.Revision != 1 {
		t.Fatalf("create as runtime role: %+v err=%v", saved, err)
	}
	if saved, err := libraries.Save(f.ctx, f.userID, moves, routines, 1, "sl-runtime-2", "h2"); err != nil || saved.Revision != 2 {
		t.Fatalf("replace as runtime role: %+v err=%v", saved, err)
	}
	if read, err := libraries.Get(f.ctx, f.userID); err != nil || read.Revision != 2 {
		t.Fatalf("read as runtime role: %+v err=%v", read, err)
	}
}
