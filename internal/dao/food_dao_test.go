package dao

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func daoSource(t *testing.T, file string) string {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

var backtickSQL = regexp.MustCompile("(?s)`([^`]*)`")

// sqlStatements returns every backtick string in the file that reads like SQL.
func sqlStatements(source string) []string {
	var out []string
	for _, match := range backtickSQL.FindAllStringSubmatch(source, -1) {
		upper := strings.ToUpper(match[1])
		if strings.Contains(upper, "SELECT") || strings.Contains(upper, "INSERT") || strings.Contains(upper, "UPDATE ") || strings.Contains(upper, "DELETE") {
			out = append(out, match[1])
		}
	}
	return out
}

func TestFoodDAOOwnerScopingByTable(t *testing.T) {
	for _, file := range []string{"food_dao.go", "food_entry_dao.go"} {
		for _, statement := range sqlStatements(daoSource(t, file)) {
			touchesOwned := strings.Contains(statement, "user_foods") || strings.Contains(statement, "food_entries") || strings.Contains(statement, "nutrition_days")
			touchesCatalogOnly := strings.Contains(statement, "food_catalog") && !touchesOwned && !strings.Contains(statement, "idempotency")
			if touchesOwned && !strings.Contains(statement, "user_id") {
				t.Errorf("%s: owned-table statement lacks user_id:\n%s", file, statement)
			}
			if touchesCatalogOnly && strings.Contains(statement, "user_id") {
				t.Errorf("%s: the shared catalog must not be owner-scoped:\n%s", file, statement)
			}
		}
	}
}

func TestFoodDAOCatalogQueries(t *testing.T) {
	source := daoSource(t, "food_dao.go")
	for _, marker := range []string{
		"ON CONFLICT (source, source_id) DO UPDATE SET",
		"fetched_at=EXCLUDED.fetched_at",
		"WHERE search_vector @@ websearch_to_tsquery('simple', $1)",
		"ORDER BY ts_rank(search_vector, websearch_to_tsquery('simple', $1)) DESC",
		"WHERE gtin=$1",
		"ORDER BY (source = 'usda') DESC, fetched_at DESC",
		"WHERE user_id=$1 AND gtin=$2 AND archived_at IS NULL",
		"WHERE user_id=$1 AND archived_at IS NULL",
		"ON CONFLICT (gtin) DO UPDATE SET checked_at=EXCLUDED.checked_at",
		"WHERE gtin=$1 AND checked_at >= $2",
	} {
		if !strings.Contains(source, marker) {
			t.Errorf("food_dao.go missing %q", marker)
		}
	}
	if strings.Contains(source, "DELETE FROM food_catalog") {
		t.Error("catalog rows are never deleted")
	}
}

func TestFoodDAORecentFoodsQuery(t *testing.T) {
	source := daoSource(t, "food_dao.go")
	start := strings.Index(source, "func (r *foodDAO) ListRecentFoods(")
	end := strings.Index(source, "func (r *foodDAO) CreateUserFood(")
	if start < 0 || end < start {
		t.Fatal("ListRecentFoods not found")
	}
	assertMarkersInOrder(t, source[start:end],
		"COALESCE(catalog_food_id, user_food_id) AS food_id",
		"count(*) AS times_logged, max(logged_at) AS last_logged_at",
		"WHERE user_id=$1 AND kind='food' AND logged_at >= $2",
		"ORDER BY times_logged DESC, last_logged_at DESC",
		"LIMIT $3",
		"JOIN LATERAL",
		"ORDER BY logged_at DESC",
		"LIMIT 1",
		"r.ListFoodsByIDs(ctx, userID, ids)",
	)
}

func TestFoodDAOUserFoodTransactionsKeepAtomicOrder(t *testing.T) {
	source := daoSource(t, "food_dao.go")
	create := source[strings.Index(source, "func (r *foodDAO) CreateUserFood("):strings.Index(source, "func (r *foodDAO) UpdateUserFood(")]
	assertMarkersInOrder(t, create,
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, userFoodCreateScope",
		"INSERT INTO user_foods",
		"insertIdempotency(ctx, tx, userID, userFoodRecord(userFoodCreateScope",
		"tx.Commit(ctx)",
	)
	update := source[strings.Index(source, "func (r *foodDAO) UpdateUserFood("):strings.Index(source, "func (r *foodDAO) ArchiveUserFood(")]
	assertMarkersInOrder(t, update,
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, userFoodUpdateScope",
		"lockUserFood(ctx, tx, userID, id)",
		"*req.ExpectedRevision != *existing.Revision",
		"revisionConflict(req.ExpectedRevision, *existing.Revision, existing)",
		"UPDATE user_foods",
		"revision=revision+1",
		"insertIdempotency(ctx, tx, userID, userFoodRecord(userFoodUpdateScope",
		"tx.Commit(ctx)",
	)
	archive := source[strings.Index(source, "func (r *foodDAO) ArchiveUserFood("):strings.Index(source, "func lockUserFood(")]
	assertMarkersInOrder(t, archive,
		"findIdempotency(ctx, tx, userID, userFoodArchiveScope",
		"expectedRevision != *existing.Revision",
		"SET archived_at=COALESCE(archived_at, now())",
		"tx.Commit(ctx)",
	)
	if strings.Contains(archive, "DELETE FROM user_foods") {
		t.Error("archiving never deletes; entries keep their reference")
	}
	for _, fn := range []string{create, update, archive} {
		if strings.Count(fn, "defer tx.Rollback(ctx)") != 1 {
			t.Error("each transaction relies on one deferred rollback")
		}
	}
}

func TestRevisionConflictHelper(t *testing.T) {
	var validation *model.ValidationError
	if err := revisionConflict(nil, 3, nil); !errors.As(err, &validation) || validation.Field != "expected_revision" {
		t.Errorf("nil expected → 422: %v", err)
	}
	expected := int64(2)
	food := &model.Food{ID: uuid.New()}
	var conflict *model.ConflictError
	err := revisionConflict(&expected, 3, food)
	if !errors.As(err, &conflict) || conflict.Message != "resource revision conflict" || conflict.Expected != 2 || conflict.Actual != 3 || conflict.Authoritative != food {
		t.Errorf("stale → REVISION_CONFLICT with the resource: %v", err)
	}
}

func TestScanUserFoodDerivesPer100g(t *testing.T) {
	label, grams := "1 scoop", 30.0
	var food model.Food
	revision := int64(2)
	row := fakeRow{values: []any{uuid.New(), "Whey", (*string)(nil), (*string)(nil), label, grams, 120, 24.0, 3.0, 1.5, (*timeValue)(nil), revision}}
	if err := scanUserFood(row, &food); err != nil {
		t.Fatal(err)
	}
	if food.Source != model.FoodSourceOwn || food.Per100g != (model.Per100g{EnergyKcal: 400, ProteinG: 80, CarbsG: 10, FatG: 5}) || len(food.Servings) != 2 || *food.Revision != 2 || food.Archived {
		t.Errorf("user food projection: %+v", food)
	}
}

func TestAccountCleanupIncludesFoodLogging(t *testing.T) {
	source := daoSource(t, "account_dao.go")
	entries := strings.Index(source, "DELETE FROM food_entries WHERE user_id = $1")
	foods := strings.Index(source, "DELETE FROM user_foods WHERE user_id = $1")
	days := strings.Index(source, "DELETE FROM nutrition_days WHERE user_id = $1")
	if entries < 0 || foods < 0 || entries > foods || foods > days {
		t.Fatal("account deletion must remove food_entries, then user_foods, before nutrition_days")
	}
	if strings.Contains(source, "food_catalog") || strings.Contains(source, "food_barcode_misses") {
		t.Fatal("shared tables hold no personal data and survive account deletion")
	}
}

// fakeRow satisfies rowScanner for projection tests; pointer destinations
// receive the matching value by simple assignment through context-free copies.
type fakeRow struct{ values []any }

type timeValue struct{}

func (r fakeRow) Scan(dest ...any) error {
	if len(dest) != len(r.values) {
		return errors.New("column count mismatch")
	}
	for i, d := range dest {
		switch target := d.(type) {
		case *uuid.UUID:
			if v, ok := r.values[i].(uuid.UUID); ok {
				*target = v
			}
		case *string:
			if v, ok := r.values[i].(string); ok {
				*target = v
			}
		case **string:
			if v, ok := r.values[i].(*string); ok {
				*target = v
			}
		case *float64:
			if v, ok := r.values[i].(float64); ok {
				*target = v
			}
		case *int:
			if v, ok := r.values[i].(int); ok {
				*target = v
			}
		case *int64:
			if v, ok := r.values[i].(int64); ok {
				*target = v
			}
		default:
			// nullable timestamp stays nil
		}
	}
	return nil
}

var _ = context.Background
