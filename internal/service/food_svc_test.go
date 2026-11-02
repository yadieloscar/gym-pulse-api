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

type fakeFoodDAO struct {
	own          []model.Food
	catalog      []model.Food
	byGTIN       *model.Food
	ownByGTIN    *model.Food
	foods        map[uuid.UUID]*model.Food
	recentMiss   bool
	upserted     [][]model.Food
	misses       []string
	calls        []string
	userFoodReq  model.SaveUserFoodRequest
	userFoodGTIN *string
}

func (f *fakeFoodDAO) UpsertCatalogFoods(_ context.Context, foods []model.Food) ([]model.Food, error) {
	f.calls = append(f.calls, "upsert")
	f.upserted = append(f.upserted, foods)
	out := make([]model.Food, len(foods))
	for i, food := range foods {
		if food.ID == uuid.Nil {
			food.ID = uuid.New()
		}
		out[i] = food
	}
	return out, nil
}
func (f *fakeFoodDAO) SearchCatalog(context.Context, string, int) ([]model.Food, error) {
	f.calls = append(f.calls, "catalog-search")
	return f.catalog, nil
}
func (f *fakeFoodDAO) FindCatalogByGTIN(context.Context, string) (*model.Food, bool, error) {
	f.calls = append(f.calls, "catalog-gtin")
	return f.byGTIN, f.byGTIN != nil, nil
}
func (f *fakeFoodDAO) SearchUserFoods(context.Context, uuid.UUID, string, int) ([]model.Food, error) {
	f.calls = append(f.calls, "own-search")
	return f.own, nil
}
func (f *fakeFoodDAO) FindUserFoodByGTIN(context.Context, uuid.UUID, string) (*model.Food, bool, error) {
	f.calls = append(f.calls, "own-gtin")
	return f.ownByGTIN, f.ownByGTIN != nil, nil
}
func (f *fakeFoodDAO) GetFood(_ context.Context, _ uuid.UUID, id uuid.UUID) (*model.Food, error) {
	if food, ok := f.foods[id]; ok {
		return food, nil
	}
	return nil, &model.NotFoundError{Message: "food not found"}
}
func (f *fakeFoodDAO) ListFoodsByIDs(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]model.Food, error) {
	return nil, nil
}
func (f *fakeFoodDAO) RecordBarcodeMiss(_ context.Context, gtin string, _ time.Time) error {
	f.calls = append(f.calls, "miss-record")
	f.misses = append(f.misses, gtin)
	return nil
}
func (f *fakeFoodDAO) IsRecentMiss(context.Context, string, time.Time) (bool, error) {
	f.calls = append(f.calls, "miss-check")
	return f.recentMiss, nil
}
func (f *fakeFoodDAO) ListRecentFoods(context.Context, uuid.UUID, time.Time, int) ([]model.RecentFood, error) {
	return nil, nil
}
func (f *fakeFoodDAO) CreateUserFood(_ context.Context, _ uuid.UUID, req model.SaveUserFoodRequest, gtin *string, _, _ string) (*model.Food, model.UserFoodOutcome, error) {
	f.userFoodReq, f.userFoodGTIN = req, gtin
	return &model.Food{Source: model.FoodSourceOwn}, model.UserFoodCreated, nil
}
func (f *fakeFoodDAO) UpdateUserFood(_ context.Context, _, _ uuid.UUID, req model.SaveUserFoodRequest, gtin *string, _, _ string) (*model.Food, model.UserFoodOutcome, error) {
	f.userFoodReq, f.userFoodGTIN = req, gtin
	return &model.Food{Source: model.FoodSourceOwn}, model.UserFoodReplaced, nil
}
func (f *fakeFoodDAO) ArchiveUserFood(context.Context, uuid.UUID, uuid.UUID, int64, string, string) (model.UserFoodOutcome, error) {
	return model.UserFoodArchived, nil
}

type fakeEntryDAO struct {
	entry  *model.FoodEntry
	hashes []string
	change model.UpdateFoodEntryRequest
}

