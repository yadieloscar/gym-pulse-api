package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// recapHandlerStore is an in-memory recap DAO so the handler tests run the
// real service.
type recapHandlerStore struct {
	read *dao.ProgramRecapRead
	err  error
}

func (s *recapHandlerStore) Read(context.Context, uuid.UUID, uuid.UUID) (*dao.ProgramRecapRead, error) {
	return s.read, s.err
}

func recapRequest(t *testing.T, id string) *http.Request {
	t.Helper()
	return guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", map[string]string{"id": id})
}

func TestProgramRecapHandler(t *testing.T) {
	programID := uuid.New()
	store := &recapHandlerStore{read: &dao.ProgramRecapRead{
		Name: "Push Pull Legs", TrainingDays: 1,
		Sessions:     []dao.ProgramRecapSession{{Date: "2026-09-07", Status: model.WorkoutStatusCompleted}},
		TrainedDates: []string{"2026-09-07"},
		LiftDays:     []dao.ProgramRecapLiftDay{{Name: "Back Squat", Date: "2026-09-07", Weight: 185.5, Reps: 5}},
	}}
	h := NewProgramRecapHandler(service.NewProgramRecapService(store))

	rec := httptest.NewRecorder()
	h.Get(rec, recapRequest(t, programID.String()))
	want := `{"program_id":"` + programID.String() + `","name":"Push Pull Legs","active":false,` +
		`"first_date":"2026-09-07","last_date":"2026-09-07","weeks":{"total":1,"on_target":1},"workouts_done":1,` +
		`"lifts":[{"name":"Back Squat","days":1,"first":{"weight":185.5,"reps":5,"date":"2026-09-07"},"last":{"weight":185.5,"reps":5,"date":"2026-09-07"}}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Fatalf("status=%d body=%s\nwant 200 %s", rec.Code, got, want)
	}

	// A program with nothing done yet: null dates and an empty list.
	store.read = &dao.ProgramRecapRead{Name: "New", Active: true}
	rec = httptest.NewRecorder()
	h.Get(rec, recapRequest(t, programID.String()))
	want = `{"program_id":"` + programID.String() + `","name":"New","active":true,"first_date":null,"last_date":null,` +
		`"weeks":{"total":0,"on_target":0},"workouts_done":0,"lifts":[]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Fatalf("status=%d body=%s\nwant 200 %s", rec.Code, got, want)
	}
}

func TestProgramRecapHandlerErrors(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		err      error
		wantCode int
		wantBody string
	}{
		{name: "malformed id", id: "nope", wantCode: http.StatusBadRequest, wantBody: `"code":"BAD_REQUEST"`},
		{name: "missing or foreign program", id: uuid.NewString(), err: &model.NotFoundError{Message: "program not found"},
			wantCode: http.StatusNotFound, wantBody: `"code":"NOT_FOUND"`},
		{name: "database failure", id: uuid.NewString(), err: context.DeadlineExceeded,
			wantCode: http.StatusInternalServerError, wantBody: `"code":"INTERNAL_ERROR"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := NewProgramRecapHandler(service.NewProgramRecapService(&recapHandlerStore{err: test.err}))
			rec := httptest.NewRecorder()
			h.Get(rec, recapRequest(t, test.id))
			if rec.Code != test.wantCode || !strings.Contains(rec.Body.String(), test.wantBody) {
				t.Fatalf("status=%d body=%s, want %d with %s", rec.Code, rec.Body.String(), test.wantCode, test.wantBody)
			}
		})
	}
}
