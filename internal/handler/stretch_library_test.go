package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// stretchLibraryHandlerStore is an in-memory library DAO so the handler tests
// run the real service and its validation.
type stretchLibraryHandlerStore struct {
	stored *model.StretchLibrary
	err    error
}

func (s *stretchLibraryHandlerStore) current() *model.StretchLibrary {
	if s.stored == nil {
		return model.EmptyStretchLibrary()
	}
	return s.stored
}

func (s *stretchLibraryHandlerStore) Get(context.Context, uuid.UUID) (*model.StretchLibrary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.current(), nil
}

func (s *stretchLibraryHandlerStore) Save(_ context.Context, _ uuid.UUID, moves []model.StretchMove, routines []model.StretchRoutine, expected int64, _, _ string) (*model.StretchLibrary, error) {
	if s.err != nil {
		return nil, s.err
	}
	current := s.current()
	if expected != current.Revision {
		return nil, &model.ConflictError{Message: "resource revision conflict", Expected: expected, Actual: current.Revision, Authoritative: current}
	}
	updatedAt := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	s.stored = &model.StretchLibrary{Revision: expected + 1, Moves: moves, Routines: routines, UpdatedAt: &updatedAt}
	return s.stored, nil
}

func stretchLibraryHandler(store *stretchLibraryHandlerStore) *StretchLibraryHandler {
	return NewStretchLibraryHandler(service.NewStretchLibraryService(store))
}

func stretchLibraryBody(expectedRevision int, operationKey string) map[string]any {
	return map[string]any{
		"expected_revision": expectedRevision,
		"operation_key":     operationKey,
		"moves": []map[string]any{
			{"id": "my-couch", "name": "Couch stretch", "cue": "Hips forward", "areas": "Hips", "mode": "hold", "sided": true, "hold_seconds": 45, "reps": nil},
			{"id": "my-swings", "name": "Leg swings", "cue": "", "areas": "", "mode": "reps", "sided": false, "hold_seconds": nil, "reps": 12},
		},
		"routines": []map[string]any{
			{"id": "my-evening", "name": "Evening", "items": []map[string]any{
				{"move_id": "my-couch", "hold_seconds": nil, "reps": nil},
				{"move_id": "cat-cow", "hold_seconds": nil, "reps": 8},
			}},
		},
	}
}

