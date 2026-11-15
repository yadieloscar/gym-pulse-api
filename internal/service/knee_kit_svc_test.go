package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// kneeKitRepo records the DAO call and can mirror the idempotency ledger.
type kneeKitRepo struct {
	day       *model.KneeKitDay
	expected  int64
	key, hash string
	calls     int
	ledger    map[string]string
	outcome   model.KneeKitDayOutcome
	err       error
	listed    []model.KneeKitDay
	from, to  string
}

func (r *kneeKitRepo) SaveDay(_ context.Context, _ uuid.UUID, day *model.KneeKitDay, expected int64, key, hash string) (*model.KneeKitDay, model.KneeKitDayOutcome, error) {
	r.day, r.expected, r.key, r.hash = day, expected, key, hash
	r.calls++
	if r.err != nil {
		return nil, model.KneeKitDayCreated, r.err
	}
	if r.ledger != nil {
		if stored, ok := r.ledger[key]; ok {
			if stored != hash {
				return nil, model.KneeKitDayCreated, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
			}
			return day, model.KneeKitDayCreated, nil
		}
		r.ledger[key] = hash
	}
	return day, r.outcome, nil
}

func (r *kneeKitRepo) ListDays(_ context.Context, _ uuid.UUID, from, to string) ([]model.KneeKitDay, error) {
	r.from, r.to = from, to
	return r.listed, r.err
}

// The shared nutrition clock: 2026-10-25 23:30 UTC is already the 26th in
// Auckland and still the 25th in New York.
func newKneeKitServiceForTest(repo *kneeKitRepo, timezone string) *kneeKitService {
	return &kneeKitService{days: repo, profiles: sportProfileRepo{profile: &model.TrainingProfile{Timezone: timezone}}, now: nutritionClock}
}

func validKneeKitRequest() model.SaveKneeKitDayRequest {
	return model.SaveKneeKitDayRequest{
		CompletedItems:   []string{"morning-check", "isometric-hold"},
		MorningPain:      ptrInt(3),
		ExpectedRevision: ptrInt64(0),
		OperationKey:     " knee-day-1 ",
	}
}

func TestKneeKitServiceSaveDayNormalizesAndSortsItems(t *testing.T) {
	repo := &kneeKitRepo{outcome: model.KneeKitDayCreated}
	req := validKneeKitRequest()
	sent := slices.Clone(req.CompletedItems)
	got, outcome, err := newKneeKitServiceForTest(repo, "America/New_York").SaveDay(context.Background(), uuid.New(), "2026-10-25", req)
	if err != nil || got == nil || outcome != model.KneeKitDayCreated {
		t.Fatalf("err=%v outcome=%v", err, outcome)
	}
	if repo.key != "knee-day-1" || repo.hash == "" || repo.expected != 0 || repo.day.Date != "2026-10-25" {
		t.Fatalf("not normalized: key=%q expected=%d day=%+v", repo.key, repo.expected, repo.day)
	}
	if !slices.Equal(repo.day.CompletedItems, []string{"isometric-hold", "morning-check"}) {
		t.Fatalf("items must be stored sorted: %v", repo.day.CompletedItems)
	}
	if !slices.Equal(req.CompletedItems, sent) {
		t.Fatalf("the caller's slice must not be reordered: %v", req.CompletedItems)
	}
	if *repo.day.MorningPain != 3 || repo.day.ActivityPain != nil || repo.day.SharpPain || repo.day.Limping {
		t.Fatalf("values not carried: %+v", repo.day)
	}
}

func TestKneeKitServiceSaveDayStoresAnEmptyDay(t *testing.T) {
	// There is no delete-by-empty: unticking everything is still a record.
	repo := &kneeKitRepo{outcome: model.KneeKitDayReplaced}
	req := model.SaveKneeKitDayRequest{CompletedItems: []string{}, ExpectedRevision: ptrInt64(4), OperationKey: "knee-clear"}
	_, outcome, err := newKneeKitServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), "2026-10-25", req)
	if err != nil || outcome != model.KneeKitDayReplaced || repo.calls != 1 {
		t.Fatalf("err=%v outcome=%v calls=%d", err, outcome, repo.calls)
	}
	if repo.day.CompletedItems == nil || len(repo.day.CompletedItems) != 0 || repo.day.MorningPain != nil || repo.expected != 4 {
		t.Fatalf("an empty day must be written as an empty array: %+v expected=%d", repo.day, repo.expected)
	}
}

func TestKneeKitServiceSaveDayLocalTodayFollowsProfileTimezone(t *testing.T) {
	for _, test := range []struct {
		timezone, date string
		wantErr        bool
	}{
		{"America/New_York", "2026-10-25", false},
		{"America/New_York", "2026-10-26", true},
		{"Pacific/Auckland", "2026-10-26", false},
		{"Pacific/Auckland", "2026-10-27", true},
		{"UTC", "2020-01-01", false},
	} {
		_, _, err := newKneeKitServiceForTest(&kneeKitRepo{}, test.timezone).SaveDay(context.Background(), uuid.New(), test.date, validKneeKitRequest())
		var validation *model.ValidationError
		if test.wantErr != errors.As(err, &validation) || (test.wantErr && validation.Field != "date") {
			t.Errorf("%s %s: err=%v wantErr=%v", test.timezone, test.date, err, test.wantErr)
		}
	}
}