func (f *fakeEntryDAO) ListDayEntries(context.Context, uuid.UUID, string) (*model.DayEntries, error) {
	return &model.DayEntries{Entries: []model.FoodEntry{}}, nil
}
func (f *fakeEntryDAO) CreateEntry(_ context.Context, _ uuid.UUID, entry *model.FoodEntry, _, hash string) (*model.FoodEntryCreated, model.FoodEntryOutcome, error) {
	f.entry, f.hashes = entry, append(f.hashes, hash)
	return &model.FoodEntryCreated{Entry: *entry, Converted: []model.FoodEntry{}}, model.FoodEntryCreatedOutcome, nil
}
func (f *fakeEntryDAO) UpdateEntry(_ context.Context, _, _ uuid.UUID, change model.UpdateFoodEntryRequest, _, hash string) (*model.FoodEntryUpdated, model.FoodEntryOutcome, error) {
	f.change, f.hashes = change, append(f.hashes, hash)
	return &model.FoodEntryUpdated{}, model.FoodEntryReplacedOutcome, nil
}
func (f *fakeEntryDAO) DeleteEntry(_ context.Context, _, _ uuid.UUID, _ int64, _, hash string) (*model.FoodEntryDeleted, model.FoodEntryOutcome, error) {
	f.hashes = append(f.hashes, hash)
	return &model.FoodEntryDeleted{}, model.FoodEntryRemovedOutcome, nil
}

type fakeSource struct {
	name    string
	search  []model.Food
	barcode *model.Food
	err     error
	calls   *[]string
}

func (s *fakeSource) Search(context.Context, string, int) ([]model.Food, error) {
	*s.calls = append(*s.calls, s.name+"-search")
	return s.search, s.err
}
func (s *fakeSource) LookupBarcode(context.Context, string) (*model.Food, bool, error) {
	*s.calls = append(*s.calls, s.name+"-barcode")
	return s.barcode, s.barcode != nil, s.err
}

type fakeProfileDAO struct{ tz string }

func (p *fakeProfileDAO) Get(context.Context, uuid.UUID) (*model.TrainingProfile, error) {
	return &model.TrainingProfile{Timezone: p.tz}, nil
}
func (p *fakeProfileDAO) Put(context.Context, uuid.UUID, *model.TrainingProfile, int64) error {
	return nil
}

func newFoodServiceForTest(foods *fakeFoodDAO, entries *fakeEntryDAO, usda, off *fakeSource) (*foodService, *[]string) {
	calls := &[]string{}
	usda.calls, off.calls = calls, calls
	svc := &foodService{foods: foods, entries: entries, profiles: &fakeProfileDAO{tz: "UTC"}, usda: usda, off: off,
		now: func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }}
	return svc, calls
}

func catalogFood(source model.FoodSource, sourceID, name string) model.Food {
	return model.Food{ID: uuid.New(), Source: source, SourceID: sourceID, Name: name, Servings: []model.Serving{model.HundredGramServing}, Per100g: model.Per100g{EnergyKcal: 100, ProteinG: 10, CarbsG: 5, FatG: 2}}
}

func TestFoodSearchValidationAndMerge(t *testing.T) {
	own := model.Food{ID: uuid.New(), Source: model.FoodSourceOwn, Name: "My shake"}
	usdaA := catalogFood(model.FoodSourceUSDA, "1", "Chicken A")
	usdaB := catalogFood(model.FoodSourceUSDA, "2", "Chicken B")
	foods := &fakeFoodDAO{own: []model.Food{own}, catalog: []model.Food{usdaB, catalogFood(model.FoodSourceOpenFoodFacts, "9", "Chicken OFF")}}
	svc, calls := newFoodServiceForTest(foods, &fakeEntryDAO{}, &fakeSource{name: "usda", search: []model.Food{usdaA, usdaB}}, &fakeSource{name: "off"})
	for _, test := range []struct {
		q     string
		limit int
		field string
	}{{"c", 25, "q"}, {strings.Repeat("x", 101), 25, "q"}, {"chicken", 0, "limit"}, {"chicken", 26, "limit"}} {
		_, err := svc.Search(context.Background(), uuid.New(), test.q, test.limit)
		var validation *model.ValidationError
		if !errors.As(err, &validation) || validation.Field != test.field {
			t.Errorf("%q/%d: want 422 on %s, got %v", test.q, test.limit, test.field, err)
		}
	}
	result, err := svc.Search(context.Background(), uuid.New(), " chicken ", 25)
	if err != nil {
		t.Fatal(err)
	}
	if result.Incomplete {
		t.Error("usda answered; not incomplete")
	}
	names := []string{}
	for _, food := range result.Foods {
		names = append(names, food.Name)
	}
	if strings.Join(names, ",") != "My shake,Chicken A,Chicken B,Chicken OFF" {
		t.Errorf("merge order own → usda → catalog, de-duplicated: %v", names)
	}
	if len(foods.upserted) != 1 || len(foods.upserted[0]) != 2 {
		t.Errorf("usda items upserted once: %+v", foods.upserted)
	}
	if strings.Join(*calls, ",") != "usda-search" {
		t.Errorf("off is never searched: %v", *calls)
	}
	capped, _ := svc.Search(context.Background(), uuid.New(), "chicken", 2)
	if len(capped.Foods) != 2 {
		t.Errorf("limit caps the merged list: %d", len(capped.Foods))
	}
}

