package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// weekOffHandlerStore is an in-memory weeks-off DAO so the handler tests run
// the real service and its validation. It has no training profile, so the
// service judges the current week in UTC.
type weekOffHandlerStore struct {
	weeks []model.WeekOff
	err   error
}

func (s *weekOffHandlerStore) List(context.Context, uuid.UUID, string, string) ([]model.WeekOff, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]model.WeekOff{}, s.weeks...), nil
}

func (s *weekOffHandlerStore) Timezone(context.Context, uuid.UUID) (string, error) { return "", s.err }

func (s *weekOffHandlerStore) Put(_ context.Context, _ uuid.UUID, weekStart string) (*model.WeekOff, error) {
	for i := range s.weeks {
		if s.weeks[i].WeekStart == weekStart {
			return &s.weeks[i], nil
		}
	}
	s.weeks = append(s.weeks, model.WeekOff{WeekStart: weekStart, CreatedAt: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)})
	return &s.weeks[len(s.weeks)-1], nil
}

func (s *weekOffHandlerStore) Delete(_ context.Context, _ uuid.UUID, weekStart string) error {
	kept := s.weeks[:0]
	for _, week := range s.weeks {
		if week.WeekStart != weekStart {
			kept = append(kept, week)
		}
	}
	s.weeks = kept
	return nil
}

// thisMonday returns the Monday of the current UTC week, offset by weeks.
func thisMonday(weeks int) string {
	now := time.Now().UTC()
	monday := now.AddDate(0, 0, -((int(now.Weekday()) + 6) % 7))
	return monday.AddDate(0, 0, 7*weeks).Format(time.DateOnly)
}

func weekOffRequest(t *testing.T, method, weekStart string) *http.Request {
	t.Helper()
	return guidedHandlerRequest(t, method, "/", nil, uuid.New(), "", map[string]string{"week_start": weekStart})
}

func TestWeekOffHandlerTakeListEnd(t *testing.T) {
	store := &weekOffHandlerStore{}
	h := NewWeekOffHandler(service.NewWeekOffService(store))
	next := thisMonday(1)

	empty := httptest.NewRecorder()
	h.List(empty, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-01-01&to=2026-12-31", nil, uuid.New(), "", nil))
	if got, want := strings.TrimSpace(empty.Body.String()), `{"weeks_off":[]}`; empty.Code != http.StatusOK || got != want {
		t.Fatalf("empty list status=%d body=%s, want 200 %s", empty.Code, got, want)
	}

	want := `{"week_start":"` + next + `","created_at":"2026-11-20T12:00:00Z"}`
	for range 2 {
		rec := httptest.NewRecorder()
		h.Take(rec, weekOffRequest(t, http.MethodPut, next))
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
			t.Fatalf("take status=%d body=%s, want 200 %s", rec.Code, got, want)
		}
	}
	if len(store.weeks) != 1 {
		t.Fatalf("stored %d weeks after two identical takes, want 1", len(store.weeks))
	}

	listed := httptest.NewRecorder()
	h.List(listed, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-01-01&to=2026-12-31", nil, uuid.New(), "", nil))
	if got := strings.TrimSpace(listed.Body.String()); got != `{"weeks_off":[`+want+`]}` {
		t.Fatalf("list body = %s", got)
	}

	for range 2 {
		rec := httptest.NewRecorder()
		h.End(rec, weekOffRequest(t, http.MethodDelete, next))
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("end status=%d body=%s, want an empty 204", rec.Code, rec.Body.String())
		}
	}
	if len(store.weeks) != 0 {
		t.Fatalf("stored %d weeks after ending, want 0", len(store.weeks))
	}
}

func TestWeekOffHandlerErrors(t *testing.T) {
	store := &weekOffHandlerStore{}
	h := NewWeekOffHandler(service.NewWeekOffService(store))
	tuesday, _ := time.Parse(time.DateOnly, thisMonday(1))
	tests := []struct {
		name   string
		invoke func(http.ResponseWriter)
	}{
		{name: "take a Tuesday", invoke: func(w http.ResponseWriter) {
			h.Take(w, weekOffRequest(t, http.MethodPut, tuesday.AddDate(0, 0, 1).Format(time.DateOnly)))
		}},
		{name: "take last week", invoke: func(w http.ResponseWriter) { h.Take(w, weekOffRequest(t, http.MethodPut, thisMonday(-1))) }},
		{name: "take too far ahead", invoke: func(w http.ResponseWriter) { h.Take(w, weekOffRequest(t, http.MethodPut, thisMonday(9))) }},
		{name: "take a non-date", invoke: func(w http.ResponseWriter) { h.Take(w, weekOffRequest(t, http.MethodPut, "soon")) }},
		{name: "end last week", invoke: func(w http.ResponseWriter) { h.End(w, weekOffRequest(t, http.MethodDelete, thisMonday(-1))) }},
		{name: "list without a range", invoke: func(w http.ResponseWriter) {
			h.List(w, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", nil))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.invoke(rec)
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"code":"VALIDATION_ERROR"`) {
				t.Fatalf("status=%d body=%s, want a 422 validation error", rec.Code, rec.Body.String())
			}
		})
	}
	if len(store.weeks) != 0 {
		t.Fatalf("stored %v, want nothing", store.weeks)
	}

	store.err = context.DeadlineExceeded
	failed := httptest.NewRecorder()
	h.List(failed, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-01-01&to=2026-12-31", nil, uuid.New(), "", nil))
	if failed.Code != http.StatusInternalServerError || !strings.Contains(failed.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("failed list status=%d body=%s", failed.Code, failed.Body.String())
	}
}
