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

type fakeSavedMealDAO struct {
	meals []model.SavedMeal
	err   error

	created, updated *model.SaveMealRequest
	updatedID        uuid.UUID
	deletedID        uuid.UUID
	deletedRevision  int64
	loggedMeal       uuid.UUID
	loggedDate       string
	loggedItems      []model.MealItemInput
	loggedSection    *model.MealSection
	loggedAt         time.Time
	key, hash        string
	calls            int
}

func (d *fakeSavedMealDAO) ListMeals(context.Context, uuid.UUID) ([]model.SavedMeal, error) {
	return d.meals, d.err
}
func (d *fakeSavedMealDAO) CreateMeal(_ context.Context, _ uuid.UUID, req model.SaveMealRequest, key, hash string) (*model.SavedMeal, model.SavedMealOutcome, error) {
	d.calls++
	d.created, d.key, d.hash = &req, key, hash
	return &model.SavedMeal{Name: req.Name}, model.SavedMealCreated, d.err
}
func (d *fakeSavedMealDAO) UpdateMeal(_ context.Context, _, id uuid.UUID, req model.SaveMealRequest, key, hash string) (*model.SavedMeal, model.SavedMealOutcome, error) {
	d.calls++
	d.updated, d.updatedID, d.key, d.hash = &req, id, key, hash
	return &model.SavedMeal{Name: req.Name}, model.SavedMealReplaced, d.err
}
func (d *fakeSavedMealDAO) DeleteMeal(_ context.Context, _, id uuid.UUID, revision int64, key, hash string) (model.SavedMealOutcome, error) {
	d.calls++
	d.deletedID, d.deletedRevision, d.key, d.hash = id, revision, key, hash
	return model.SavedMealRemoved, d.err
}
func (d *fakeSavedMealDAO) LogMeal(_ context.Context, _, mealID uuid.UUID, date string, items []model.MealItemInput, section *model.MealSection, loggedAt time.Time, key, hash string) (*model.MealLogged, model.SavedMealOutcome, error) {
	d.calls++
	d.loggedMeal, d.loggedDate, d.loggedItems, d.loggedSection, d.loggedAt, d.key, d.hash = mealID, date, items, section, loggedAt, key, hash
	return &model.MealLogged{Entries: []model.FoodEntry{}, Converted: []model.FoodEntry{}}, model.SavedMealLogged, d.err
}

var savedMealTestNow = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func newSavedMealServiceForTest(meals *fakeSavedMealDAO) *savedMealService {
	return &savedMealService{meals: meals, profiles: &fakeProfileDAO{tz: "UTC"}, now: func() time.Time { return savedMealTestNow }}
}

func mealItem(grams, amount float64) model.MealItemInput {
	return model.MealItemInput{Food: model.FoodRef{ID: uuid.New()}, Serving: model.Serving{Label: "1 cup", Grams: grams}, Amount: amount}
}

func validMealRequest() model.SaveMealRequest {
	return model.SaveMealRequest{Name: "Usual breakfast", Items: []model.MealItemInput{mealItem(81, 1), mealItem(30, 1.5)}, OperationKey: "meal:1"}
}

// wantFieldIndex asserts a validation error on field, and on the item index
// when index is not negative.
func wantFieldIndex(t *testing.T, label string, err error, field string, index int) {
	t.Helper()
	var validation *model.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("%s: want a validation error, got %v", label, err)
	}
	if validation.Field != field {
		t.Fatalf("%s: field = %q, want %q (%s)", label, validation.Field, field, validation.Message)
	}
	switch {
	case index < 0 && validation.Index != nil:
		t.Fatalf("%s: unexpected item index %d", label, *validation.Index)
	case index >= 0 && (validation.Index == nil || *validation.Index != index):
		t.Fatalf("%s: item index = %v, want %d", label, validation.Index, index)
	}
}

