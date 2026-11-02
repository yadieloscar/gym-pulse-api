package dao

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestFoodEntryDAOCreateKeepsAtomicOrder(t *testing.T) {
	source := daoSource(t, "food_entry_dao.go")
	start := strings.Index(source, "func (r *foodEntryDAO) CreateEntry(")
	end := strings.Index(source, "func (r *foodEntryDAO) UpdateEntry(")
	if start < 0 || end < start {
		t.Fatal("CreateEntry not found")
	}
	assertMarkersInOrder(t, source[start:end],
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, foodEntryCreateScope",
		"getFood(ctx, tx, userID, foodRefID(entry))",
		"if food.Archived {",
		"lockNutritionDay(ctx, tx, userID, entry.Date)",
		"existing.Source != model.NutritionSourceFoods",
		"Kind:       model.FoodEntryKindQuickAdd",
		"LoggedAt:   existing.LoggedAt",
		"converted = append(converted, *saved)",
		"insertFoodEntry(ctx, tx, userID, entry)",
		"recomputeNutritionDay(ctx, tx, userID, entry.Date, existing)",
		"insertIdempotency(ctx, tx, userID, foodEntryRecord(foodEntryCreateScope",
		"tx.Commit(ctx)",
	)
	if strings.Count(source[start:end], "defer tx.Rollback(ctx)") != 1 {
		t.Fatal("early returns rely on one deferred rollback")
	}
}

func TestFoodEntryDAOUpdateAndDeleteKeepAtomicOrder(t *testing.T) {
	source := daoSource(t, "food_entry_dao.go")
	update := source[strings.Index(source, "func (r *foodEntryDAO) UpdateEntry("):strings.Index(source, "func (r *foodEntryDAO) DeleteEntry(")]
	assertMarkersInOrder(t, update,
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, foodEntryUpdateScope",
		"lockFoodEntry(ctx, tx, userID, entryID)",
		"change.ExpectedRevision != existing.Revision",
		"Authoritative: existing",
		"applyEntryChange(ctx, tx, userID, existing, change)",
		"lockNutritionDay(ctx, tx, userID, existing.Date)",
		"updateFoodEntry(ctx, tx, userID, existing)",
		"recomputeNutritionDay(ctx, tx, userID, existing.Date, day)",
		"insertIdempotency(ctx, tx, userID, foodEntryRecord(foodEntryUpdateScope",
		"tx.Commit(ctx)",
	)
	del := source[strings.Index(source, "func (r *foodEntryDAO) DeleteEntry("):strings.Index(source, "func applyEntryChange(")]
	assertMarkersInOrder(t, del,
		"findIdempotency(ctx, tx, userID, foodEntryDeleteScope",
		"lockFoodEntry(ctx, tx, userID, entryID)",
		"expectedRevision != existing.Revision",
		"lockNutritionDay(ctx, tx, userID, existing.Date)",
		"DELETE FROM food_entries WHERE user_id=$1 AND id=$2",
		"recomputeNutritionDay(ctx, tx, userID, existing.Date, day)",
		"insertIdempotency(ctx, tx, userID, record, foodEntryLedger{Outcome: model.FoodEntryRemovedOutcome",
		"tx.Commit(ctx)",
	)
}

func TestFoodEntryDAORecomputeWritesTheDayAsTheSum(t *testing.T) {
	source := daoSource(t, "food_entry_dao.go")
	start := strings.Index(source, "func recomputeNutritionDay(")
	end := strings.Index(source, "func foodEntryRecord(")
	if start < 0 || end < start {
		t.Fatal("recomputeNutritionDay not found")
	}
	assertMarkersInOrder(t, source[start:end],
		"SELECT count(*), sum(energy_kcal), sum(protein_g), sum(carbs_g), sum(fat_g)",
		"FROM food_entries",
		"WHERE user_id=$1 AND date=$2",
		"if count == 0 {",
		"DELETE FROM nutrition_days WHERE user_id=$1 AND date=$2",
		"Source: model.NutritionSourceFoods",
		"insertNutritionDay(ctx, tx, userID, day)",
		"updateNutritionDay(ctx, tx, userID, day)",
	)
	if !strings.Contains(source, "FOR UPDATE OF e") || !strings.Contains(source, "WHERE user_id=$1 AND date=$2 FOR UPDATE") {
		t.Error("the entry and the day are locked before the change")
	}
	if !strings.Contains(source, "ORDER BY e.logged_at ASC, e.created_at ASC") {
		t.Error("day entries list oldest first")
	}
}

func TestApplyEntryChangeShapes(t *testing.T) {
	ctx := context.Background()
	quick := &model.FoodEntry{Kind: model.FoodEntryKindQuickAdd}
	protein := 30.0
	if err := applyEntryChange(ctx, nil, uuid.New(), quick, model.UpdateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: &protein}}); err != nil || *quick.ProteinG != 30 || quick.EnergyKcal != nil {
		t.Errorf("quick add replaces values: %+v %v", quick, err)
	}
	var validation *model.ValidationError
	if err := applyEntryChange(ctx, nil, uuid.New(), quick, model.UpdateFoodEntryRequest{Serving: &model.HundredGramServing, Amount: &protein}); !errors.As(err, &validation) || validation.Field != "quick_add" {
		t.Errorf("serving on a quick add → 422 quick_add: %v", err)
	}
	food := &model.FoodEntry{Kind: model.FoodEntryKindFood}
	if err := applyEntryChange(ctx, nil, uuid.New(), food, model.UpdateFoodEntryRequest{QuickAdd: &model.QuickAdd{ProteinG: &protein}}); !errors.As(err, &validation) || validation.Field != "serving" {
		t.Errorf("quick add on a food entry → 422 serving before any query: %v", err)
	}
}

func TestNutritionDAORefusesTypedSavesOnFoodsDays(t *testing.T) {
	source := nutritionDAOSource(t)
	start := strings.Index(source, "func (r *nutritionDAO) SaveDay(")
	end := strings.Index(source, "func existingRevision(")
	assertMarkersInOrder(t, source[start:end],
		"WHERE user_id=$1 AND date=$2\n\t\tFOR UPDATE",
		"existing.Source == model.NutritionSourceFoods && day.Source != model.NutritionSourceAppleHealth",
		"&model.DayHasFoodEntriesError{Day: existing}",
		"case remove:",
		"case day.Source == model.NutritionSourceAppleHealth:",
		"result, outcome = existing, model.NutritionDayIgnored",
	)
}
