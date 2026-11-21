package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// weekOffStore is an in-memory WeekOffDAO.
type weekOffStore struct {
	timezone string
	weeks    map[string]model.WeekOff
	err      error
	listed   [2]string
}

func (s *weekOffStore) List(_ context.Context, _ uuid.UUID, from, to string) ([]model.WeekOff, error) {
	s.listed = [2]string{from, to}
	if s.err != nil {
		return nil, s.err
	}
	weeks := []model.WeekOff{}
	for _, week := range s.weeks {
		weeks = append(weeks, week)
	}
	return weeks, nil
}

func (s *weekOffStore) Timezone(context.Context, uuid.UUID) (string, error) {
	return s.timezone, s.err
}

func (s *weekOffStore) Put(_ context.Context, _ uuid.UUID, weekStart string) (*model.WeekOff, error) {
	if s.weeks == nil {
		s.weeks = map[string]model.WeekOff{}
	}
	week, ok := s.weeks[weekStart]
	if !ok {
		week = model.WeekOff{WeekStart: weekStart, CreatedAt: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
		s.weeks[weekStart] = week
	}
	return &week, nil
}

func (s *weekOffStore) Delete(_ context.Context, _ uuid.UUID, weekStart string) error {
	delete(s.weeks, weekStart)
	return nil
}

// weekOffServiceAt returns a service whose clock reads now.
func weekOffServiceAt(store *weekOffStore, now time.Time) *weekOffService {
	return &weekOffService{weeks: store, now: func() time.Time { return now }}
}

func validationField(t *testing.T, err error) string {
	t.Helper()
	var validation *model.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("err = %v, want a validation error", err)
	}
	return validation.Field
}

func TestWeekOffTake(t *testing.T) {
	// Friday 2026-11-20 in New York; the current week starts Monday 11-16.
	now := time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		weekStart string
		wantField string
	}{
		{name: "current week", weekStart: "2026-11-16"},
		{name: "next week", weekStart: "2026-11-23"},
		{name: "eight weeks ahead", weekStart: "2027-01-11"},
		{name: "nine weeks ahead", weekStart: "2027-01-18", wantField: "week_start"},
		{name: "last week", weekStart: "2026-11-09", wantField: "week_start"},
		{name: "not a Monday", weekStart: "2026-11-24", wantField: "week_start"},
		{name: "not a date", weekStart: "next", wantField: "week_start"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &weekOffStore{timezone: "America/New_York"}
			week, err := weekOffServiceAt(store, now).Take(context.Background(), uuid.New(), tt.weekStart)
			if tt.wantField != "" {
				if got := validationField(t, err); got != tt.wantField {
					t.Fatalf("field = %q, want %q", got, tt.wantField)
				}
				if len(store.weeks) != 0 {
					t.Fatalf("stored %v, want nothing", store.weeks)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if week.WeekStart != tt.weekStart {
				t.Fatalf("week_start = %q, want %q", week.WeekStart, tt.weekStart)
			}
		})
	}
}

func TestWeekOffTakeIsIdempotent(t *testing.T) {
	store := &weekOffStore{}
	svc := weekOffServiceAt(store, time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC))
	first, err := svc.Take(context.Background(), uuid.New(), "2026-11-23")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Take(context.Background(), uuid.New(), "2026-11-23")
	if err != nil {
		t.Fatal(err)
	}
	if *first != *second || len(store.weeks) != 1 {
		t.Fatalf("first = %+v, second = %+v, stored = %d; want one unchanged week", first, second, len(store.weeks))
	}
}

// The current week is judged in the athlete's timezone: late Sunday in New
// York is already Monday in UTC.
func TestWeekOffCurrentWeekFollowsTheSavedTimezone(t *testing.T) {
	now := time.Date(2026, 11, 23, 3, 0, 0, 0, time.UTC) // Sunday 22:00 in New York
	if _, err := weekOffServiceAt(&weekOffStore{timezone: "America/New_York"}, now).Take(context.Background(), uuid.New(), "2026-11-16"); err != nil {
		t.Fatalf("New York still in the week of 11-16: %v", err)
	}
	_, err := weekOffServiceAt(&weekOffStore{}, now).Take(context.Background(), uuid.New(), "2026-11-16")
	if got := validationField(t, err); got != "week_start" {
		t.Fatalf("no profile uses UTC, where 11-16 is last week; field = %q", got)
	}
	_, err = weekOffServiceAt(&weekOffStore{timezone: "Not/AZone"}, now).Take(context.Background(), uuid.New(), "2026-11-23")
	if got := validationField(t, err); got != "timezone" {
		t.Fatalf("field = %q, want timezone", got)
	}
}

func TestWeekOffEnd(t *testing.T) {
	now := time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)
	store := &weekOffStore{weeks: map[string]model.WeekOff{"2026-11-16": {WeekStart: "2026-11-16"}, "2026-11-09": {WeekStart: "2026-11-09"}}}
	svc := weekOffServiceAt(store, now)
	if err := svc.End(context.Background(), uuid.New(), "2026-11-16"); err != nil {
		t.Fatal(err)
	}
	if err := svc.End(context.Background(), uuid.New(), "2026-11-16"); err != nil {
		t.Fatalf("ending a week that is not off: %v", err)
	}
	if got := validationField(t, svc.End(context.Background(), uuid.New(), "2026-11-09")); got != "week_start" {
		t.Fatalf("ending a past week: field = %q, want week_start", got)
	}
	if got := validationField(t, svc.End(context.Background(), uuid.New(), "2026-11-17")); got != "week_start" {
		t.Fatalf("ending a Tuesday: field = %q, want week_start", got)
	}
	if _, kept := store.weeks["2026-11-09"]; !kept || len(store.weeks) != 1 {
		t.Fatalf("stored = %v, want only the past week", store.weeks)
	}
}

func TestWeekOffList(t *testing.T) {
	store := &weekOffStore{}
	svc := NewWeekOffService(store)
	got, err := svc.List(context.Background(), uuid.New(), "2026-11-01", "2026-11-30")
	if err != nil {
		t.Fatal(err)
	}
	if got.WeeksOff == nil || len(got.WeeksOff) != 0 {
		t.Fatalf("weeks_off = %#v, want an empty list", got.WeeksOff)
	}
	if store.listed != [2]string{"2026-11-01", "2026-11-30"} {
		t.Fatalf("listed %v", store.listed)
	}
	for _, tt := range []struct{ from, to, field string }{
		{from: "2026-11-30", to: "2026-11-01", field: "to"},
		{from: "soon", to: "2026-11-01", field: "from"},
		{from: "2026-11-01", to: "later", field: "to"},
		{from: "2025-01-01", to: "2027-01-02", field: "range"},
	} {
		_, err := svc.List(context.Background(), uuid.New(), tt.from, tt.to)
		if got := validationField(t, err); got != tt.field {
			t.Fatalf("List(%s, %s) field = %q, want %q", tt.from, tt.to, got, tt.field)
		}
	}
	if _, err := svc.List(context.Background(), uuid.New(), "2025-01-01", "2027-01-01"); err != nil {
		t.Fatalf("a 731-day range: %v", err)
	}
	store.err = errors.New("database down")
	if _, err := svc.List(context.Background(), uuid.New(), "2026-11-01", "2026-11-30"); err == nil {
		t.Fatal("want the DAO error")
	}
	if _, err := svc.Take(context.Background(), uuid.New(), "2026-11-23"); err == nil {
		t.Fatal("want the timezone lookup error")
	}
}