func TestKneeKitServiceSaveDayValidation(t *testing.T) {
	thirteen := make([]string, 13)
	for i := range thirteen {
		thirteen[i] = "item-" + string(rune('a'+i))
	}
	for _, test := range []struct {
		name   string
		mutate func(*model.SaveKneeKitDayRequest)
		date   string
		field  string
	}{
		{"bad date", func(*model.SaveKneeKitDayRequest) {}, "25-10-2026", "date"},
		{"impossible date", func(*model.SaveKneeKitDayRequest) {}, "2026-02-30", "date"},
		{"items absent", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = nil }, "2026-10-25", "completed_items"},
		{"too many items", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = thirteen }, "2026-10-25", "completed_items"},
		{"empty item", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{""} }, "2026-10-25", "completed_items"},
		{"uppercase item", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{"Morning-Check"} }, "2026-10-25", "completed_items"},
		{"underscore item", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{"morning_check"} }, "2026-10-25", "completed_items"},
		{"trailing hyphen", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{"morning-"} }, "2026-10-25", "completed_items"},
		{"padded item", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{" morning-check"} }, "2026-10-25", "completed_items"},
		{"trailing newline", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{"morning-check\n"} }, "2026-10-25", "completed_items"},
		{"long item", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{strings.Repeat("a", 65)} }, "2026-10-25", "completed_items"},
		{"duplicate item", func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{"wall-sit", "wall-sit"} }, "2026-10-25", "completed_items"},
		{"negative morning pain", func(r *model.SaveKneeKitDayRequest) { r.MorningPain = ptrInt(-1) }, "2026-10-25", "morning_pain"},
		{"morning pain over bound", func(r *model.SaveKneeKitDayRequest) { r.MorningPain = ptrInt(11) }, "2026-10-25", "morning_pain"},
		{"activity pain over bound", func(r *model.SaveKneeKitDayRequest) { r.ActivityPain = ptrInt(11) }, "2026-10-25", "activity_pain"},
		{"revision absent", func(r *model.SaveKneeKitDayRequest) { r.ExpectedRevision = nil }, "2026-10-25", "expected_revision"},
		{"negative revision", func(r *model.SaveKneeKitDayRequest) { r.ExpectedRevision = ptrInt64(-1) }, "2026-10-25", "expected_revision"},
		{"blank key", func(r *model.SaveKneeKitDayRequest) { r.OperationKey = "  " }, "2026-10-25", "operation_key"},
		{"long key", func(r *model.SaveKneeKitDayRequest) { r.OperationKey = strings.Repeat("k", 129) }, "2026-10-25", "operation_key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := validKneeKitRequest()
			test.mutate(&req)
			repo := &kneeKitRepo{}
			_, _, err := newKneeKitServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), test.date, req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("err=%v want field %s", err, test.field)
			}
			if repo.calls != 0 {
				t.Fatal("DAO must not be called on validation failure")
			}
		})
	}
}

func TestKneeKitServiceSaveDayAcceptsBounds(t *testing.T) {
	twelve := make([]string, model.KneeKitMaxCompletedItems)
	for i := range twelve {
		twelve[i] = "item-" + string(rune('a'+i))
	}
	twelve[0] = strings.Repeat("a", 64)
	req := model.SaveKneeKitDayRequest{CompletedItems: twelve, MorningPain: ptrInt(0), ActivityPain: ptrInt(10), SharpPain: true, Limping: true,
		ExpectedRevision: ptrInt64(0), OperationKey: strings.Repeat("k", 128)}
	repo := &kneeKitRepo{}
	if _, _, err := newKneeKitServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), "2026-10-25", req); err != nil {
		t.Fatalf("boundary values must pass: %v", err)
	}
	if len(repo.day.CompletedItems) != 12 || *repo.day.MorningPain != 0 || *repo.day.ActivityPain != 10 || !repo.day.SharpPain || !repo.day.Limping {
		t.Fatalf("values must be stored as sent: %+v", repo.day)
	}
}

