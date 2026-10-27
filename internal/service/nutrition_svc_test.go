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

// nutritionRepo records the DAO call and can mirror the idempotency ledger.
type nutritionRepo struct {
	day       *model.NutritionDay
	expected  *int64
	remove    bool
	key, hash string
	target    *model.NutritionTarget
	targetRev int64
	today     string
	ledger    map[string]string
	outcome   model.NutritionDayOutcome
	err       error
	listed    *model.NutritionRange
	current   *model.NutritionTarget
}

func (r *nutritionRepo) SaveDay(_ context.Context, _ uuid.UUID, day *model.NutritionDay, expected *int64, remove bool, key, hash string) (*model.NutritionDay, model.NutritionDayOutcome, error) {
	r.day, r.expected, r.remove, r.key, r.hash = day, expected, remove, key, hash
	if r.err != nil {
		return nil, model.NutritionDayCreated, r.err
	}
	if r.ledger != nil {
		if stored, ok := r.ledger[key]; ok {
			if stored != hash {
				return nil, model.NutritionDayCreated, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
			}
			return day, model.NutritionDayReplaced, nil
		}
		r.ledger[key] = hash
	}
	return day, r.outcome, nil
}

func (r *nutritionRepo) ListDays(context.Context, uuid.UUID, string, string) (*model.NutritionRange, error) {
	return r.listed, r.err
}

func (r *nutritionRepo) GetCurrentTarget(_ context.Context, _ uuid.UUID, today string) (*model.NutritionTarget, error) {
	r.today = today
	if r.current == nil {
		return nil, &model.NotFoundError{Message: "nutrition target not found"}
	}
	return r.current, nil
}

func (r *nutritionRepo) SaveTarget(_ context.Context, _ uuid.UUID, target *model.NutritionTarget, expected int64, today, key, hash string) (*model.NutritionTarget, model.NutritionTargetOutcome, error) {
	r.target, r.targetRev, r.today, r.key, r.hash = target, expected, today, key, hash
	if r.err != nil {
		return nil, model.NutritionTargetCreated, r.err
	}
	if r.ledger != nil {
		if stored, ok := r.ledger[key]; ok {
			if stored != hash {
				return nil, model.NutritionTargetCreated, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
			}
			return target, model.NutritionTargetUnchanged, nil
		}
		r.ledger[key] = hash
	}
	return target, model.NutritionTargetCreated, nil
}

// 2026-10-25 23:30 UTC is already 2026-10-26 in Auckland and still 2026-10-25 in New York.
var nutritionClock = func() time.Time { return time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC) }

func newNutritionServiceForTest(repo *nutritionRepo, timezone string) *nutritionService {
	return &nutritionService{nutrition: repo, profiles: sportProfileRepo{profile: &model.TrainingProfile{Timezone: timezone}}, now: nutritionClock}
}

func ptrInt(v int) *int           { return &v }
func ptrFloat(v float64) *float64 { return &v }
func ptrInt64(v int64) *int64     { return &v }

func validDayRequest() model.SaveNutritionDayRequest {
	return model.SaveNutritionDayRequest{ProteinG: ptrFloat(162.5), EnergyKcal: ptrInt(2150), ExpectedRevision: ptrInt64(0), OperationKey: " day-1 "}
}

func TestNutritionServiceSaveDayNormalizesAndDefaults(t *testing.T) {
	repo := &nutritionRepo{outcome: model.NutritionDayCreated}
	got, outcome, err := newNutritionServiceForTest(repo, "America/New_York").SaveDay(context.Background(), uuid.New(), "2026-10-25", validDayRequest())
	if err != nil || got == nil || outcome != model.NutritionDayCreated {
		t.Fatalf("err=%v outcome=%v", err, outcome)
	}
	if repo.key != "day-1" || repo.hash == "" || repo.remove || repo.day.Source != model.NutritionSourceManual || repo.day.Date != "2026-10-25" {
		t.Fatalf("not normalized: key=%q day=%+v remove=%v", repo.key, repo.day, repo.remove)
	}
	if repo.expected == nil || *repo.expected != 0 || *repo.day.ProteinG != 162.5 || *repo.day.EnergyKcal != 2150 || repo.day.CarbsG != nil {
		t.Fatalf("values not carried: expected=%v day=%+v", repo.expected, repo.day)
	}
}