func TestFoodSearchDegradesWhenUSDAUnavailable(t *testing.T) {
	local := catalogFood(model.FoodSourceUSDA, "2", "Chicken B")
	foods := &fakeFoodDAO{catalog: []model.Food{local}}
	svc, _ := newFoodServiceForTest(foods, &fakeEntryDAO{}, &fakeSource{name: "usda", err: ErrSourceUnavailable}, &fakeSource{name: "off"})
	result, err := svc.Search(context.Background(), uuid.New(), "chicken", 25)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Incomplete || len(result.Foods) != 1 || result.Foods[0].Name != "Chicken B" {
		t.Errorf("local rows with incomplete=true: %+v", result)
	}
	if len(foods.upserted) != 0 {
		t.Error("nothing to upsert when the source failed")
	}
	none := &fakeFoodDAO{}
	svc, _ = newFoodServiceForTest(none, &fakeEntryDAO{}, &fakeSource{name: "usda"}, &fakeSource{name: "off"})
	result, _ = svc.Search(context.Background(), uuid.New(), "zzzz", 25)
	if result.Foods == nil || len(result.Foods) != 0 || result.Incomplete {
		t.Errorf("no match is an empty list, never an error: %+v", result)
	}
}

func TestFoodLookupBarcodeChain(t *testing.T) {
	const gtin = "00096619123452"
	own := &model.Food{ID: uuid.New(), Source: model.FoodSourceOwn, Name: "Mine"}
	cached := &model.Food{ID: uuid.New(), Source: model.FoodSourceUSDA, SourceID: "1", Name: "Cached", FetchedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	usdaFood := catalogFood(model.FoodSourceUSDA, "1", "From USDA")
	offFood := catalogFood(model.FoodSourceOpenFoodFacts, "096619123452", "From OFF")
	type step struct {
		name       string
		dao        *fakeFoodDAO
		usda, off  *fakeSource
		wantName   string
		wantErr    string
		wantCalls  string
		wantMisses int
	}
	for _, test := range []step{
		{"own first", &fakeFoodDAO{ownByGTIN: own}, &fakeSource{name: "usda"}, &fakeSource{name: "off"}, "Mine", "", "", 0},
		{"catalog second", &fakeFoodDAO{byGTIN: cached}, &fakeSource{name: "usda"}, &fakeSource{name: "off"}, "Cached", "", "", 0},
		{"recent miss short-circuits", &fakeFoodDAO{recentMiss: true}, &fakeSource{name: "usda", barcode: &usdaFood}, &fakeSource{name: "off"}, "", "not found", "", 0},
		{"usda third", &fakeFoodDAO{}, &fakeSource{name: "usda", barcode: &usdaFood}, &fakeSource{name: "off", barcode: &offFood}, "From USDA", "", "usda-barcode", 0},
		{"off fourth", &fakeFoodDAO{}, &fakeSource{name: "usda"}, &fakeSource{name: "off", barcode: &offFood}, "From OFF", "", "usda-barcode,off-barcode", 0},
		{"both said no → miss recorded", &fakeFoodDAO{}, &fakeSource{name: "usda"}, &fakeSource{name: "off"}, "", "not found", "usda-barcode,off-barcode", 1},
		{"usda down → no miss recorded", &fakeFoodDAO{}, &fakeSource{name: "usda", err: ErrSourceUnavailable}, &fakeSource{name: "off"}, "", "not found", "usda-barcode,off-barcode", 0},
		{"off budget spent", &fakeFoodDAO{}, &fakeSource{name: "usda"}, &fakeSource{name: "off", err: &BudgetSpentError{RetryAfter: 41 * time.Second}}, "", "budget", "usda-barcode,off-barcode", 0},
	} {
		svc, calls := newFoodServiceForTest(test.dao, &fakeEntryDAO{}, test.usda, test.off)
		food, err := svc.LookupBarcode(context.Background(), uuid.New(), "0 96619 12345 2")
		switch {
		case test.wantErr == "" && (err != nil || food == nil || food.Name != test.wantName):
			t.Errorf("%s: got %+v %v", test.name, food, err)
		case test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)):
			t.Errorf("%s: want error containing %q, got %v", test.name, test.wantErr, err)
		}
		if got := strings.Join(*calls, ","); got != test.wantCalls {
			t.Errorf("%s: source calls %q, want %q", test.name, got, test.wantCalls)
		}
		if len(test.dao.misses) != test.wantMisses {
			t.Errorf("%s: misses recorded %v", test.name, test.dao.misses)
		}
		if test.wantMisses == 1 && test.dao.misses[0] != gtin {
			t.Errorf("%s: miss stores the GTIN-14: %v", test.name, test.dao.misses)
		}
	}
	svc, _ := newFoodServiceForTest(&fakeFoodDAO{}, &fakeEntryDAO{}, &fakeSource{name: "usda"}, &fakeSource{name: "off", err: &BudgetSpentError{RetryAfter: 40500 * time.Millisecond}})
	_, err := svc.LookupBarcode(context.Background(), uuid.New(), "096619123452")
	var budget *model.FoodLookupBudgetError
	if !errors.As(err, &budget) || budget.RetryAfterSeconds != 41 {
		t.Errorf("budget error rounds up to whole seconds: %v", err)
	}
	_, err = svc.LookupBarcode(context.Background(), uuid.New(), "096619123456")
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "code" {
		t.Errorf("bad check digit → 422 code, got %v", err)
	}
}