func TestKneeKitServiceSaveDayHashCoversDateAndBodyButNotItemOrder(t *testing.T) {
	repo := &kneeKitRepo{ledger: map[string]string{}}
	svc := newKneeKitServiceForTest(repo, "UTC")
	userID := uuid.New()
	if _, _, err := svc.SaveDay(context.Background(), userID, "2026-10-25", validKneeKitRequest()); err != nil {
		t.Fatal(err)
	}
	first := repo.hash
	if _, _, err := svc.SaveDay(context.Background(), userID, "2026-10-25", validKneeKitRequest()); err != nil || repo.hash != first {
		t.Fatalf("verbatim retry must replay: err=%v", err)
	}
	reordered := validKneeKitRequest()
	slices.Reverse(reordered.CompletedItems)
	if _, _, err := svc.SaveDay(context.Background(), userID, "2026-10-25", reordered); err != nil || repo.hash != first {
		t.Fatalf("the same items in another order are the same payload: err=%v", err)
	}
	var conflict *model.ConflictError
	if _, _, err := svc.SaveDay(context.Background(), userID, "2026-10-24", validKneeKitRequest()); !errors.As(err, &conflict) {
		t.Fatalf("same key for a different date must conflict: %v", err)
	}
	for name, mutate := range map[string]func(*model.SaveKneeKitDayRequest){
		"items":    func(r *model.SaveKneeKitDayRequest) { r.CompletedItems = []string{"morning-check"} },
		"pain":     func(r *model.SaveKneeKitDayRequest) { r.MorningPain = ptrInt(4) },
		"no pain":  func(r *model.SaveKneeKitDayRequest) { r.MorningPain = nil },
		"flag":     func(r *model.SaveKneeKitDayRequest) { r.Limping = true },
		"revision": func(r *model.SaveKneeKitDayRequest) { r.ExpectedRevision = ptrInt64(1) },
	} {
		changed := validKneeKitRequest()
		mutate(&changed)
		if _, _, err := svc.SaveDay(context.Background(), userID, "2026-10-25", changed); !errors.As(err, &conflict) {
			t.Errorf("same key with a different %s must conflict: %v", name, err)
		}
	}
}

func TestKneeKitServiceRequiresProfileAndValidTimezone(t *testing.T) {
	repo := &kneeKitRepo{listed: []model.KneeKitDay{}}
	missing := &kneeKitService{days: repo, profiles: sportProfileRepo{err: &model.NotFoundError{Message: "training profile not found"}}, now: nutritionClock}
	var notFound *model.NotFoundError
	if _, _, err := missing.SaveDay(context.Background(), uuid.New(), "2026-10-25", validKneeKitRequest()); !errors.As(err, &notFound) || repo.calls != 0 {
		t.Fatalf("missing profile must surface not found before any write: %v", err)
	}
	// The range read needs no training profile.
	if got, err := missing.ListDays(context.Background(), uuid.New(), "2026-10-01", "2026-10-25"); err != nil || got == nil {
		t.Fatalf("range read without a profile: %v %v", got, err)
	}
	var validation *model.ValidationError
	if _, _, err := newKneeKitServiceForTest(&kneeKitRepo{}, "Mars/Olympus").SaveDay(context.Background(), uuid.New(), "2026-10-25", validKneeKitRequest()); !errors.As(err, &validation) || validation.Field != "timezone" {
		t.Fatalf("invalid saved timezone: %v", err)
	}
}

func TestKneeKitServiceListDaysValidatesRangeAndNeverReturnsNull(t *testing.T) {
	repo := &kneeKitRepo{}
	svc := newKneeKitServiceForTest(repo, "UTC")
	got, err := svc.ListDays(context.Background(), uuid.New(), "2026-10-01", "2026-10-25")
	if err != nil || got == nil || got.Days == nil || len(got.Days) != 0 || repo.from != "2026-10-01" || repo.to != "2026-10-25" {
		t.Fatalf("valid range: %+v %v from=%s to=%s", got, err, repo.from, repo.to)
	}
	repo.listed = []model.KneeKitDay{{Date: "2026-10-02"}, {Date: "2026-10-20"}}
	if got, err := svc.ListDays(context.Background(), uuid.New(), "2026-10-01", "2026-10-25"); err != nil || len(got.Days) != 2 || got.Days[0].Date != "2026-10-02" {
		t.Fatalf("days must pass through in DAO order: %+v %v", got, err)
	}
	var validation *model.ValidationError
	for _, bad := range [][2]string{{"2026-10-25", "2026-10-01"}, {"2025-01-01", "2026-10-25"}, {"", "2026-10-25"}, {"2026-10-01", ""}, {"10/01/2026", "2026-10-25"}} {
		if _, err := svc.ListDays(context.Background(), uuid.New(), bad[0], bad[1]); !errors.As(err, &validation) {
			t.Errorf("range %v must fail validation: %v", bad, err)
		}
	}
}

func TestKneeKitServicePassesDAOErrors(t *testing.T) {
	conflict := &model.ConflictError{Message: "resource revision conflict", Expected: 1, Actual: 2}
	repo := &kneeKitRepo{err: conflict}
	if _, _, err := newKneeKitServiceForTest(repo, "UTC").SaveDay(context.Background(), uuid.New(), "2026-10-25", validKneeKitRequest()); !errors.Is(err, conflict) {
		t.Fatalf("DAO error must pass through: %v", err)
	}
	if _, err := newKneeKitServiceForTest(repo, "UTC").ListDays(context.Background(), uuid.New(), "2026-10-01", "2026-10-25"); !errors.Is(err, conflict) {
		t.Fatalf("DAO error must pass through: %v", err)
	}
}

func TestNewKneeKitServiceUsesTheWallClock(t *testing.T) {
	svc, ok := NewKneeKitService(&kneeKitRepo{}, sportProfileRepo{}).(*kneeKitService)
	if !ok || svc.now == nil || svc.days == nil {
		t.Fatalf("constructor wiring: %+v", svc)
	}
}