func TestNutritionServiceSaveDayLocalTodayFollowsProfileTimezone(t *testing.T) {
	// 23:30 UTC on the 25th: New York may still log the 25th but not the 26th;
	// Auckland is already on the 26th.
	for _, test := range []struct {
		timezone, date string
		wantErr        bool
	}{
		{"America/New_York", "2026-10-25", false},
		{"America/New_York", "2026-10-26", true},
		{"Pacific/Auckland", "2026-10-26", false},
		{"Pacific/Auckland", "2026-10-27", true},
	} {
		repo := &nutritionRepo{}
		_, _, err := newNutritionServiceForTest(repo, test.timezone).SaveDay(context.Background(), uuid.New(), test.date, validDayRequest())
		var validation *model.ValidationError
		if test.wantErr != errors.As(err, &validation) || (test.wantErr && validation.Field != "date") {
			t.Errorf("%s %s: err=%v wantErr=%v", test.timezone, test.date, err, test.wantErr)
		}
	}
}

func TestNutritionServiceSaveDayValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*model.SaveNutritionDayRequest)
		date   string
		field  string
	}{
		{"bad date", func(*model.SaveNutritionDayRequest) {}, "25-10-2026", "date"},
		{"negative energy", func(r *model.SaveNutritionDayRequest) { r.EnergyKcal = ptrInt(-1) }, "2026-10-25", "energy_kcal"},
		{"energy over bound", func(r *model.SaveNutritionDayRequest) { r.EnergyKcal = ptrInt(20001) }, "2026-10-25", "energy_kcal"},
		{"protein over bound", func(r *model.SaveNutritionDayRequest) { r.ProteinG = ptrFloat(2000.5) }, "2026-10-25", "protein_g"},
		{"two decimals", func(r *model.SaveNutritionDayRequest) { r.CarbsG = ptrFloat(12.34) }, "2026-10-25", "carbs_g"},
		{"negative fat", func(r *model.SaveNutritionDayRequest) { r.FatG = ptrFloat(-0.1) }, "2026-10-25", "fat_g"},
		{"unknown source", func(r *model.SaveNutritionDayRequest) { r.Source = "fitbit" }, "2026-10-25", "source"},
		{"manual without revision", func(r *model.SaveNutritionDayRequest) { r.ExpectedRevision = nil }, "2026-10-25", "expected_revision"},
		{"negative revision", func(r *model.SaveNutritionDayRequest) { r.ExpectedRevision = ptrInt64(-1) }, "2026-10-25", "expected_revision"},
		{"blank key", func(r *model.SaveNutritionDayRequest) { r.OperationKey = "  " }, "2026-10-25", "operation_key"},
		{"long key", func(r *model.SaveNutritionDayRequest) { r.OperationKey = strings.Repeat("k", 129) }, "2026-10-25", "operation_key"},
		{"health without values", func(r *model.SaveNutritionDayRequest) {
			r.Source = model.NutritionSourceAppleHealth
			r.ProteinG, r.EnergyKcal = nil, nil
		}, "2026-10-25", "source"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := validDayRequest()
			test.mutate(&req)
			repo := &nutritionRepo{}
			_, _, err := newNutritionServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), test.date, req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("err=%v want field %s", err, test.field)
			}
			if repo.day != nil {
				t.Fatal("DAO must not be called on validation failure")
			}
		})
	}
}

func TestNutritionServiceSaveDayZeroAndOneDecimalAccepted(t *testing.T) {
	req := validDayRequest()
	req.EnergyKcal = ptrInt(0)
	req.ProteinG = ptrFloat(0.1)
	req.CarbsG = ptrFloat(230)
	req.FatG = ptrFloat(70.3)
	repo := &nutritionRepo{}
	if _, _, err := newNutritionServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), "2026-10-25", req); err != nil {
		t.Fatalf("zero and one-decimal values must pass: %v", err)
	}
	if *repo.day.FatG != 70.3 || *repo.day.ProteinG != 0.1 || *repo.day.EnergyKcal != 0 {
		t.Fatalf("values must be stored as sent: %+v", repo.day)
	}
}

func TestNutritionServiceSaveDayRemoveAndHealthRules(t *testing.T) {
	remove := model.SaveNutritionDayRequest{ExpectedRevision: ptrInt64(2), OperationKey: "clear"}
	repo := &nutritionRepo{outcome: model.NutritionDayRemoved}
	_, outcome, err := newNutritionServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), "2026-10-25", remove)
	if err != nil || outcome != model.NutritionDayRemoved || !repo.remove || repo.expected == nil || *repo.expected != 2 {
		t.Fatalf("all-absent manual save must remove with its revision: err=%v remove=%v expected=%v", err, repo.remove, repo.expected)
	}

	health := model.SaveNutritionDayRequest{Source: model.NutritionSourceAppleHealth, ProteinG: ptrFloat(120), ExpectedRevision: ptrInt64(7), OperationKey: "hk-nutrition:2026-10-25:202610252330"}
	repo = &nutritionRepo{outcome: model.NutritionDayIgnored}
	_, outcome, err = newNutritionServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), "2026-10-25", health)
	if err != nil || outcome != model.NutritionDayIgnored {
		t.Fatalf("outcome passthrough: err=%v outcome=%v", err, outcome)
	}
	if repo.expected != nil || repo.remove || repo.day.Source != model.NutritionSourceAppleHealth {
		t.Fatalf("a Health save ignores expected_revision and keeps its source: expected=%v source=%s", repo.expected, repo.day.Source)
	}
}