func TestFoodLookupRefreshesStaleCatalogRows(t *testing.T) {
	stale := &model.Food{ID: uuid.New(), Source: model.FoodSourceUSDA, SourceID: "1", Name: "Old", FetchedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	fresh := catalogFood(model.FoodSourceUSDA, "1", "New")
	dao := &fakeFoodDAO{byGTIN: stale}
	svc, calls := newFoodServiceForTest(dao, &fakeEntryDAO{}, &fakeSource{name: "usda", barcode: &fresh}, &fakeSource{name: "off"})
	food, err := svc.LookupBarcode(context.Background(), uuid.New(), "096619123452")
	if err != nil || food.Name != "New" || len(dao.upserted) != 1 || strings.Join(*calls, ",") != "usda-barcode" {
		t.Errorf("stale row refreshed from the source: %+v %v %v", food, err, *calls)
	}
	dao = &fakeFoodDAO{byGTIN: stale}
	svc, _ = newFoodServiceForTest(dao, &fakeEntryDAO{}, &fakeSource{name: "usda", err: ErrSourceUnavailable}, &fakeSource{name: "off", err: ErrSourceUnavailable})
	food, err = svc.LookupBarcode(context.Background(), uuid.New(), "096619123452")
	if err != nil || food.Name != "Old" || len(dao.upserted) != 0 {
		t.Errorf("a failed refresh keeps the old row: %+v %v", food, err)
	}
}

func TestCreateEntryValidationAndSnapshot(t *testing.T) {
	catalog := catalogFood(model.FoodSourceUSDA, "1", "Yogurt")
	catalog.Servings = []model.Serving{{Label: "1 container (170 g)", Grams: 170}, model.HundredGramServing}
	catalog.Per100g = model.Per100g{EnergyKcal: 59, ProteinG: 10.2, CarbsG: 3.6, FatG: 0.4}
	ownRev := int64(1)
	own := model.Food{ID: uuid.New(), Source: model.FoodSourceOwn, Name: "Mine", Servings: []model.Serving{{Label: "1 scoop", Grams: 30}, model.HundredGramServing}, Per100g: model.Per100g{EnergyKcal: 400, ProteinG: 80}, Revision: &ownRev}
	archived := own
	archived.ID, archived.Archived = uuid.New(), true
	dao := &fakeFoodDAO{foods: map[uuid.UUID]*model.Food{catalog.ID: &catalog, own.ID: &own, archived.ID: &archived}}
	entries := &fakeEntryDAO{}
	svc, _ := newFoodServiceForTest(dao, entries, &fakeSource{name: "usda"}, &fakeSource{name: "off"})
	user := uuid.New()
	amount := 1.5
	serving := model.Serving{Label: "1 container (170 g)", Grams: 170}
	protein := 30.0
	for _, test := range []struct {
		name  string
		date  string
		req   model.CreateFoodEntryRequest
		field string
	}{
		{"future date", "2026-10-28", model.CreateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: &protein}, OperationKey: "k"}, "date"},
		{"bad date", "2026-13-01", model.CreateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: &protein}, OperationKey: "k"}, "date"},
		{"missing key", "2026-10-27", model.CreateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: &protein}}, "operation_key"},
		{"neither", "2026-10-27", model.CreateFoodEntryRequest{OperationKey: "k"}, "food"},
		{"both", "2026-10-27", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: catalog.ID}, Serving: &serving, Amount: &amount, QuickAdd: &model.QuickAdd{ProteinG: &protein}, OperationKey: "k"}, "food"},
		{"no serving", "2026-10-27", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: catalog.ID}, Amount: &amount, OperationKey: "k"}, "serving"},
		{"no amount", "2026-10-27", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: catalog.ID}, Serving: &serving, OperationKey: "k"}, "amount"},
		{"amount too big", "2026-10-27", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: catalog.ID}, Serving: &serving, Amount: fptr(50.5), OperationKey: "k"}, "amount"},
		{"amount decimals", "2026-10-27", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: catalog.ID}, Serving: &serving, Amount: fptr(1.234), OperationKey: "k"}, "amount"},
		{"wrong serving", "2026-10-27", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: catalog.ID}, Serving: &model.Serving{Label: "1 container (170 g)", Grams: 200}, Amount: &amount, OperationKey: "k"}, "serving"},
		{"empty quick add", "2026-10-27", model.CreateFoodEntryRequest{QuickAdd: &model.QuickAdd{}, OperationKey: "k"}, "quick_add"},
		{"quick add bounds", "2026-10-27", model.CreateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: fptr(2001)}, OperationKey: "k"}, "protein_g"},
	} {
		_, _, err := svc.CreateEntry(context.Background(), user, test.date, test.req)
		var validation *model.ValidationError
		if !errors.As(err, &validation) || validation.Field != test.field {
			t.Errorf("%s: want 422 on %s, got %v", test.name, test.field, err)
		}
	}
	for _, id := range []uuid.UUID{uuid.New(), archived.ID} {
		_, _, err := svc.CreateEntry(context.Background(), user, "2026-10-27", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: id}, Serving: &model.HundredGramServing, Amount: &amount, OperationKey: "k"})
		var notFound *model.NotFoundError
		if !errors.As(err, &notFound) {
			t.Errorf("unknown or archived food → shared 404, got %v", err)
		}
	}
	created, outcome, err := svc.CreateEntry(context.Background(), user, "2026-10-27", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: catalog.ID}, Serving: &serving, Amount: &amount, OperationKey: "k1"})
	if err != nil || outcome != model.FoodEntryCreatedOutcome {
		t.Fatal(err)
	}
	e := created.Entry
	if e.Kind != model.FoodEntryKindFood || e.CatalogFoodID == nil || *e.CatalogFoodID != catalog.ID || e.UserFoodID != nil || *e.EnergyKcal != 150 || *e.ProteinG != 26 || *e.CarbsG != 9.2 || *e.FatG != 1 || !e.LoggedAt.Equal(svc.now()) {
		t.Errorf("snapshot: %+v", e)
	}
	_, _, err = svc.CreateEntry(context.Background(), user, "2026-10-26", model.CreateFoodEntryRequest{Food: &model.FoodRef{ID: own.ID}, Serving: &model.Serving{Label: "35 g", Grams: 35}, Amount: fptr(2), OperationKey: "k2"})
	if err != nil || entries.entry.UserFoodID == nil || *entries.entry.UserFoodID != own.ID || entries.entry.CatalogFoodID != nil || *entries.entry.EnergyKcal != 280 || *entries.entry.ProteinG != 56 {
		t.Errorf("own food by grams: %+v %v", entries.entry, err)
	}
	_, _, err = svc.CreateEntry(context.Background(), user, "2026-10-27", model.CreateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: fptr(30.5)}, OperationKey: "k3"})
	if err != nil || entries.entry.Kind != model.FoodEntryKindQuickAdd || *entries.entry.ProteinG != 30.5 || entries.entry.EnergyKcal != nil {
		t.Errorf("quick add keeps the typed value and omits absent values: %+v %v", entries.entry, err)
	}
	_, _, err = svc.CreateEntry(context.Background(), user, "2026-10-27", model.CreateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: fptr(30.04)}, OperationKey: "k3"})
	var twoDecimals *model.ValidationError
	if !errors.As(err, &twoDecimals) || twoDecimals.Field != "protein_g" {
		t.Errorf("quick add follows the spec 012 one-decimal rule: %v", err)
	}
	req := model.CreateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: &protein}, OperationKey: "same"}
	_, _, _ = svc.CreateEntry(context.Background(), user, "2026-10-27", req)
	_, _, _ = svc.CreateEntry(context.Background(), user, "2026-10-26", req)
	n := len(entries.hashes)
	if entries.hashes[n-1] == entries.hashes[n-2] {
		t.Error("hash covers the path date")
	}
}

