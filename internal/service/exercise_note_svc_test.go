package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// exerciseNoteStore is an in-memory ExerciseNoteDAO keyed by athlete, so the
// tests can show that one athlete's notes never reach another.
type exerciseNoteStore struct {
	notes map[uuid.UUID]map[string]model.ExerciseNote
	limit int
	err   error
}

var exerciseNoteSavedAt = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func (s *exerciseNoteStore) List(_ context.Context, userID uuid.UUID) ([]model.ExerciseNote, error) {
	if s.err != nil {
		return nil, s.err
	}
	notes := []model.ExerciseNote{}
	for _, note := range s.notes[userID] {
		notes = append(notes, note)
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].ExerciseKey < notes[j].ExerciseKey })
	return notes, nil
}

func (s *exerciseNoteStore) Save(_ context.Context, userID uuid.UUID, note model.ExerciseNote) (*model.ExerciseNote, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.notes == nil {
		s.notes = map[uuid.UUID]map[string]model.ExerciseNote{}
	}
	if s.notes[userID] == nil {
		s.notes[userID] = map[string]model.ExerciseNote{}
	}
	if _, ok := s.notes[userID][note.ExerciseKey]; !ok && s.limit > 0 && len(s.notes[userID]) >= s.limit {
		return nil, dao.ErrExerciseNoteLimit
	}
	note.UpdatedAt = exerciseNoteSavedAt
	s.notes[userID][note.ExerciseKey] = note
	return &note, nil
}

func (s *exerciseNoteStore) Delete(_ context.Context, userID uuid.UUID, exerciseKey string) error {
	if s.err != nil {
		return s.err
	}
	delete(s.notes[userID], exerciseKey)
	return nil
}

func newExerciseNoteService(store *exerciseNoteStore) *exerciseNoteService {
	return &exerciseNoteService{notes: store, now: func() time.Time { return exerciseNoteSavedAt }}
}

func TestExerciseNoteSave(t *testing.T) {
	athlete := uuid.New()
	long := strings.Repeat("é", model.MaxExerciseNoteLength)
	tests := []struct {
		name      string
		req       model.SaveExerciseNoteRequest
		want      model.ExerciseNote
		wantField string
	}{
		{name: "saves a note", req: model.SaveExerciseNoteRequest{ExerciseName: "Leg press", Note: "seat height 4"},
			want: model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "Leg press", Note: "seat height 4", UpdatedAt: exerciseNoteSavedAt}},
		{name: "trims the note and cleans the name", req: model.SaveExerciseNoteRequest{ExerciseName: "  Leg   PRESS ", Note: "  narrow grip\n"},
			want: model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "Leg PRESS", Note: "narrow grip", UpdatedAt: exerciseNoteSavedAt}},
		{name: "keeps a line break inside the note", req: model.SaveExerciseNoteRequest{ExerciseName: "Row", Note: "seat 4\npad 2"},
			want: model.ExerciseNote{ExerciseKey: "row", ExerciseName: "Row", Note: "seat 4\npad 2", UpdatedAt: exerciseNoteSavedAt}},
		{name: "accepts 200 characters, counted as characters", req: model.SaveExerciseNoteRequest{ExerciseName: "Row", Note: long},
			want: model.ExerciseNote{ExerciseKey: "row", ExerciseName: "Row", Note: long, UpdatedAt: exerciseNoteSavedAt}},
		{name: "accepts a 120 character name", req: model.SaveExerciseNoteRequest{ExerciseName: strings.Repeat("a", 120), Note: "x"},
			want: model.ExerciseNote{ExerciseKey: strings.Repeat("a", 120), ExerciseName: strings.Repeat("a", 120), Note: "x", UpdatedAt: exerciseNoteSavedAt}},
		{name: "refuses 201 characters", req: model.SaveExerciseNoteRequest{ExerciseName: "Row", Note: long + "é"}, wantField: "note"},
		{name: "refuses a missing name", req: model.SaveExerciseNoteRequest{Note: "seat height 4"}, wantField: "exercise_name"},
		{name: "refuses a blank name", req: model.SaveExerciseNoteRequest{ExerciseName: " \t", Note: "seat height 4"}, wantField: "exercise_name"},
		{name: "refuses a blank name even when clearing", req: model.SaveExerciseNoteRequest{ExerciseName: "", Note: ""}, wantField: "exercise_name"},
		{name: "refuses a 121 character name", req: model.SaveExerciseNoteRequest{ExerciseName: strings.Repeat("a", 121), Note: "x"}, wantField: "exercise_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &exerciseNoteStore{}
			got, err := newExerciseNoteService(store).Save(context.Background(), athlete, tt.req)
			if tt.wantField != "" {
				var validation *model.ValidationError
				if !errors.As(err, &validation) || validation.Field != tt.wantField {
					t.Fatalf("Save() error = %v, want a validation error on %q", err, tt.wantField)
				}
				if len(store.notes[athlete]) != 0 {
					t.Fatalf("a refused save stored %v", store.notes[athlete])
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if *got != tt.want {
				t.Errorf("Save() = %+v, want %+v", *got, tt.want)
			}
			if stored := store.notes[athlete][tt.want.ExerciseKey]; stored != tt.want {
				t.Errorf("stored %+v, want %+v", stored, tt.want)
			}
		})
	}
}