func TestCreateMealValidation(t *testing.T) {
	twentyOne := make([]model.MealItemInput, 21)
	for i := range twentyOne {
		twentyOne[i] = mealItem(100, 1)
	}
	duplicate := mealItem(81, 1)
	revision := int64(1)
	for _, test := range []struct {
		label  string
		change func(*model.SaveMealRequest)
		field  string
		index  int
	}{
		{"blank name", func(r *model.SaveMealRequest) { r.Name = "   " }, "name", -1},
		{"61 character name", func(r *model.SaveMealRequest) { r.Name = strings.Repeat("a", 61) }, "name", -1},
		{"no items", func(r *model.SaveMealRequest) { r.Items = nil }, "items", -1},
		{"21 items", func(r *model.SaveMealRequest) { r.Items = twentyOne }, "items", -1},
		{"missing food", func(r *model.SaveMealRequest) { r.Items[1].Food.ID = uuid.Nil }, "items", 1},
		{"blank serving label", func(r *model.SaveMealRequest) { r.Items[0].Serving.Label = " " }, "items", 0},
		{"zero grams", func(r *model.SaveMealRequest) { r.Items[1].Serving.Grams = 0 }, "items", 1},
		{"grams that store as zero", func(r *model.SaveMealRequest) { r.Items[1].Serving.Grams = 0.04 }, "items", 1},
		{"negative grams", func(r *model.SaveMealRequest) { r.Items[0].Serving.Grams = -5 }, "items", 0},
		{"same food and serving once stored to one decimal", func(r *model.SaveMealRequest) {
			a, b := duplicate, duplicate
			a.Serving, b.Serving = model.Serving{Label: "100 g", Grams: 100}, model.Serving{Label: "100 g", Grams: 100.04}
			r.Items = []model.MealItemInput{a, b}
		}, "items", 1},
		{"grams above 5000", func(r *model.SaveMealRequest) { r.Items[1].Serving.Grams = 5000.1 }, "items", 1},
		{"amount below 0.1", func(r *model.SaveMealRequest) { r.Items[0].Amount = 0.05 }, "items", 0},
		{"amount above 50", func(r *model.SaveMealRequest) { r.Items[1].Amount = 50.01 }, "items", 1},
		{"amount with three decimals", func(r *model.SaveMealRequest) { r.Items[1].Amount = 1.234 }, "items", 1},
		{"same food and serving twice", func(r *model.SaveMealRequest) { r.Items = []model.MealItemInput{duplicate, mealItem(30, 1), duplicate} }, "items", 2},
		{"blank operation key", func(r *model.SaveMealRequest) { r.OperationKey = " " }, "operation_key", -1},
		{"unknown heading", func(r *model.SaveMealRequest) { brunch := model.MealSection("brunch"); r.Section = &brunch }, "section", -1},
		{"revision on create", func(r *model.SaveMealRequest) { r.ExpectedRevision = &revision }, "expected_revision", -1},
	} {
		repo := &fakeSavedMealDAO{}
		req := validMealRequest()
		test.change(&req)
		_, _, err := newSavedMealServiceForTest(repo).CreateMeal(context.Background(), uuid.New(), req)
		wantFieldIndex(t, test.label, err, test.field, test.index)
		if repo.calls != 0 {
			t.Fatalf("%s: an invalid request must not reach the DAO", test.label)
		}
	}
}

func TestCreateMealNormalizesAndHashes(t *testing.T) {
	repo := &fakeSavedMealDAO{}
	svc := newSavedMealServiceForTest(repo)
	req := validMealRequest()
	req.Name, req.OperationKey = "  Usual breakfast ", " meal:1 "
	sameFoodOtherServing := req.Items[0]
	sameFoodOtherServing.Serving = model.Serving{Label: "100 g", Grams: 100}
	req.Items = append(req.Items, sameFoodOtherServing)
	meal, outcome, err := svc.CreateMeal(context.Background(), uuid.New(), req)
	if err != nil || outcome != model.SavedMealCreated || meal.Name != "Usual breakfast" {
		t.Fatalf("create: %v %v %+v", err, outcome, meal)
	}
	if repo.created.Name != "Usual breakfast" || repo.key != "meal:1" || len(repo.created.Items) != 3 {
		t.Fatalf("the DAO gets the trimmed request, and one food with two servings is allowed: %+v key %q", repo.created, repo.key)
	}
	first := repo.hash
	if _, _, err := svc.CreateMeal(context.Background(), uuid.New(), req); err != nil || repo.hash != first {
		t.Fatalf("a verbatim retry hashes the same: %v", err)
	}
	req.Items[0].Amount = 2
	if _, _, err := svc.CreateMeal(context.Background(), uuid.New(), req); err != nil || repo.hash == first {
		t.Fatalf("a different payload hashes differently: %v", err)
	}
}