func fptr(v float64) *float64 { return &v }

func TestUpdateAndDeleteEntryValidation(t *testing.T) {
	entries := &fakeEntryDAO{}
	svc, _ := newFoodServiceForTest(&fakeFoodDAO{}, entries, &fakeSource{name: "usda"}, &fakeSource{name: "off"})
	id := uuid.New()
	for _, test := range []struct {
		name  string
		req   model.UpdateFoodEntryRequest
		field string
	}{
		{"revision", model.UpdateFoodEntryRequest{Serving: &model.HundredGramServing, Amount: fptr(1), OperationKey: "k"}, "expected_revision"},
		{"neither", model.UpdateFoodEntryRequest{ExpectedRevision: 1, OperationKey: "k"}, "serving"},
		{"both", model.UpdateFoodEntryRequest{Serving: &model.HundredGramServing, Amount: fptr(1), QuickAdd: &model.QuickAdd{ProteinG: fptr(1)}, ExpectedRevision: 1, OperationKey: "k"}, "serving"},
		{"amount only", model.UpdateFoodEntryRequest{Amount: fptr(1), ExpectedRevision: 1, OperationKey: "k"}, "serving"},
		{"amount range", model.UpdateFoodEntryRequest{Serving: &model.HundredGramServing, Amount: fptr(0), ExpectedRevision: 1, OperationKey: "k"}, "amount"},
		{"empty quick add", model.UpdateFoodEntryRequest{QuickAdd: &model.QuickAdd{}, ExpectedRevision: 1, OperationKey: "k"}, "quick_add"},
	} {
		_, _, err := svc.UpdateEntry(context.Background(), uuid.New(), id, test.req)
		var validation *model.ValidationError
		if !errors.As(err, &validation) || validation.Field != test.field {
			t.Errorf("%s: want 422 on %s, got %v", test.name, test.field, err)
		}
	}
	if _, _, err := svc.UpdateEntry(context.Background(), uuid.New(), id, model.UpdateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: fptr(12.34)}, ExpectedRevision: 2, OperationKey: "k"}); err == nil {
		t.Error("two decimals on a quick add → 422")
	}
	if _, _, err := svc.UpdateEntry(context.Background(), uuid.New(), id, model.UpdateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: fptr(12.3)}, ExpectedRevision: 2, OperationKey: "k"}); err != nil || *entries.change.QuickAdd.ProteinG != 12.3 {
		t.Errorf("quick add values pass through: %+v %v", entries.change, err)
	}
	if _, _, err := svc.DeleteEntry(context.Background(), uuid.New(), id, 0, "k"); err == nil {
		t.Error("delete needs a revision of 1 or more")
	}
	if _, _, err := svc.DeleteEntry(context.Background(), uuid.New(), id, 1, " "); err == nil {
		t.Error("delete needs an operation key")
	}
	_, _, _ = svc.DeleteEntry(context.Background(), uuid.New(), id, 1, "k")
	_, _, _ = svc.DeleteEntry(context.Background(), uuid.New(), id, 2, "k")
	n := len(entries.hashes)
	if entries.hashes[n-1] == entries.hashes[n-2] {
		t.Error("delete hash covers the revision")
	}
}

