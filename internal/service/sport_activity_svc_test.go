package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type sportActivityRepo struct {
	activities []model.SportActivity
	created    *model.SportActivity
	timezone   string
	key        string
	hash       string
	err        error
}

func (r *sportActivityRepo) List(context.Context, uuid.UUID, string, string) ([]model.SportActivity, error) {
	return r.activities, r.err
}
func (r *sportActivityRepo) Get(context.Context, uuid.UUID, uuid.UUID) (*model.SportActivity, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &r.activities[0], nil
}
func (r *sportActivityRepo) Create(_ context.Context, _ uuid.UUID, activity *model.SportActivity, timezone, key, hash string) (*model.SportActivity, bool, error) {
	r.created, r.timezone, r.key, r.hash = activity, timezone, key, hash
	if r.err != nil {
		return nil, false, r.err
	}
	result := *activity
	result.ID = uuid.New()
	return &result, false, nil
}

type sportProfileRepo struct {
	profile *model.TrainingProfile
	err     error
}

func (r sportProfileRepo) Get(context.Context, uuid.UUID) (*model.TrainingProfile, error) {
	return r.profile, r.err
}
func (sportProfileRepo) Put(context.Context, uuid.UUID, *model.TrainingProfile, int64) error {
	return nil
}

func newSportServiceForTest(repo *sportActivityRepo, timezone string) *sportActivityService {
	svc := &sportActivityService{activities: repo, profiles: sportProfileRepo{profile: &model.TrainingProfile{Timezone: timezone}}}
	svc.now = func() time.Time { return time.Date(2026, 8, 3, 1, 30, 0, 0, time.UTC) }
	return svc
}

func TestSportActivityServiceCreateNormalizesAndUsesProfileCivilDate(t *testing.T) {
	repo := &sportActivityRepo{}
	svc := newSportServiceForTest(repo, "America/Los_Angeles")
	notes := "  Pickup game  "
	got, err := svc.Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{
		SportID: " basketball ", SportName: " Basketball ", DurationMinutes: 60,
		Notes: &notes, OperationKey: " sport-op ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || repo.created.Date != "2026-08-02" {
		t.Fatalf("date=%v want athlete-local 2026-08-02", repo.created)
	}
	if repo.created.SportID != "basketball" || repo.created.SportName != "Basketball" || *repo.created.Notes != "Pickup game" {
		t.Fatalf("request not normalized: %+v", repo.created)
	}
	if repo.timezone != "America/Los_Angeles" || repo.key != "sport-op" || repo.hash == "" {
		t.Fatalf("persistence metadata missing: timezone=%q key=%q hash=%q", repo.timezone, repo.key, repo.hash)
	}
}

func TestSportActivityServiceCreateValidation(t *testing.T) {
	longNotes := strings.Repeat("x", 2001)
	tests := []struct {
		name  string
		req   model.CreateSportActivityRequest
		field string
	}{
		{"future date", model.CreateSportActivityRequest{Date: "2026-08-04", SportID: "tennis", SportName: "Tennis", DurationMinutes: 30, OperationKey: "op"}, "date"},
		{"bad date", model.CreateSportActivityRequest{Date: "08/02/2026", SportID: "tennis", SportName: "Tennis", DurationMinutes: 30, OperationKey: "op"}, "date"},
		{"bad id", model.CreateSportActivityRequest{Date: "2026-08-02", SportID: "Tennis!", SportName: "Tennis", DurationMinutes: 30, OperationKey: "op"}, "sport_id"},
		{"other without custom", model.CreateSportActivityRequest{Date: "2026-08-02", SportID: "other", SportName: "Other", DurationMinutes: 30, OperationKey: "op"}, "sport_name"},
		{"duration", model.CreateSportActivityRequest{Date: "2026-08-02", SportID: "tennis", SportName: "Tennis", DurationMinutes: 0, OperationKey: "op"}, "duration_minutes"},
		{"notes", model.CreateSportActivityRequest{Date: "2026-08-02", SportID: "tennis", SportName: "Tennis", DurationMinutes: 30, Notes: &longNotes, OperationKey: "op"}, "notes"},
		{"operation", model.CreateSportActivityRequest{Date: "2026-08-02", SportID: "tennis", SportName: "Tennis", DurationMinutes: 30}, "operation_key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newSportServiceForTest(&sportActivityRepo{}, "UTC").Create(context.Background(), uuid.New(), test.req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("error=%v want validation field %s", err, test.field)
			}
		})
	}
}

func TestSportActivityServiceRejectsInvalidSavedTimezoneAndRange(t *testing.T) {
	_, err := newSportServiceForTest(&sportActivityRepo{}, "Mars/Olympus").Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{})
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "timezone" {
		t.Fatalf("error=%v want timezone validation", err)
	}

	svc := newSportServiceForTest(&sportActivityRepo{}, "UTC")
	for _, dates := range [][2]string{{"bad", "2026-08-02"}, {"2026-08-03", "2026-08-02"}, {"2025-01-01", "2026-08-02"}} {
		if _, err := svc.List(context.Background(), uuid.New(), dates[0], dates[1]); err == nil {
			t.Fatalf("range %v unexpectedly accepted", dates)
		}
	}
}

func TestSportActivityServicePassesThroughRepositories(t *testing.T) {
	want := []model.SportActivity{{ID: uuid.New(), Date: "2026-08-02"}}
	repo := &sportActivityRepo{activities: want}
	svc := newSportServiceForTest(repo, "UTC")
	got, err := svc.List(context.Background(), uuid.New(), "2026-08-01", "2026-08-02")
	if err != nil || len(got) != 1 || got[0].ID != want[0].ID {
		t.Fatalf("list=%+v error=%v", got, err)
	}
	activity, err := svc.Get(context.Background(), uuid.New(), want[0].ID)
	if err != nil || activity.ID != want[0].ID {
		t.Fatalf("get=%+v error=%v", activity, err)
	}

	repo.err = errors.New("database unavailable")
	if _, err := svc.List(context.Background(), uuid.New(), "2026-08-01", "2026-08-02"); err == nil {
		t.Fatal("list error not returned")
	}
	if _, err := svc.Get(context.Background(), uuid.New(), want[0].ID); err == nil {
		t.Fatal("get error not returned")
	}
	if _, err := svc.Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{Date: "2026-08-02", SportID: "tennis", SportName: "Tennis", DurationMinutes: 30, OperationKey: "op"}); err == nil {
		t.Fatal("create error not returned")
	}
}

func TestSportActivityServiceReturnsProfileError(t *testing.T) {
	want := errors.New("profile unavailable")
	svc := NewSportActivityService(&sportActivityRepo{}, sportProfileRepo{err: want})
	_, err := svc.Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v want profile error", err)
	}
}