func TestExerciseNoteEditReplacesAndRepeatsSafely(t *testing.T) {
	athlete := uuid.New()
	store := &exerciseNoteStore{}
	svc := newExerciseNoteService(store)
	ctx := context.Background()

	for _, req := range []model.SaveExerciseNoteRequest{
		{ExerciseName: "Leg press", Note: "seat height 4"},
		{ExerciseName: "leg PRESS", Note: "seat height 5"},
		{ExerciseName: "leg PRESS", Note: "seat height 5"}, // the same save replayed from a queue
	} {
		if _, err := svc.Save(ctx, athlete, req); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := svc.List(ctx, athlete)
	if err != nil {
		t.Fatal(err)
	}
	want := model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "leg PRESS", Note: "seat height 5", UpdatedAt: exerciseNoteSavedAt}
	if len(listed.ExerciseNotes) != 1 || listed.ExerciseNotes[0] != want {
		t.Fatalf("List() = %+v, want only %+v", listed.ExerciseNotes, want)
	}
}

func TestExerciseNoteClear(t *testing.T) {
	athlete := uuid.New()
	store := &exerciseNoteStore{}
	svc := newExerciseNoteService(store)
	ctx := context.Background()
	if _, err := svc.Save(ctx, athlete, model.SaveExerciseNoteRequest{ExerciseName: "Leg press", Note: "seat height 4"}); err != nil {
		t.Fatal(err)
	}
	want := model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "Leg Press", Note: "", UpdatedAt: exerciseNoteSavedAt}
	// The second clear finds no note and answers the same way.
	for range 2 {
		got, err := svc.Save(ctx, athlete, model.SaveExerciseNoteRequest{ExerciseName: "Leg Press", Note: "  \n"})
		if err != nil {
			t.Fatal(err)
		}
		if *got != want {
			t.Fatalf("clear = %+v, want %+v", *got, want)
		}
	}
	listed, err := svc.List(ctx, athlete)
	if err != nil || listed.ExerciseNotes == nil || len(listed.ExerciseNotes) != 0 {
		t.Fatalf("List() = %+v, err = %v; want a non-nil empty list", listed, err)
	}
}

func TestExerciseNoteList(t *testing.T) {
	athlete, other := uuid.New(), uuid.New()
	store := &exerciseNoteStore{}
	svc := newExerciseNoteService(store)
	ctx := context.Background()

	empty, err := svc.List(ctx, athlete)
	if err != nil || empty.ExerciseNotes == nil || len(empty.ExerciseNotes) != 0 {
		t.Fatalf("empty List() = %+v, err = %v; want a non-nil empty list", empty, err)
	}
	for _, req := range []model.SaveExerciseNoteRequest{
		{ExerciseName: "Squat", Note: "heels on plates"},
		{ExerciseName: "Bench press", Note: "narrow grip"},
	} {
		if _, err := svc.Save(ctx, athlete, req); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Save(ctx, other, model.SaveExerciseNoteRequest{ExerciseName: "Squat", Note: "theirs"}); err != nil {
		t.Fatal(err)
	}
	listed, err := svc.List(ctx, athlete)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.ExerciseNotes) != 2 || listed.ExerciseNotes[0].ExerciseKey != "bench press" || listed.ExerciseNotes[1].Note != "heels on plates" {
		t.Fatalf("List() = %+v, want the athlete's two notes ordered by key", listed.ExerciseNotes)
	}
}

func TestExerciseNoteLimit(t *testing.T) {
	athlete := uuid.New()
	store := &exerciseNoteStore{limit: 2}
	svc := newExerciseNoteService(store)
	ctx := context.Background()
	for _, name := range []string{"Squat", "Row"} {
		if _, err := svc.Save(ctx, athlete, model.SaveExerciseNoteRequest{ExerciseName: name, Note: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := svc.Save(ctx, athlete, model.SaveExerciseNoteRequest{ExerciseName: "Curl", Note: "x"})
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "exercise_name" {
		t.Fatalf("Save() past the limit error = %v, want a validation error on exercise_name", err)
	}
	if _, err := svc.Save(ctx, athlete, model.SaveExerciseNoteRequest{ExerciseName: "squat", Note: "edited"}); err != nil {
		t.Fatalf("editing at the limit: %v", err)
	}
	if _, err := svc.Save(ctx, athlete, model.SaveExerciseNoteRequest{ExerciseName: "Row", Note: ""}); err != nil {
		t.Fatalf("clearing at the limit: %v", err)
	}
}

func TestExerciseNoteStoreErrorsPassThrough(t *testing.T) {
	store := &exerciseNoteStore{err: context.DeadlineExceeded}
	svc := NewExerciseNoteService(store)
	ctx := context.Background()
	if _, err := svc.List(ctx, uuid.New()); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("List() error = %v", err)
	}
	if _, err := svc.Save(ctx, uuid.New(), model.SaveExerciseNoteRequest{ExerciseName: "Row", Note: "x"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Save() error = %v", err)
	}
	if _, err := svc.Save(ctx, uuid.New(), model.SaveExerciseNoteRequest{ExerciseName: "Row", Note: ""}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("clearing Save() error = %v", err)
	}
}