func TestUserFoodValidation(t *testing.T) {
	dao := &fakeFoodDAO{}
	svc, _ := newFoodServiceForTest(dao, &fakeEntryDAO{}, &fakeSource{name: "usda"}, &fakeSource{name: "off"})
	valid := func() model.SaveUserFoodRequest {
		return model.SaveUserFoodRequest{Name: " Whey ", Brand: ptrString(" Brand "), Barcode: ptrString("096619123452"), Serving: model.Serving{Label: " 1 scoop ", Grams: 30}, EnergyKcal: 120, ProteinG: 24, CarbsG: 3, FatG: 1.5, OperationKey: "k"}
	}
	for _, test := range []struct {
		name   string
		mutate func(*model.SaveUserFoodRequest)
		field  string
	}{
		{"name", func(r *model.SaveUserFoodRequest) { r.Name = "  " }, "name"},
		{"name long", func(r *model.SaveUserFoodRequest) { r.Name = strings.Repeat("a", 201) }, "name"},
		{"brand long", func(r *model.SaveUserFoodRequest) { r.Brand = ptrString(strings.Repeat("b", 121)) }, "brand"},
		{"barcode", func(r *model.SaveUserFoodRequest) { r.Barcode = ptrString("12345") }, "barcode"},
		{"serving label", func(r *model.SaveUserFoodRequest) { r.Serving.Label = "" }, "serving"},
		{"serving grams", func(r *model.SaveUserFoodRequest) { r.Serving.Grams = 0 }, "serving"},
		{"serving decimals", func(r *model.SaveUserFoodRequest) { r.Serving.Grams = 30.25 }, "serving"},
		{"energy", func(r *model.SaveUserFoodRequest) { r.EnergyKcal = -1 }, "energy_kcal"},
		{"protein", func(r *model.SaveUserFoodRequest) { r.ProteinG = 24.25 }, "protein_g"},
		{"fat", func(r *model.SaveUserFoodRequest) { r.FatG = 2001 }, "fat_g"},
		{"key", func(r *model.SaveUserFoodRequest) { r.OperationKey = "" }, "operation_key"},
	} {
		req := valid()
		test.mutate(&req)
		_, _, err := svc.CreateUserFood(context.Background(), uuid.New(), req)
		var validation *model.ValidationError
		if !errors.As(err, &validation) || validation.Field != test.field {
			t.Errorf("%s: want 422 on %s, got %v", test.name, test.field, err)
		}
	}
	if _, _, err := svc.CreateUserFood(context.Background(), uuid.New(), valid()); err != nil {
		t.Fatal(err)
	}
	if dao.userFoodReq.Name != "Whey" || *dao.userFoodReq.Brand != "Brand" || dao.userFoodReq.Serving.Label != "1 scoop" || dao.userFoodGTIN == nil || *dao.userFoodGTIN != "00096619123452" {
		t.Errorf("normalised request: %+v gtin %v", dao.userFoodReq, dao.userFoodGTIN)
	}
	blank := valid()
	blank.Brand, blank.Barcode = ptrString(" "), ptrString("")
	_, _, _ = svc.CreateUserFood(context.Background(), uuid.New(), blank)
	if dao.userFoodReq.Brand != nil || dao.userFoodGTIN != nil {
		t.Error("blank brand and barcode become absent")
	}
	update := valid()
	_, _, err := svc.UpdateUserFood(context.Background(), uuid.New(), uuid.New(), update)
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "expected_revision" {
		t.Errorf("update requires expected_revision: %v", err)
	}
	if _, err := svc.ArchiveUserFood(context.Background(), uuid.New(), uuid.New(), 0, "k"); err == nil {
		t.Error("archive requires a positive revision")
	}
}

func ptrString(v string) *string { return &v }
