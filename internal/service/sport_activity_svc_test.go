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
	outcome    model.SportActivityCreateOutcome
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
func (r *sportActivityRepo) Create(_ context.Context, _ uuid.UUID, activity *model.SportActivity, timezone, key, hash string) (*model.SportActivity, model.SportActivityCreateOutcome, error) {
	r.created, r.timezone, r.key, r.hash = activity, timezone, key, hash
	if r.err != nil {
		return nil, model.SportActivityCreated, r.err
	}
	result := *activity
	result.ID = uuid.New()
	return &result, r.outcome, nil
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
	got, _, err := svc.Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{
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
			_, _, err := newSportServiceForTest(&sportActivityRepo{}, "UTC").Create(context.Background(), uuid.New(), test.req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("error=%v want validation field %s", err, test.field)
			}
		})
	}
}

func TestSportActivityServiceRejectsInvalidSavedTimezoneAndRange(t *testing.T) {
	_, _, err := newSportServiceForTest(&sportActivityRepo{}, "Mars/Olympus").Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{})
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
	if _, _, err := svc.Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{Date: "2026-08-02", SportID: "tennis", SportName: "Tennis", DurationMinutes: 30, OperationKey: "op"}); err == nil {
		t.Fatal("create error not returned")
	}
}

func TestSportActivityServiceReturnsProfileError(t *testing.T) {
	want := errors.New("profile unavailable")
	svc := NewSportActivityService(&sportActivityRepo{}, sportProfileRepo{err: want})
	_, _, err := svc.Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v want profile error", err)
	}
}

func TestSportActivityServiceCreateDefaultsSourceToManual(t *testing.T) {
	repo := &sportActivityRepo{}
	_, outcome, err := newSportServiceForTest(repo, "UTC").Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{
		Date: "2026-08-02", SportID: "tennis", SportName: "Tennis", DurationMinutes: 30, OperationKey: "op",
	})
	if err != nil || outcome != model.SportActivityCreated {
		t.Fatalf("outcome=%v error=%v", outcome, err)
	}
	if repo.created.Source != model.ActivitySourceManual || repo.created.ExternalID != nil {
		t.Fatalf("manual default not applied: %+v", repo.created)
	}
}

func TestSportActivityServiceCreateCarriesImportFields(t *testing.T) {
	repo := &sportActivityRepo{outcome: model.SportActivityExisting}
	external := "  9B3C2E1A-6F0D-4A51-9C7E-2B1F0D8E4A77  "
	_, outcome, err := newSportServiceForTest(repo, "America/New_York").Create(context.Background(), uuid.New(), model.CreateSportActivityRequest{
		Date: "2026-07-30", SportID: "running", SportName: "Running", DurationMinutes: 42, OperationKey: "hk:9B3C",
		Source: model.ActivitySourceAppleHealth, ExternalID: &external,
		DistanceMeters: intPtr(8120), EnergyKcal: intPtr(512), AvgHeartRateBPM: intPtr(151),
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != model.SportActivityExisting {
		t.Fatalf("outcome=%v want the DAO outcome passed through", outcome)
	}
	got := repo.created
	if got.Source != model.ActivitySourceAppleHealth || got.ExternalID == nil || *got.ExternalID != "9B3C2E1A-6F0D-4A51-9C7E-2B1F0D8E4A77" {
		t.Fatalf("import identity not carried or trimmed: %+v", got)
	}
	if got.DistanceMeters == nil || *got.DistanceMeters != 8120 || got.EnergyKcal == nil || *got.EnergyKcal != 512 || got.AvgHeartRateBPM == nil || *got.AvgHeartRateBPM != 151 {
		t.Fatalf("measurements not carried: %+v", got)
	}
	if got.Date != "2026-07-30" {
		t.Fatalf("past import date must be accepted as given: %s", got.Date)
	}
}

func TestSportActivityServiceCreateImportValidation(t *testing.T) {
	base := func() model.CreateSportActivityRequest {
		return model.CreateSportActivityRequest{Date: "2026-08-02", SportID: "running", SportName: "Running", DurationMinutes: 42, OperationKey: "op"}
	}
	tests := []struct {
		name  string
		edit  func(*model.CreateSportActivityRequest)
		field string
	}{
		{"manual with identity", func(r *model.CreateSportActivityRequest) { r.ExternalID = strPtr("abc") }, "external_id"},
		{"imported without identity", func(r *model.CreateSportActivityRequest) { r.Source = model.ActivitySourceAppleHealth }, "external_id"},
		{"imported blank identity", func(r *model.CreateSportActivityRequest) {
			r.Source = model.ActivitySourceAppleHealth
			r.ExternalID = strPtr("   ")
		}, "external_id"},
		{"identity too long", func(r *model.CreateSportActivityRequest) {
			r.Source = model.ActivitySourceAppleHealth
			r.ExternalID = strPtr(strings.Repeat("x", 129))
		}, "external_id"},
		{"unknown source", func(r *model.CreateSportActivityRequest) { r.Source = "garmin"; r.ExternalID = strPtr("abc") }, "source"},
		{"distance zero", func(r *model.CreateSportActivityRequest) { r.DistanceMeters = intPtr(0) }, "distance_meters"},
		{"distance too far", func(r *model.CreateSportActivityRequest) { r.DistanceMeters = intPtr(1000001) }, "distance_meters"},
		{"energy zero", func(r *model.CreateSportActivityRequest) { r.EnergyKcal = intPtr(0) }, "energy_kcal"},
		{"energy too high", func(r *model.CreateSportActivityRequest) { r.EnergyKcal = intPtr(20001) }, "energy_kcal"},
		{"heart rate low", func(r *model.CreateSportActivityRequest) { r.AvgHeartRateBPM = intPtr(29) }, "avg_heart_rate_bpm"},
		{"heart rate high", func(r *model.CreateSportActivityRequest) { r.AvgHeartRateBPM = intPtr(251) }, "avg_heart_rate_bpm"},
		{"future import date", func(r *model.CreateSportActivityRequest) {
			r.Date = "2026-08-04"
			r.Source = model.ActivitySourceAppleHealth
			r.ExternalID = strPtr("abc")
		}, "date"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := base()
			test.edit(&req)
			repo := &sportActivityRepo{}
			_, _, err := newSportServiceForTest(repo, "UTC").Create(context.Background(), uuid.New(), req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("error=%v want validation field %s", err, test.field)
			}
			if repo.created != nil {
				t.Fatal("invalid request must not reach the repository")
			}
		})
	}

	// Boundary values are accepted and absent measurements stay absent.
	repo := &sportActivityRepo{}
	req := base()
	req.Source = model.ActivitySourceAppleHealth
	req.ExternalID = strPtr(strings.Repeat("x", 128))
	req.DistanceMeters, req.EnergyKcal, req.AvgHeartRateBPM = intPtr(1000000), intPtr(20000), intPtr(250)
	if _, _, err := newSportServiceForTest(repo, "UTC").Create(context.Background(), uuid.New(), req); err != nil {
		t.Fatalf("upper bounds rejected: %v", err)
	}
	repo = &sportActivityRepo{}
	if _, _, err := newSportServiceForTest(repo, "UTC").Create(context.Background(), uuid.New(), base()); err != nil {
		t.Fatal(err)
	}
	if repo.created.DistanceMeters != nil || repo.created.EnergyKcal != nil || repo.created.AvgHeartRateBPM != nil {
		t.Fatalf("absent measurements must stay nil: %+v", repo.created)
	}
}