func TestNutritionServiceSaveDayHashCoversDateAndBody(t *testing.T) {
	repo := &nutritionRepo{ledger: map[string]string{}}
	svc := newNutritionServiceForTest(repo, "UTC")
	userID := uuid.New()
	if _, _, err := svc.SaveDay(context.Background(), userID, "2026-10-25", validDayRequest()); err != nil {
		t.Fatal(err)
	}
	first := repo.hash
	if _, outcome, err := svc.SaveDay(context.Background(), userID, "2026-10-25", validDayRequest()); err != nil || outcome != model.NutritionDayReplaced || repo.hash != first {
		t.Fatalf("verbatim retry must replay: err=%v outcome=%v", err, outcome)
	}
	if _, _, err := svc.SaveDay(context.Background(), userID, "2026-10-24", validDayRequest()); err == nil {
		t.Fatal("same key for a different date must conflict")
	}
	changed := validDayRequest()
	changed.ProteinG = ptrFloat(100)
	var conflict *model.ConflictError
	if _, _, err := svc.SaveDay(context.Background(), userID, "2026-10-25", changed); !errors.As(err, &conflict) {
		t.Fatalf("same key with a different body must conflict: %v", err)
	}
}

func TestNutritionServiceRequiresProfileAndValidTimezone(t *testing.T) {
	missing := &nutritionService{nutrition: &nutritionRepo{}, profiles: sportProfileRepo{err: &model.NotFoundError{Message: "training profile not found"}}, now: nutritionClock}
	var notFound *model.NotFoundError
	if _, _, err := missing.SaveDay(context.Background(), uuid.New(), "2026-10-25", validDayRequest()); !errors.As(err, &notFound) {
		t.Fatalf("missing profile must surface not found: %v", err)
	}
	if _, err := missing.GetTarget(context.Background(), uuid.New()); !errors.As(err, &notFound) {
		t.Fatalf("GetTarget without a profile must surface not found: %v", err)
	}
	var validation *model.ValidationError
	if _, _, err := newNutritionServiceForTest(&nutritionRepo{}, "Mars/Olympus").SaveDay(context.Background(), uuid.New(), "2026-10-25", validDayRequest()); !errors.As(err, &validation) || validation.Field != "timezone" {
		t.Fatalf("invalid saved timezone: %v", err)
	}
}

func TestNutritionServiceListDaysValidatesRange(t *testing.T) {
	repo := &nutritionRepo{listed: &model.NutritionRange{Days: []model.NutritionDay{}, Targets: []model.NutritionTarget{}}}
	svc := newNutritionServiceForTest(repo, "UTC")
	if got, err := svc.ListDays(context.Background(), uuid.New(), "2026-10-01", "2026-10-25"); err != nil || got == nil || got.Days == nil {
		t.Fatalf("valid range: %v %v", got, err)
	}
	var validation *model.ValidationError
	if _, err := svc.ListDays(context.Background(), uuid.New(), "2026-10-25", "2026-10-01"); !errors.As(err, &validation) {
		t.Fatalf("to before from must fail: %v", err)
	}
	if _, err := svc.ListDays(context.Background(), uuid.New(), "2025-01-01", "2026-10-25"); !errors.As(err, &validation) || (validation.Field != "to" && validation.Field != "range") {
		t.Fatalf("over 366 days must fail on the range: %v", err)
	}
}

func validTargetRequest() model.SaveNutritionTargetRequest {
	return model.SaveNutritionTargetRequest{ProteinG: 150, EnergyKcal: ptrInt(2200), ExpectedRevision: 0, OperationKey: "target-1"}
}

