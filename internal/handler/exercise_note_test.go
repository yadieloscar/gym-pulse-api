package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// exerciseNoteHandlerStore is an in-memory exercise-notes DAO keyed by
// athlete, so the handler tests run the real service and its validation.
type exerciseNoteHandlerStore struct {
	notes map[uuid.UUID]map[string]model.ExerciseNote
	err   error
}

func (s *exerciseNoteHandlerStore) List(_ context.Context, userID uuid.UUID) ([]model.ExerciseNote, error) {
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

func (s *exerciseNoteHandlerStore) Save(_ context.Context, userID uuid.UUID, note model.ExerciseNote) (*model.ExerciseNote, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.notes == nil {
		s.notes = map[uuid.UUID]map[string]model.ExerciseNote{}
	}
	if s.notes[userID] == nil {
		s.notes[userID] = map[string]model.ExerciseNote{}
	}
	note.UpdatedAt = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	s.notes[userID][note.ExerciseKey] = note
	return &note, nil
}

func (s *exerciseNoteHandlerStore) Delete(_ context.Context, userID uuid.UUID, exerciseKey string) error {
	if s.err != nil {
		return s.err
	}
	delete(s.notes[userID], exerciseKey)
	return nil
}

func exerciseNoteList(t *testing.T, h *ExerciseNoteHandler, userID uuid.UUID) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.List(rec, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	return strings.TrimSpace(rec.Body.String())
}

func exerciseNoteSave(t *testing.T, h *ExerciseNoteHandler, userID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Save(rec, guidedHandlerRequest(t, http.MethodPut, "/", body, userID, "", nil))
	return rec
}

func TestExerciseNoteHandlerSaveListClear(t *testing.T) {
	store := &exerciseNoteHandlerStore{}
	h := NewExerciseNoteHandler(service.NewExerciseNoteService(store))
	athlete := uuid.New()

	if got, want := exerciseNoteList(t, h, athlete), `{"exercise_notes":[]}`; got != want {
		t.Fatalf("empty list = %s, want %s", got, want)
	}

	saved := `{"exercise_key":"leg press","exercise_name":"Leg press","note":"seat height 4","updated_at":"2026-11-25T12:00:00Z"}`
	// The second save is the same request replayed: one note, same answer.
	for range 2 {
		rec := exerciseNoteSave(t, h, athlete, map[string]string{"exercise_name": "Leg press", "note": " seat height 4 "})
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != saved {
			t.Fatalf("save status=%d body=%s, want 200 %s", rec.Code, got, saved)
		}
	}
	if got, want := exerciseNoteList(t, h, athlete), `{"exercise_notes":[`+saved+`]}`; got != want {
		t.Fatalf("list after save = %s, want %s", got, want)
	}

	edited := exerciseNoteSave(t, h, athlete, map[string]string{"exercise_name": "LEG  press", "note": "seat height 5"})
	if edited.Code != http.StatusOK || !strings.Contains(edited.Body.String(), `"exercise_key":"leg press","exercise_name":"LEG press","note":"seat height 5"`) {
		t.Fatalf("edit status=%d body=%s", edited.Code, edited.Body.String())
	}
	if len(store.notes[athlete]) != 1 {
		t.Fatalf("stored %d notes after an edit, want 1", len(store.notes[athlete]))
	}

	cleared := exerciseNoteSave(t, h, athlete, map[string]string{"exercise_name": "Leg press", "note": ""})
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"exercise_key":"leg press","exercise_name":"Leg press","note":""`) {
		t.Fatalf("clear status=%d body=%s", cleared.Code, cleared.Body.String())
	}
	if got, want := exerciseNoteList(t, h, athlete), `{"exercise_notes":[]}`; got != want {
		t.Fatalf("list after clear = %s, want %s", got, want)
	}
}

func TestExerciseNoteHandlerScopesToTheAuthenticatedAthlete(t *testing.T) {
	store := &exerciseNoteHandlerStore{}
	h := NewExerciseNoteHandler(service.NewExerciseNoteService(store))
	mine, theirs := uuid.New(), uuid.New()

	if rec := exerciseNoteSave(t, h, mine, map[string]string{"exercise_name": "Leg press", "note": "seat height 4"}); rec.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got, want := exerciseNoteList(t, h, theirs), `{"exercise_notes":[]}`; got != want {
		t.Fatalf("another athlete's list = %s, want %s", got, want)
	}
	// The other athlete saving and clearing the same exercise name touches
	// only their own note.
	if rec := exerciseNoteSave(t, h, theirs, map[string]string{"exercise_name": "Leg press", "note": "theirs"}); rec.Code != http.StatusOK {
		t.Fatalf("their save status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := exerciseNoteSave(t, h, theirs, map[string]string{"exercise_name": "Leg press", "note": ""}); rec.Code != http.StatusOK {
		t.Fatalf("their clear status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := exerciseNoteList(t, h, mine); !strings.Contains(got, `"note":"seat height 4"`) || strings.Contains(got, "theirs") {
		t.Fatalf("my list after their save and clear = %s", got)
	}
}

func TestExerciseNoteHandlerErrors(t *testing.T) {
	store := &exerciseNoteHandlerStore{}
	h := NewExerciseNoteHandler(service.NewExerciseNoteService(store))
	athlete := uuid.New()
	tests := []struct {
		name      string
		body      any
		wantField string
	}{
		{name: "no name", body: map[string]string{"note": "seat height 4"}, wantField: "exercise_name"},
		{name: "blank name", body: map[string]string{"exercise_name": "   ", "note": "seat height 4"}, wantField: "exercise_name"},
		{name: "long name", body: map[string]string{"exercise_name": strings.Repeat("a", 121), "note": "x"}, wantField: "exercise_name"},
		{name: "long note", body: map[string]string{"exercise_name": "Row", "note": strings.Repeat("a", 201)}, wantField: "note"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := exerciseNoteSave(t, h, athlete, tt.body)
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"code":"VALIDATION_ERROR"`) ||
				!strings.Contains(rec.Body.String(), `"field":"`+tt.wantField+`"`) {
				t.Fatalf("status=%d body=%s, want a 422 validation error on %s", rec.Code, rec.Body.String(), tt.wantField)
			}
			// A refusal names the field and never repeats the athlete's text.
			if strings.Contains(rec.Body.String(), "seat height") {
				t.Fatalf("error body repeats the note: %s", rec.Body.String())
			}
		})
	}
	if len(store.notes[athlete]) != 0 {
		t.Fatalf("stored %v, want nothing", store.notes[athlete])
	}

	malformed := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"exercise_name":`))
	h.Save(malformed, req)
	if malformed.Code != http.StatusBadRequest || !strings.Contains(malformed.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("malformed body status=%d body=%s", malformed.Code, malformed.Body.String())
	}

	store.err = context.DeadlineExceeded
	failedList := httptest.NewRecorder()
	h.List(failedList, guidedHandlerRequest(t, http.MethodGet, "/", nil, athlete, "", nil))
	if failedList.Code != http.StatusInternalServerError || !strings.Contains(failedList.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("failed list status=%d body=%s", failedList.Code, failedList.Body.String())
	}
	if rec := exerciseNoteSave(t, h, athlete, map[string]string{"exercise_name": "Row", "note": "x"}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("failed save status=%d body=%s", rec.Code, rec.Body.String())
	}
}