func TestUpdateAndDeleteMealRequireARevision(t *testing.T) {
	repo := &fakeSavedMealDAO{}
	svc := newSavedMealServiceForTest(repo)
	id := uuid.New()
	req := validMealRequest()
	_, _, err := svc.UpdateMeal(context.Background(), uuid.New(), id, req)
	wantFieldIndex(t, "update without revision", err, "expected_revision", -1)
	zero := int64(0)
	req.ExpectedRevision = &zero
	_, _, err = svc.UpdateMeal(context.Background(), uuid.New(), id, req)
	wantFieldIndex(t, "update with revision 0", err, "expected_revision", -1)
	two := int64(2)
	req.ExpectedRevision = &two
	if _, outcome, err := svc.UpdateMeal(context.Background(), uuid.New(), id, req); err != nil || outcome != model.SavedMealReplaced || repo.updatedID != id || *repo.updated.ExpectedRevision != 2 {
		t.Fatalf("update passes the id and revision through: %v %v", err, outcome)
	}
	updateHash := repo.hash
	if _, _, err := svc.UpdateMeal(context.Background(), uuid.New(), uuid.New(), req); err != nil || repo.hash == updateHash {
		t.Fatalf("the hash covers the meal id: %v", err)
	}

	_, err = svc.DeleteMeal(context.Background(), uuid.New(), id, 0, "del:1")
	wantFieldIndex(t, "delete with revision 0", err, "expected_revision", -1)
	_, err = svc.DeleteMeal(context.Background(), uuid.New(), id, 1, "  ")
	wantFieldIndex(t, "delete without key", err, "operation_key", -1)
	if outcome, err := svc.DeleteMeal(context.Background(), uuid.New(), id, 3, " del:1 "); err != nil || outcome != model.SavedMealRemoved || repo.deletedID != id || repo.deletedRevision != 3 || repo.key != "del:1" {
		t.Fatalf("delete passes id, revision, and trimmed key: %v %+v", err, repo)
	}
}

func TestLogMealValidation(t *testing.T) {
	mealID := uuid.New()
	valid := func() model.LogMealRequest {
		return model.LogMealRequest{Meal: &model.MealRef{ID: mealID}, OperationKey: "log:1"}
	}
	for _, test := range []struct {
		label  string
		date   string
		change func(*model.LogMealRequest)
		field  string
		index  int
	}{
		{"malformed date", "15-11-2026", func(*model.LogMealRequest) {}, "date", -1},
		{"future date", "2026-11-16", func(*model.LogMealRequest) {}, "date", -1},
		{"missing meal", "2026-11-15", func(r *model.LogMealRequest) { r.Meal = nil }, "meal", -1},
		{"nil meal id", "2026-11-15", func(r *model.LogMealRequest) { r.Meal = &model.MealRef{} }, "meal", -1},
		{"blank key", "2026-11-15", func(r *model.LogMealRequest) { r.OperationKey = "" }, "operation_key", -1},
		{"unknown heading", "2026-11-15", func(r *model.LogMealRequest) { other := model.MealSection("Breakfast"); r.Section = &other }, "section", -1},
		{"empty override list", "2026-11-15", func(r *model.LogMealRequest) { r.Items = []model.MealItemInput{} }, "items", -1},
		{"bad override amount", "2026-11-15", func(r *model.LogMealRequest) { r.Items = []model.MealItemInput{mealItem(81, 1), mealItem(30, 51)} }, "items", 1},
		{"override grams that store as zero", "2026-11-15", func(r *model.LogMealRequest) { r.Items = []model.MealItemInput{mealItem(0.04, 1)} }, "items", 0},
	} {
		repo := &fakeSavedMealDAO{}
		req := valid()
		test.change(&req)
		_, _, err := newSavedMealServiceForTest(repo).LogMeal(context.Background(), uuid.New(), test.date, req)
		wantFieldIndex(t, test.label, err, test.field, test.index)
		if repo.calls != 0 {
			t.Fatalf("%s: an invalid request must not reach the DAO", test.label)
		}
	}
}