func TestStretchLibraryHandlerGet(t *testing.T) {
	store := &stretchLibraryHandlerStore{}
	rec := httptest.NewRecorder()
	stretchLibraryHandler(store).Get(rec, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"revision":0,"moves":[],"routines":[],"updated_at":null}`; got != want {
		t.Errorf("empty library body = %s, want %s", got, want)
	}

	store.err = context.DeadlineExceeded
	failed := httptest.NewRecorder()
	stretchLibraryHandler(store).Get(failed, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", nil))
	if failed.Code != http.StatusInternalServerError || !strings.Contains(failed.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Errorf("failed read status=%d body=%s", failed.Code, failed.Body.String())
	}
}

func TestStretchLibraryHandlerSave(t *testing.T) {
	store := &stretchLibraryHandlerStore{}
	h := stretchLibraryHandler(store)
	userID := uuid.New()

	rec := httptest.NewRecorder()
	h.Save(rec, guidedHandlerRequest(t, http.MethodPut, "/", stretchLibraryBody(0, "op-1"), userID, "op-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	want := `{"revision":1,` +
		`"moves":[{"id":"my-couch","name":"Couch stretch","cue":"Hips forward","areas":"Hips","mode":"hold","sided":true,"hold_seconds":45,"reps":null},` +
		`{"id":"my-swings","name":"Leg swings","cue":"","areas":"","mode":"reps","sided":false,"hold_seconds":null,"reps":12}],` +
		`"routines":[{"id":"my-evening","name":"Evening","items":[{"move_id":"my-couch","hold_seconds":null,"reps":null},{"move_id":"cat-cow","hold_seconds":null,"reps":8}]}],` +
		`"updated_at":"2026-11-18T12:00:00Z"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("saved library body =\n%s\nwant\n%s", got, want)
	}

	read := httptest.NewRecorder()
	h.Get(read, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", nil))
	if got := strings.TrimSpace(read.Body.String()); got != want {
		t.Errorf("read after save =\n%s\nwant\n%s", got, want)
	}
}

func TestStretchLibraryHandlerSaveErrors(t *testing.T) {
	userID := uuid.New()
	updatedAt := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	stored := func() *stretchLibraryHandlerStore {
		return &stretchLibraryHandlerStore{stored: &model.StretchLibrary{Revision: 3, Moves: []model.StretchMove{}, Routines: []model.StretchRoutine{}, UpdatedAt: &updatedAt}}
	}
	badMove := stretchLibraryBody(0, "op")
	badMove["moves"] = []map[string]any{{"id": "couch", "name": "Couch stretch", "mode": "hold", "hold_seconds": 45}}
	badRoutine := stretchLibraryBody(0, "op")
	badRoutine["routines"] = []map[string]any{{"id": "my-evening", "name": "Evening", "items": []map[string]any{{"move_id": "my-gone"}}}}
	missingLists := map[string]any{"expected_revision": 0, "operation_key": "op"}

	tests := []struct {
		name      string
		store     *stretchLibraryHandlerStore
		body      any
		headerKey string
		status    int
		contains  []string
	}{
		{"malformed body", &stretchLibraryHandlerStore{}, `{"moves":`, "op", http.StatusBadRequest, []string{`"code":"BAD_REQUEST"`}},
		{"missing Idempotency-Key", &stretchLibraryHandlerStore{}, stretchLibraryBody(0, "op"), "", http.StatusUnprocessableEntity, []string{`"code":"VALIDATION_ERROR"`, `"field":"Idempotency-Key"`}},
		{"Idempotency-Key differs from operation_key", &stretchLibraryHandlerStore{}, stretchLibraryBody(0, "op"), "other", http.StatusUnprocessableEntity, []string{`"field":"Idempotency-Key"`}},
		{"bad move", &stretchLibraryHandlerStore{}, badMove, "op", http.StatusUnprocessableEntity, []string{`"code":"VALIDATION_ERROR"`, `"field":"moves"`, `"index":0`}},
		{"bad routine", &stretchLibraryHandlerStore{}, badRoutine, "op", http.StatusUnprocessableEntity, []string{`"code":"VALIDATION_ERROR"`, `"field":"routines"`, `"index":0`}},
		{"lists left out", &stretchLibraryHandlerStore{}, missingLists, "op", http.StatusUnprocessableEntity, []string{`"field":"moves"`}},
		{"negative expected_revision", &stretchLibraryHandlerStore{}, stretchLibraryBody(-1, "op"), "op", http.StatusUnprocessableEntity, []string{`"field":"expected_revision"`}},
		{"stale revision", stored(), stretchLibraryBody(2, "op"), "op", http.StatusConflict, []string{
			`"code":"REVISION_CONFLICT"`, `"expected_revision":2`, `"actual_revision":3`,
			`"resource":{"revision":3,"moves":[],"routines":[],"updated_at":"2026-11-17T08:00:00Z"}`,
		}},
		{"stale revision before any save", &stretchLibraryHandlerStore{}, stretchLibraryBody(1, "op"), "op", http.StatusConflict, []string{
			`"code":"REVISION_CONFLICT"`, `"expected_revision":1`, `"actual_revision":0`,
			`"resource":{"revision":0,"moves":[],"routines":[],"updated_at":null}`,
		}},
		{"key reused with another body", &stretchLibraryHandlerStore{err: &model.ConflictError{Message: "idempotency key was already used with a different payload"}}, stretchLibraryBody(0, "op"), "op", http.StatusConflict, []string{`"code":"IDEMPOTENCY_CONFLICT"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			stretchLibraryHandler(tt.store).Save(rec, guidedHandlerRequest(t, http.MethodPut, "/", tt.body, userID, tt.headerKey, nil))
			if rec.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tt.status, rec.Body.String())
			}
			for _, want := range tt.contains {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("body %s lacks %s", rec.Body.String(), want)
				}
			}
			var envelope map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope["error"] == "" {
				t.Errorf("error body is not the JSON envelope: %s", rec.Body.String())
			}
		})
	}
}
