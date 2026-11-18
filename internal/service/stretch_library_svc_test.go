package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// stretchLibraryStore is an in-memory StretchLibraryDAO that records what the
// service hands it.
type stretchLibraryStore struct {
	stored *model.StretchLibrary
	err    error
	saves  int
	key    string
	hash   string
}

func (s *stretchLibraryStore) Get(context.Context, uuid.UUID) (*model.StretchLibrary, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.stored == nil {
		return model.EmptyStretchLibrary(), nil
	}
	return s.stored, nil
}

func (s *stretchLibraryStore) Save(_ context.Context, _ uuid.UUID, moves []model.StretchMove, routines []model.StretchRoutine, expected int64, key, hash string) (*model.StretchLibrary, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.saves++
	s.key, s.hash = key, hash
	s.stored = &model.StretchLibrary{Revision: expected + 1, Moves: moves, Routines: routines}
	return s.stored, nil
}

func holdMove(id string) model.StretchMove {
	return model.StretchMove{ID: id, Name: "Couch stretch", Cue: "Hips forward", Areas: "Hips", Mode: model.StretchModeHold, Sided: true, HoldSeconds: intPtr(45)}
}

func repsMove(id string) model.StretchMove {
	return model.StretchMove{ID: id, Name: "Leg swings", Mode: model.StretchModeReps, Reps: intPtr(12)}
}

func validStretchRequest() model.SaveStretchLibraryRequest {
	return model.SaveStretchLibraryRequest{
		ExpectedRevision: 0,
		OperationKey:     "op-1",
		Moves:            []model.StretchMove{holdMove("my-couch"), repsMove("my-swings")},
		Routines: []model.StretchRoutine{{
			ID: "my-evening", Name: "Evening",
			Items: []model.StretchRoutineItem{
				{MoveID: "my-couch"},
				{MoveID: "worlds-greatest-stretch", HoldSeconds: intPtr(30)},
				{MoveID: "my-swings", Reps: intPtr(20)},
			},
		}},
	}
}

func TestStretchLibraryServiceGet(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	store := &stretchLibraryStore{}
	svc := NewStretchLibraryService(store)

	empty, err := svc.Get(ctx, userID)
	if err != nil || empty.Revision != 0 || empty.Moves == nil || len(empty.Moves) != 0 || empty.Routines == nil || len(empty.Routines) != 0 || empty.UpdatedAt != nil {
		t.Fatalf("Get() before a save = %+v, %v; want the empty library at revision 0", empty, err)
	}

	store.err = errors.New("database unavailable")
	if _, err := svc.Get(ctx, userID); err == nil {
		t.Fatal("Get() must return the DAO error")
	}
	if _, err := svc.Save(ctx, userID, validStretchRequest()); err == nil {
		t.Fatal("Save() must return the DAO error")
	}
}

func TestStretchLibraryServiceSaveStoresTheNormalizedLibrary(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	store := &stretchLibraryStore{}
	svc := NewStretchLibraryService(store)

	req := validStretchRequest()
	req.OperationKey = "  op-1  "
	req.Moves[0].Name = "  Couch stretch  "
	req.Moves[0].Cue = " Hips forward "
	req.Moves[0].Areas = " Hips "
	req.Routines[0].Name = " Evening "
	saved, err := svc.Save(ctx, userID, req)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.Revision != 1 || len(saved.Moves) != 2 || len(saved.Routines) != 1 {
		t.Fatalf("Save() = %+v, want revision 1 with 2 moves and 1 routine", saved)
	}
	if got := saved.Moves[0]; got.Name != "Couch stretch" || got.Cue != "Hips forward" || got.Areas != "Hips" {
		t.Errorf("move text was not trimmed: %+v", got)
	}
	if saved.Routines[0].Name != "Evening" {
		t.Errorf("routine name = %q, want it trimmed", saved.Routines[0].Name)
	}
	if store.key != "op-1" || store.hash == "" {
		t.Errorf("DAO got key=%q hash=%q, want the trimmed key and a request hash", store.key, store.hash)
	}

	// The same body hashes the same; a different body does not.
	first := store.hash
	if _, err := svc.Save(ctx, userID, validStretchRequest()); err != nil || store.hash != first {
		t.Errorf("an equal body hashed to %q, want %q (err=%v)", store.hash, first, err)
	}
	changed := validStretchRequest()
	changed.Moves[1].Reps = intPtr(15)
	if _, err := svc.Save(ctx, userID, changed); err != nil || store.hash == first {
		t.Errorf("a different body must hash differently (err=%v)", err)
	}

	// Empty lists are a valid library: they remove every entry.
	cleared, err := svc.Save(ctx, userID, model.SaveStretchLibraryRequest{ExpectedRevision: 3, OperationKey: "clear", Moves: []model.StretchMove{}, Routines: []model.StretchRoutine{}})
	if err != nil || cleared.Revision != 4 || len(cleared.Moves) != 0 || len(cleared.Routines) != 0 {
		t.Errorf("Save(empty lists) = %+v, %v; want an empty library at revision 4", cleared, err)
	}
}

