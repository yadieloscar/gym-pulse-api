package model

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// Limits on what one athlete may save. The item cap also bounds a single
// meal log.
const (
	MaxSavedMeals          = 50
	MaxSavedMealItems      = 20
	MaxSavedMealNameLength = 60
)

// MealTotals is the sum of a meal's items at their foods' current facts.
type MealTotals struct {
	EnergyKcal int     `json:"energy_kcal"`
	ProteinG   float64 `json:"protein_g"`
	CarbsG     float64 `json:"carbs_g"`
	FatG       float64 `json:"fat_g"`
}

// SavedMealItem is one food in a saved meal with the serving and amount to
// log. The four values are computed, never stored, and are omitted when the
// food's current facts cannot be shown (Available is false).
type SavedMealItem struct {
	Food       Food     `json:"food"`
	Serving    Serving  `json:"serving"`
	Amount     float64  `json:"amount"`
	Available  bool     `json:"available"`
	EnergyKcal *int     `json:"energy_kcal,omitempty"`
	ProteinG   *float64 `json:"protein_g,omitempty"`
	CarbsG     *float64 `json:"carbs_g,omitempty"`
	FatG       *float64 `json:"fat_g,omitempty"`
}

// SavedMeal is a named, ordered list of foods an athlete saved to log again
// in one request. Totals is nil when any item is unavailable. Revision
// advances when the name or items change; logging the meal does not advance
// it.
type SavedMeal struct {
	ID           uuid.UUID       `json:"id"`
	Name         string          `json:"name"`
	Section      *MealSection    `json:"section"`
	Items        []SavedMealItem `json:"items"`
	Totals       *MealTotals     `json:"totals"`
	Revision     int64           `json:"revision"`
	LastLoggedAt *time.Time      `json:"last_logged_at"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// SavedMeals is the list response; Meals is never nil.
type SavedMeals struct {
	Meals []SavedMeal `json:"meals"`
}

// MealItemInput names a food with the serving and amount to save or log.
type MealItemInput struct {
	Food    FoodRef `json:"food"`
	Serving Serving `json:"serving"`
	Amount  float64 `json:"amount"`
}

// SaveMealRequest creates or replaces a saved meal. ExpectedRevision is
// required on replace and absent on create.
type SaveMealRequest struct {
	Name             string          `json:"name"`
	Section          *MealSection    `json:"section,omitempty"`
	Items            []MealItemInput `json:"items"`
	ExpectedRevision *int64          `json:"expected_revision,omitempty"`
	OperationKey     string          `json:"operation_key"`
}

// MealRef names a saved meal by id.
type MealRef struct {
	ID uuid.UUID `json:"id"`
}

// LogMealRequest logs a saved meal to a day. Items, when present, replaces
// the saved items for this log only and never changes the saved meal.
type LogMealRequest struct {
	Meal  *MealRef        `json:"meal"`
	Items []MealItemInput `json:"items,omitempty"`
	// Section is the heading for every entry of this log; nil uses the
	// meal's own.
	Section      *MealSection `json:"section,omitempty"`
	LoggedAt     *time.Time   `json:"logged_at,omitempty"`
	OperationKey string       `json:"operation_key"`
}

// MealLogged is the result of logging a meal: one ordinary food entry per
// item in item order, the day as the sum of its entries, and the quick add
// made from typed totals that were already on the day, if any.
type MealLogged struct {
	Entries   []FoodEntry   `json:"entries"`
	Day       *NutritionDay `json:"day"`
	Converted []FoodEntry   `json:"converted"`
}

// SavedMealOutcome drives the handler status and is stored in the idempotency
// ledger so a verbatim retry replays the original status.
type SavedMealOutcome string

const (
	SavedMealCreated  SavedMealOutcome = "created"
	SavedMealReplaced SavedMealOutcome = "replaced"
	SavedMealRemoved  SavedMealOutcome = "removed"
	SavedMealLogged   SavedMealOutcome = "logged"
)

// SavedMealLimitError refuses another saved meal once the athlete holds
// Limit. The handler maps it to 409 SAVED_MEAL_LIMIT_REACHED.
type SavedMealLimitError struct {
	Limit int
}

func (e *SavedMealLimitError) Error() string { return "saved meal limit reached" }

// MatchSavedServing returns the serving a saved item logs with. The saved
// label is matched against the food's current servings and the food's own
// gram weight is used, so a weight the database rounded when the item was
// stored, or one the athlete or the source changed since, never leaves the
// meal unloggable or out of step with the food. When the food no longer has
// a serving with that label, the saved label and weight are used as they
// are: a gram weight is enough to compute an entry.
func MatchSavedServing(food *Food, saved Serving) Serving {
	for _, s := range food.Servings {
		if s.Label == saved.Label {
			return s
		}
	}
	return saved
}

// NewSavedMealItem builds an item with its values at the food's current
// facts. A food whose facts are not plausible yields an unavailable item
// with no values rather than wrong numbers.
func NewSavedMealItem(food Food, serving Serving, amount float64) SavedMealItem {
	item := SavedMealItem{Food: food, Serving: serving, Amount: amount}
	if !PlausiblePer100g(food.Per100g) {
		return item
	}
	energy, protein, carbs, fat := EntryValues(food.Per100g, serving.Grams, amount)
	item.Available = true
	item.EnergyKcal, item.ProteinG, item.CarbsG, item.FatG = &energy, &protein, &carbs, &fat
	return item
}

// SumMealItems adds the items' values, which is what a day's totals rise by
// when the meal is logged. It returns nil when any item is unavailable.
func SumMealItems(items []SavedMealItem) *MealTotals {
	totals := &MealTotals{}
	for _, item := range items {
		if !item.Available {
			return nil
		}
		totals.EnergyKcal += *item.EnergyKcal
		totals.ProteinG += *item.ProteinG
		totals.CarbsG += *item.CarbsG
		totals.FatG += *item.FatG
	}
	round1 := func(v float64) float64 { return math.Round(v*10) / 10 }
	totals.ProteinG, totals.CarbsG, totals.FatG = round1(totals.ProteinG), round1(totals.CarbsG), round1(totals.FatG)
	return totals
}
