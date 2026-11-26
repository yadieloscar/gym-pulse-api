//go:build integration

package dao

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestExerciseNoteSaveEditClear(t *testing.T) {
	f := newReleaseFixture(t)
	notes := NewExerciseNoteDAO(f.pool)

	empty, err := notes.List(f.ctx, f.userID)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %#v, err = %v; want a non-nil empty list", empty, err)
	}
	first, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "Leg press", Note: "seat height 4"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ExerciseKey != "leg press" || first.ExerciseName != "Leg press" || first.Note != "seat height 4" || first.UpdatedAt.IsZero() {
		t.Fatalf("first save = %+v", first)
	}
	// The same save replayed from an offline queue leaves one note.
	if _, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "Leg press", Note: "seat height 4"}); err != nil {
		t.Fatal(err)
	}
	edited, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "LEG press", Note: "seat height 5"})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Note != "seat height 5" || edited.ExerciseName != "LEG press" || edited.UpdatedAt.Before(first.UpdatedAt) {
		t.Fatalf("edit = %+v after %+v", edited, first)
	}
	if _, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "bench press", ExerciseName: "Bench press", Note: "narrow grip"}); err != nil {
		t.Fatal(err)
	}
	listed, err := notes.List(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ExerciseKey != "bench press" || listed[1].Note != "seat height 5" {
		t.Fatalf("listed = %+v, want bench press then the edited leg press", listed)
	}

	for range 2 { // clearing a note that is already gone is not an error
		if err := notes.Delete(f.ctx, f.userID, "leg press"); err != nil {
			t.Fatal(err)
		}
	}
	after, err := notes.List(f.ctx, f.userID)
	if err != nil || len(after) != 1 || after[0].ExerciseKey != "bench press" {
		t.Fatalf("after clear = %+v, err = %v; want only bench press", after, err)
	}
}

func TestExerciseNotesArePrivateToTheirOwner(t *testing.T) {
	f := newReleaseFixture(t)
	notes := NewExerciseNoteDAO(f.pool)
	other := uuid.New()
	f.exec(t, `INSERT INTO auth.users (id) VALUES ($1)`, other)
	t.Cleanup(func() {
		if _, err := f.pool.Exec(f.cleanupCtx, `DELETE FROM auth.users WHERE id=$1`, other); err != nil {
			t.Errorf("cleaning the second athlete: %v", err)
		}
	})

	if _, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "Leg press", Note: "seat height 4"}); err != nil {
		t.Fatal(err)
	}
	theirs, err := notes.List(f.ctx, other)
	if err != nil || len(theirs) != 0 {
		t.Fatalf("another athlete listed %+v, err = %v; want nothing", theirs, err)
	}
	// The same exercise name is a separate note for the other athlete, and
	// their edit and clear leave the first athlete's note alone.
	if _, err := notes.Save(f.ctx, other, model.ExerciseNote{ExerciseKey: "leg press", ExerciseName: "Leg press", Note: "theirs"}); err != nil {
		t.Fatal(err)
	}
	if err := notes.Delete(f.ctx, other, "leg press"); err != nil {
		t.Fatal(err)
	}
	mine, err := notes.List(f.ctx, f.userID)
	if err != nil || len(mine) != 1 || mine[0].Note != "seat height 4" {
		t.Fatalf("my notes after their save and clear = %+v, err = %v", mine, err)
	}

	// Deleting the account removes the notes with it.
	if _, err := notes.Save(f.ctx, other, model.ExerciseNote{ExerciseKey: "row", ExerciseName: "Row", Note: "theirs"}); err != nil {
		t.Fatal(err)
	}
	if err := NewAccountDAO(f.pool).DeleteUserData(f.ctx, other); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM exercise_notes WHERE user_id=$1`, other).Scan(&left); err != nil || left != 0 {
		t.Fatalf("notes left after account deletion = %d, err = %v", left, err)
	}
}

func TestExerciseNoteLimit(t *testing.T) {
	f := newReleaseFixture(t)
	notes := NewExerciseNoteDAO(f.pool)
	f.exec(t, `INSERT INTO exercise_notes (user_id, exercise_key, exercise_name, note)
		SELECT $1, 'exercise ' || n, 'Exercise ' || n, 'x' FROM generate_series(1, $2::int) AS n`, f.userID, model.MaxExerciseNotes)

	_, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "one more", ExerciseName: "One more", Note: "x"})
	if !errors.Is(err, ErrExerciseNoteLimit) {
		t.Fatalf("save past the limit error = %v, want ErrExerciseNoteLimit", err)
	}
	if _, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "exercise 1", ExerciseName: "Exercise 1", Note: "edited"}); err != nil {
		t.Fatalf("editing at the limit: %v", err)
	}
	if err := notes.Delete(f.ctx, f.userID, "exercise 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "one more", ExerciseName: "One more", Note: "x"}); err != nil {
		t.Fatalf("saving after clearing one: %v", err)
	}
	listed, err := notes.List(f.ctx, f.userID)
	if err != nil || len(listed) != model.MaxExerciseNotes {
		t.Fatalf("listed %d notes, err = %v; want %d", len(listed), err, model.MaxExerciseNotes)
	}
}

func TestExerciseNoteChecks(t *testing.T) {
	f := newReleaseFixture(t)
	notes := NewExerciseNoteDAO(f.pool)
	tests := []struct {
		name string
		note model.ExerciseNote
	}{
		{name: "empty note", note: model.ExerciseNote{ExerciseKey: "row", ExerciseName: "Row", Note: ""}},
		{name: "201 character note", note: model.ExerciseNote{ExerciseKey: "row", ExerciseName: "Row", Note: strings.Repeat("a", 201)}},
		{name: "empty key", note: model.ExerciseNote{ExerciseKey: "", ExerciseName: "Row", Note: "x"}},
		{name: "121 character key", note: model.ExerciseNote{ExerciseKey: strings.Repeat("a", 121), ExerciseName: "Row", Note: "x"}},
		{name: "empty name", note: model.ExerciseNote{ExerciseKey: "row", ExerciseName: "", Note: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := notes.Save(f.ctx, f.userID, tt.note); err == nil {
				t.Fatal("stored; want the table's check to refuse it")
			}
		})
	}
	// 200 characters are counted as characters, not bytes.
	long := strings.Repeat("é", model.MaxExerciseNoteLength)
	if _, err := notes.Save(f.ctx, f.userID, model.ExerciseNote{ExerciseKey: "row", ExerciseName: "Row", Note: long}); err != nil {
		t.Fatal(fmt.Errorf("saving a 200 character note: %w", err))
	}
}