func TestStretchLibraryServiceSaveAcceptsTheLimits(t *testing.T) {
	req := model.SaveStretchLibraryRequest{OperationKey: strings.Repeat("k", 128)}
	for i := range model.MaxStretchMoves {
		move := holdMove(fmt.Sprintf("my-move-%d", i))
		move.Name = strings.Repeat("é", model.MaxStretchNameLength)
		move.Cue = strings.Repeat("c", model.MaxStretchCueLength)
		move.Areas = strings.Repeat("a", model.MaxStretchAreasLength)
		move.HoldSeconds = intPtr(model.MaxStretchHoldSeconds)
		req.Moves = append(req.Moves, move)
	}
	req.Moves[0].ID = "my-" + strings.Repeat("x", 61)
	req.Moves[1].ID = "my-1"
	req.Moves[1].HoldSeconds = intPtr(model.MinStretchHoldSeconds)
	req.Moves[2] = repsMove("my-reps-low")
	req.Moves[3] = repsMove("my-reps-high")
	req.Moves[3].Reps = intPtr(model.MaxStretchReps)
	for i := range model.MaxStretchRoutines {
		routine := model.StretchRoutine{ID: fmt.Sprintf("my-routine-%d", i), Name: "R"}
		for range model.MaxStretchRoutineItems {
			routine.Items = append(routine.Items, model.StretchRoutineItem{MoveID: "my-1", HoldSeconds: intPtr(model.MaxStretchHoldSeconds)})
		}
		req.Routines = append(req.Routines, routine)
	}
	req.Routines[0].Items[0] = model.StretchRoutineItem{MoveID: strings.Repeat("b", 64), Reps: intPtr(model.MaxStretchReps)}
	req.Routines[0].Items[1] = model.StretchRoutineItem{MoveID: "b", Reps: intPtr(model.MinStretchReps)}
	req.Routines[0].Items[2] = model.StretchRoutineItem{MoveID: "my-1", HoldSeconds: intPtr(model.MinStretchHoldSeconds)}

	store := &stretchLibraryStore{}
	if _, err := NewStretchLibraryService(store).Save(context.Background(), uuid.New(), req); err != nil {
		t.Fatalf("Save() at every limit error = %v", err)
	}
}

