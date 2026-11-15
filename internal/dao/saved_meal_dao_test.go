package dao

import (
	"strings"
	"testing"
)

// Every statement that names a saved-meal table is scoped to the owner: by
// user_id on saved_meals, or through a join or sub-select on the owning meal
// for the items.
func TestSavedMealDAOOwnerScoping(t *testing.T) {
	for _, statement := range sqlStatements(daoSource(t, "saved_meal_dao.go")) {
		if !strings.Contains(statement, "saved_meal") {
			continue
		}
		if !strings.Contains(statement, "user_id") {
			t.Errorf("saved-meal statement lacks user_id:\n%s", statement)
		}
	}
}

func TestSavedMealDAOLogKeepsAtomicOrder(t *testing.T) {
	source := daoSource(t, "saved_meal_dao.go")
	start := strings.Index(source, "func (r *savedMealDAO) LogMeal(")
	end := strings.Index(source, "func convertTypedTotals(")
	if start < 0 || end < start {
		t.Fatal("LogMeal not found")
	}
	body := source[start:end]
	assertMarkersInOrder(t, body,
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, savedMealLogScope",
		"lockSavedMeal(ctx, tx, userID, mealID)",
		"resolveMealItems(ctx, tx, userID, items, heldFoodIDs(stored))",
		"if !item.Available {",
		"lockNutritionDay(ctx, tx, userID, date)",
		"convertTypedTotals(ctx, tx, userID, date, existing)",
		"LoggedAt: loggedAt.Add(time.Duration(i) * time.Microsecond)",
		"insertFoodEntry(ctx, tx, userID, entry)",
		"recomputeNutritionDay(ctx, tx, userID, date, existing)",
		"UPDATE saved_meals SET last_logged_at=now() WHERE user_id=$1 AND id=$2",
		"insertIdempotency(ctx, tx, userID, savedMealRecord(savedMealLogScope",
		"tx.Commit(ctx)",
	)
	if strings.Count(body, "defer tx.Rollback(ctx)") != 1 {
		t.Fatal("early returns rely on one deferred rollback so a failed item logs nothing")
	}
	if strings.Count(body, "recomputeNutritionDay(") != 1 {
		t.Fatal("the day is summed once per meal log, not once per item")
	}
	if strings.Contains(body, "revision=revision+1") {
		t.Fatal("logging a meal must not advance its revision")
	}
}

func TestSavedMealDAOMutationsLockReplayAndCheckRevision(t *testing.T) {
	source := daoSource(t, "saved_meal_dao.go")
	create := source[strings.Index(source, "func (r *savedMealDAO) CreateMeal("):strings.Index(source, "func (r *savedMealDAO) UpdateMeal(")]
	assertMarkersInOrder(t, create,
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, savedMealCreateScope",
		"SELECT count(*) FROM saved_meals WHERE user_id=$1",
		"count >= model.MaxSavedMeals",
		"checkSavedMealName(ctx, tx, userID, uuid.Nil, req.Name)",
		"resolveMealItems(ctx, tx, userID, req.Items, nil)",
		"INSERT INTO saved_meals AS m (id, user_id, name)",
		"insertSavedItems(ctx, tx, userID, meal.ID, items)",
		"insertIdempotency(ctx, tx, userID, savedMealRecord(savedMealCreateScope",
		"tx.Commit(ctx)",
	)
	update := source[strings.Index(source, "func (r *savedMealDAO) UpdateMeal("):strings.Index(source, "func (r *savedMealDAO) DeleteMeal(")]
	assertMarkersInOrder(t, update,
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, savedMealUpdateScope",
		"lockSavedMeal(ctx, tx, userID, mealID)",
		"*req.ExpectedRevision != existing.Revision",
		"revisionConflict(req.ExpectedRevision, existing.Revision, existing)",
		"checkSavedMealName(ctx, tx, userID, mealID, req.Name)",
		"resolveMealItems(ctx, tx, userID, req.Items, heldFoodIDs(stored))",
		"SET name=$3, revision=revision+1, updated_at=now()",
		"DELETE FROM saved_meal_items",
		"insertSavedItems(ctx, tx, userID, mealID, items)",
		"insertIdempotency(ctx, tx, userID, savedMealRecord(savedMealUpdateScope",
		"tx.Commit(ctx)",
	)
	del := source[strings.Index(source, "func (r *savedMealDAO) DeleteMeal("):strings.Index(source, "// LogMeal inserts")]
	assertMarkersInOrder(t, del,
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, savedMealDeleteScope",
		"lockSavedMeal(ctx, tx, userID, mealID)",
		"expectedRevision != existing.Revision",
		"DELETE FROM saved_meals WHERE user_id=$1 AND id=$2",
		"insertIdempotency(ctx, tx, userID, savedMealRecord(savedMealDeleteScope",
		"tx.Commit(ctx)",
	)
}

// Meals never store values and food entries never point at a meal, so a meal
// can change or go away without touching a logged day.
func TestSavedMealsStayApartFromLoggedHistory(t *testing.T) {
	source := daoSource(t, "saved_meal_dao.go")
	for _, forbidden := range []string{"UPDATE food_entries", "DELETE FROM food_entries", "saved_meal_id", "meal_id=e.", "DELETE FROM user_foods", "DELETE FROM food_catalog"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("saved_meal_dao.go must not contain %q", forbidden)
		}
	}
	entries := daoSource(t, "food_entry_dao.go")
	if strings.Contains(entries, "saved_meal") {
		t.Error("food entries carry no link to a saved meal")
	}
}

func TestAccountCleanupRemovesSavedMealsBeforeOwnFoods(t *testing.T) {
	source := daoSource(t, "account_dao.go")
	meals := strings.Index(source, "DELETE FROM saved_meals WHERE user_id = $1")
	foods := strings.Index(source, "DELETE FROM user_foods WHERE user_id = $1")
	if meals < 0 || foods < 0 || meals > foods {
		t.Fatal("account deletion must remove saved_meals before user_foods: meal items reference own foods with RESTRICT")
	}
}