func TestLogMealPassesSavedOrOverrideItems(t *testing.T) {
	repo := &fakeSavedMealDAO{}
	svc := newSavedMealServiceForTest(repo)
	mealID := uuid.New()
	req := model.LogMealRequest{Meal: &model.MealRef{ID: mealID}, OperationKey: "log:1"}
	logged, outcome, err := svc.LogMeal(context.Background(), uuid.New(), "2026-11-15", req)
	if err != nil || outcome != model.SavedMealLogged || logged == nil {
		t.Fatalf("log as saved: %v %v", err, outcome)
	}
	if repo.loggedMeal != mealID || repo.loggedDate != "2026-11-15" || repo.loggedItems != nil || !repo.loggedAt.Equal(savedMealTestNow) {
		t.Fatalf("as saved: nil items mean the saved list, logged now: %+v", repo)
	}
	asSaved := repo.hash

	at := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	req.Items, req.LoggedAt = []model.MealItemInput{mealItem(81, 2)}, &at
	if _, _, err := svc.LogMeal(context.Background(), uuid.New(), "2026-11-14", req); err != nil {
		t.Fatalf("log an earlier day with an override list: %v", err)
	}
	if len(repo.loggedItems) != 1 || repo.loggedItems[0].Amount != 2 || !repo.loggedAt.Equal(at) || repo.hash == asSaved {
		t.Fatalf("override items and logged_at pass through and change the hash: %+v", repo)
	}
}

func TestMealItemsKeepFractionalCatalogWeights(t *testing.T) {
	// Source servings are not rounded (a USDA ounce is 28.35 g), so a
	// two-decimal weight must pass through to the DAO untouched.
	repo := &fakeSavedMealDAO{}
	req := validMealRequest()
	req.Items[0].Serving = model.Serving{Label: "1 oz (28.35 g)", Grams: 28.35}
	if _, _, err := newSavedMealServiceForTest(repo).CreateMeal(context.Background(), uuid.New(), req); err != nil {
		t.Fatalf("a fractional source weight is valid: %v", err)
	}
	if repo.created.Items[0].Serving.Grams != 28.35 {
		t.Fatalf("the weight reaches the DAO exact: %v", repo.created.Items[0].Serving.Grams)
	}
}

func TestMealHeadingsPassThrough(t *testing.T) {
	repo := &fakeSavedMealDAO{}
	svc := newSavedMealServiceForTest(repo)
	breakfast, dinner := model.MealSectionBreakfast, model.MealSectionDinner
	req := validMealRequest()
	req.Section = &breakfast
	if _, _, err := svc.CreateMeal(context.Background(), uuid.New(), req); err != nil || repo.created.Section == nil || *repo.created.Section != model.MealSectionBreakfast {
		t.Fatalf("a saved meal may remember a heading: %v %+v", err, repo.created)
	}
	withHeading := repo.hash
	req.Section = nil
	if _, _, err := svc.CreateMeal(context.Background(), uuid.New(), req); err != nil || repo.created.Section != nil || repo.hash == withHeading {
		t.Fatalf("no heading is valid and hashes differently: %v", err)
	}
	log := model.LogMealRequest{Meal: &model.MealRef{ID: uuid.New()}, OperationKey: "log:1"}
	if _, _, err := svc.LogMeal(context.Background(), uuid.New(), "2026-11-15", log); err != nil || repo.loggedSection != nil {
		t.Fatalf("a log that names no heading leaves the choice to the meal: %v", err)
	}
	log.Section = &dinner
	if _, _, err := svc.LogMeal(context.Background(), uuid.New(), "2026-11-15", log); err != nil || repo.loggedSection == nil || *repo.loggedSection != model.MealSectionDinner {
		t.Fatalf("a log may name its own heading: %v", err)
	}
}

func TestListMealsNeverReturnsNull(t *testing.T) {
	list, err := newSavedMealServiceForTest(&fakeSavedMealDAO{}).ListMeals(context.Background(), uuid.New())
	if err != nil || list.Meals == nil || len(list.Meals) != 0 {
		t.Fatalf("an athlete with no meals gets an empty list: %v %+v", err, list)
	}
	boom := errors.New("boom")
	if _, err := newSavedMealServiceForTest(&fakeSavedMealDAO{err: boom}).ListMeals(context.Background(), uuid.New()); !errors.Is(err, boom) {
		t.Fatalf("DAO errors pass through: %v", err)
	}
}