func TestStretchLibraryServiceSaveRejectsInvalidRequests(t *testing.T) {
	tooManyMoves := func(r *model.SaveStretchLibraryRequest) {
		r.Moves = nil
		for i := range model.MaxStretchMoves + 1 {
			r.Moves = append(r.Moves, holdMove(fmt.Sprintf("my-move-%d", i)))
		}
		r.Routines = []model.StretchRoutine{}
	}
	tooManyRoutines := func(r *model.SaveStretchLibraryRequest) {
		r.Routines = nil
		for i := range model.MaxStretchRoutines + 1 {
			r.Routines = append(r.Routines, model.StretchRoutine{ID: fmt.Sprintf("my-routine-%d", i), Name: "R", Items: []model.StretchRoutineItem{{MoveID: "cat-cow"}}})
		}
	}
	tooManyItems := func(r *model.SaveStretchLibraryRequest) {
		r.Routines[0].Items = make([]model.StretchRoutineItem, model.MaxStretchRoutineItems+1)
		for i := range r.Routines[0].Items {
			r.Routines[0].Items[i] = model.StretchRoutineItem{MoveID: "cat-cow"}
		}
	}
	tests := []struct {
		name   string
		mutate func(*model.SaveStretchLibraryRequest)
		field  string
		// index is the position of the entry at fault, -1 when the error is
		// about the list or the request itself.
		index int
	}{
		{"operation key missing", func(r *model.SaveStretchLibraryRequest) { r.OperationKey = "  " }, "operation_key", -1},
		{"operation key too long", func(r *model.SaveStretchLibraryRequest) { r.OperationKey = strings.Repeat("k", 129) }, "operation_key", -1},
		{"negative expected revision", func(r *model.SaveStretchLibraryRequest) { r.ExpectedRevision = -1 }, "expected_revision", -1},

		{"moves missing", func(r *model.SaveStretchLibraryRequest) { r.Moves = nil }, "moves", -1},
		{"too many moves", tooManyMoves, "moves", -1},
		{"move id not a slug", func(r *model.SaveStretchLibraryRequest) { r.Moves[1].ID = "my-Leg Swings" }, "moves", 1},
		{"move id without the prefix", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].ID = "couch-stretch" }, "moves", 0},
		{"move id too short", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].ID = "my-" }, "moves", 0},
		{"move id too long", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].ID = "my-" + strings.Repeat("x", 62) }, "moves", 0},
		{"move id repeated", func(r *model.SaveStretchLibraryRequest) { r.Moves[1].ID = r.Moves[0].ID }, "moves", 1},
		{"move name blank", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].Name = "   " }, "moves", 0},
		{"move name too long", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].Name = strings.Repeat("n", 61) }, "moves", 0},
		{"move cue too long", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].Cue = strings.Repeat("c", 201) }, "moves", 0},
		{"move areas too long", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].Areas = strings.Repeat("a", 61) }, "moves", 0},
		{"move mode unknown", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].Mode = "flow" }, "moves", 0},
		{"move mode missing", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].Mode = "" }, "moves", 0},
		{"hold move without hold_seconds", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].HoldSeconds = nil }, "moves", 0},
		{"hold move below 5 seconds", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].HoldSeconds = intPtr(4) }, "moves", 0},
		{"hold move above 600 seconds", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].HoldSeconds = intPtr(601) }, "moves", 0},
		{"hold move with reps", func(r *model.SaveStretchLibraryRequest) { r.Moves[0].Reps = intPtr(5) }, "moves", 0},
		{"reps move without reps", func(r *model.SaveStretchLibraryRequest) { r.Moves[1].Reps = nil }, "moves", 1},
		{"reps move with 0 reps", func(r *model.SaveStretchLibraryRequest) { r.Moves[1].Reps = intPtr(0) }, "moves", 1},
		{"reps move above 100 reps", func(r *model.SaveStretchLibraryRequest) { r.Moves[1].Reps = intPtr(101) }, "moves", 1},
		{"reps move with hold_seconds", func(r *model.SaveStretchLibraryRequest) { r.Moves[1].HoldSeconds = intPtr(30) }, "moves", 1},

		{"routines missing", func(r *model.SaveStretchLibraryRequest) { r.Routines = nil }, "routines", -1},
		{"too many routines", tooManyRoutines, "routines", -1},
		{"routine id not a slug", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].ID = "my_evening" }, "routines", 0},
		{"routine id without the prefix", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].ID = "evening" }, "routines", 0},
		{"routine id too short", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].ID = "my" }, "routines", 0},
		{"routine id too long", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].ID = "my-" + strings.Repeat("x", 62) }, "routines", 0},
		{"routine id repeated", func(r *model.SaveStretchLibraryRequest) {
			r.Routines = append(r.Routines, r.Routines[0])
		}, "routines", 1},
		{"routine name blank", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Name = "" }, "routines", 0},
		{"routine name too long", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Name = strings.Repeat("n", 61) }, "routines", 0},
		{"routine without items", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items = nil }, "routines", 0},
		{"routine with too many items", tooManyItems, "routines", 0},
		{"item move_id empty", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items[1].MoveID = "" }, "routines", 0},
		{"item move_id not a slug", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items[1].MoveID = "Cat Cow" }, "routines", 0},
		{"item move_id too long", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items[1].MoveID = strings.Repeat("b", 65) }, "routines", 0},
		{"item names an own move that is not in moves", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items[0].MoveID = "my-gone" }, "routines", 0},
		{"item sets both doses", func(r *model.SaveStretchLibraryRequest) {
			r.Routines[0].Items[1].Reps = intPtr(5)
		}, "routines", 0},
		{"item hold below 5 seconds", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items[1].HoldSeconds = intPtr(4) }, "routines", 0},
		{"item hold above 600 seconds", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items[1].HoldSeconds = intPtr(601) }, "routines", 0},
		{"item with 0 reps", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items[2].Reps = intPtr(0) }, "routines", 0},
		{"item above 100 reps", func(r *model.SaveStretchLibraryRequest) { r.Routines[0].Items[2].Reps = intPtr(101) }, "routines", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stretchLibraryStore{}
			req := validStretchRequest()
			tt.mutate(&req)
			_, err := NewStretchLibraryService(store).Save(context.Background(), uuid.New(), req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("Save() error = %v, want a validation error", err)
			}
			if validation.Field != tt.field {
				t.Errorf("field = %q, want %q (%s)", validation.Field, tt.field, validation.Message)
			}
			switch {
			case tt.index < 0 && validation.Index != nil:
				t.Errorf("index = %d, want none", *validation.Index)
			case tt.index >= 0 && (validation.Index == nil || *validation.Index != tt.index):
				t.Errorf("index = %v, want %d", validation.Index, tt.index)
			}
			if store.saves != 0 {
				t.Error("a rejected request reached the DAO")
			}
		})
	}
}