func TestNutritionServiceSaveTargetDefaultsEffectiveFromAndHashesRawRequest(t *testing.T) {
	repo := &nutritionRepo{ledger: map[string]string{}}
	svc := newNutritionServiceForTest(repo, "Pacific/Auckland")
	userID := uuid.New()
	got, outcome, err := svc.SaveTarget(context.Background(), userID, validTargetRequest())
	if err != nil || got == nil || outcome != model.NutritionTargetCreated {
		t.Fatalf("err=%v outcome=%v", err, outcome)
	}
	if repo.target.EffectiveFrom != "2026-10-26" || repo.today != "2026-10-26" || repo.targetRev != 0 || repo.target.ProteinG != 150 || *repo.target.EnergyKcal != 2200 || repo.target.CarbsG != nil {
		t.Fatalf("defaults not applied: %+v today=%s rev=%d", repo.target, repo.today, repo.targetRev)
	}
	first := repo.hash
	if _, outcome, err := svc.SaveTarget(context.Background(), userID, validTargetRequest()); err != nil || outcome != model.NutritionTargetUnchanged || repo.hash != first {
		t.Fatalf("verbatim retry without effective_from must replay: err=%v outcome=%v", err, outcome)
	}
	explicit := validTargetRequest()
	explicit.EffectiveFrom = "2026-10-26"
	if _, _, err := svc.SaveTarget(context.Background(), userID, explicit); err == nil {
		t.Fatal("the same key with effective_from spelled out is a different payload and must conflict")
	}
}

func TestNutritionServiceSaveTargetValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*model.SaveNutritionTargetRequest)
		field  string
	}{
		{"protein zero", func(r *model.SaveNutritionTargetRequest) { r.ProteinG = 0 }, "protein_g"},
		{"protein negative", func(r *model.SaveNutritionTargetRequest) { r.ProteinG = -5 }, "protein_g"},
		{"protein two decimals", func(r *model.SaveNutritionTargetRequest) { r.ProteinG = 150.25 }, "protein_g"},
		{"energy zero", func(r *model.SaveNutritionTargetRequest) { r.EnergyKcal = ptrInt(0) }, "energy_kcal"},
		{"carbs over bound", func(r *model.SaveNutritionTargetRequest) { r.CarbsG = ptrFloat(2001) }, "carbs_g"},
		{"future date", func(r *model.SaveNutritionTargetRequest) { r.EffectiveFrom = "2026-10-26" }, "effective_from"},
		{"bad date", func(r *model.SaveNutritionTargetRequest) { r.EffectiveFrom = "yesterday" }, "effective_from"},
		{"negative revision", func(r *model.SaveNutritionTargetRequest) { r.ExpectedRevision = -1 }, "expected_revision"},
		{"blank key", func(r *model.SaveNutritionTargetRequest) { r.OperationKey = "" }, "operation_key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := validTargetRequest()
			test.mutate(&req)
			repo := &nutritionRepo{}
			_, _, err := newNutritionServiceForTest(repo, "America/New_York").SaveTarget(context.Background(), uuid.New(), req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("err=%v want field %s", err, test.field)
			}
			if repo.target != nil {
				t.Fatal("DAO must not be called on validation failure")
			}
		})
	}
}

func TestNutritionServiceGetTargetUsesLocalToday(t *testing.T) {
	repo := &nutritionRepo{current: &model.NutritionTarget{ProteinG: 150}}
	got, err := newNutritionServiceForTest(repo, "America/New_York").GetTarget(context.Background(), uuid.New())
	if err != nil || got.ProteinG != 150 || repo.today != "2026-10-25" {
		t.Fatalf("got=%v err=%v today=%s", got, err, repo.today)
	}
	var notFound *model.NotFoundError
	if _, err := newNutritionServiceForTest(&nutritionRepo{}, "UTC").GetTarget(context.Background(), uuid.New()); !errors.As(err, &notFound) {
		t.Fatalf("no target must be not found: %v", err)
	}
}

func TestNutritionServicePassesDAOErrors(t *testing.T) {
	conflict := &model.ConflictError{Message: "resource revision conflict", Expected: 1, Actual: 2}
	repo := &nutritionRepo{err: conflict}
	if _, _, err := newNutritionServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), "2026-10-25", validDayRequest()); !errors.Is(err, conflict) {
		t.Fatalf("DAO error must pass through: %v", err)
	}
	if _, _, err := newNutritionServiceForTest(repo, "UTC").SaveTarget(context.Background(), uuid.New(), validTargetRequest()); !errors.Is(err, conflict) {
		t.Fatalf("DAO error must pass through: %v", err)
	}
	if _, err := newNutritionServiceForTest(repo, "UTC").ListDays(context.Background(), uuid.New(), "2026-10-01", "2026-10-25"); !errors.Is(err, conflict) {
		t.Fatalf("DAO error must pass through: %v", err)
	}
}
