package model

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FoodSource names where a food's facts come from. GymPulse never edits or
// estimates them: usda and open_food_facts rows are copies of published data,
// own rows are what the athlete typed from a label.
type FoodSource string

const (
	FoodSourceUSDA          FoodSource = "usda"
	FoodSourceOpenFoodFacts FoodSource = "open_food_facts"
	FoodSourceOwn           FoodSource = "own"
)

// Serving is one way to measure a food, always with a gram weight so any
// serving can be logged by weight.
type Serving struct {
	Label string  `json:"label"`
	Grams float64 `json:"grams"`
}

// Per100g holds a food's values per 100 g, the unit both sources publish.
type Per100g struct {
	EnergyKcal float64 `json:"energy_kcal"`
	ProteinG   float64 `json:"protein_g"`
	CarbsG     float64 `json:"carbs_g"`
	FatG       float64 `json:"fat_g"`
}

// Attribution credits a source whose licence asks for it (Open Food Facts).
type Attribution struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	License string `json:"license"`
}

// Food is one catalog or private food as the client sees it. Nullable fields
// are always present so a client can tell "no brand" from an older shape.
type Food struct {
	ID          uuid.UUID    `json:"id"`
	Source      FoodSource   `json:"source"`
	Name        string       `json:"name"`
	Brand       *string      `json:"brand"`
	Barcode     *string      `json:"barcode"`
	Servings    []Serving    `json:"servings"`
	Per100g     Per100g      `json:"per_100g"`
	Attribution *Attribution `json:"attribution"`
	Revision    *int64       `json:"revision"`
	Archived    bool         `json:"archived"`

	// Catalog bookkeeping, never serialised.
	SourceID  string    `json:"-"`
	DataType  string    `json:"-"`
	GTIN      string    `json:"-"`
	FetchedAt time.Time `json:"-"`
}

// FoodSummary is the food reference carried on an entry.
type FoodSummary struct {
	ID     uuid.UUID  `json:"id"`
	Source FoodSource `json:"source"`
	Name   string     `json:"name"`
	Brand  *string    `json:"brand"`
}

// FoodEntryKind separates logged foods from raw quick-add totals.
type FoodEntryKind string

const (
	FoodEntryKindFood     FoodEntryKind = "food"
	FoodEntryKindQuickAdd FoodEntryKind = "quick_add"
)

// FoodEntry is one logged item on an athlete's day with the values it was
// logged with. A quick add has no food, serving, or amount and omits absent
// values.
type FoodEntry struct {
	ID         uuid.UUID     `json:"id"`
	Date       string        `json:"date"`
	Kind       FoodEntryKind `json:"kind"`
	Food       *FoodSummary  `json:"food"`
	Serving    *Serving      `json:"serving"`
	Amount     *float64      `json:"amount"`
	EnergyKcal *int          `json:"energy_kcal,omitempty"`
	ProteinG   *float64      `json:"protein_g,omitempty"`
	CarbsG     *float64      `json:"carbs_g,omitempty"`
	FatG       *float64      `json:"fat_g,omitempty"`
	Revision   int64         `json:"revision"`
	LoggedAt   time.Time     `json:"logged_at"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`

	// Reference columns, never serialised.
	CatalogFoodID *uuid.UUID `json:"-"`
	UserFoodID    *uuid.UUID `json:"-"`
}

// RecentFood is a food the athlete logged in the last 30 days with the serving
// and amount used last.
type RecentFood struct {
	Food         Food      `json:"food"`
	LastServing  Serving   `json:"last_serving"`
	LastAmount   float64   `json:"last_amount"`
	TimesLogged  int       `json:"times_logged"`
	LastLoggedAt time.Time `json:"last_logged_at"`
}

// FoodSearchResult is the search envelope; incomplete says the outside source
// could not be asked and only stored foods are listed.
type FoodSearchResult struct {
	Foods      []Food `json:"foods"`
	Incomplete bool   `json:"incomplete"`
}

// RecentFoods is the recents envelope.
type RecentFoods struct {
	Foods []RecentFood `json:"foods"`
}

// DayEntries is a date's totals (null when none) and its entries oldest first.
type DayEntries struct {
	Day     *NutritionDay `json:"day"`
	Entries []FoodEntry   `json:"entries"`
}

// FoodEntryCreated is the create envelope; converted holds the quick-add entry
// made from typed totals that already existed on the day, if any.
type FoodEntryCreated struct {
	Entry     FoodEntry     `json:"entry"`
	Day       *NutritionDay `json:"day"`
	Converted []FoodEntry   `json:"converted"`
}

// FoodEntryUpdated is the change envelope.
type FoodEntryUpdated struct {
	Entry FoodEntry     `json:"entry"`
	Day   *NutritionDay `json:"day"`
}

// FoodEntryDeleted is the delete envelope; day is null when no entries remain.
type FoodEntryDeleted struct {
	Day *NutritionDay `json:"day"`
}

// FoodRef names a food by the id a lookup returned.
type FoodRef struct {
	ID uuid.UUID `json:"id"`
}

// QuickAdd carries raw totals for an entry with no food; at least one value.
type QuickAdd struct {
	EnergyKcal *int     `json:"energy_kcal,omitempty"`
	ProteinG   *float64 `json:"protein_g,omitempty"`
	CarbsG     *float64 `json:"carbs_g,omitempty"`
	FatG       *float64 `json:"fat_g,omitempty"`
}

// HasValue reports whether any of the four totals is present.
func (q QuickAdd) HasValue() bool {
	return q.EnergyKcal != nil || q.ProteinG != nil || q.CarbsG != nil || q.FatG != nil
}

// CreateFoodEntryRequest logs a food (food, serving, amount) or a quick add.
type CreateFoodEntryRequest struct {
	Food         *FoodRef   `json:"food,omitempty"`
	Serving      *Serving   `json:"serving,omitempty"`
	Amount       *float64   `json:"amount,omitempty"`
	QuickAdd     *QuickAdd  `json:"quick_add,omitempty"`
	LoggedAt     *time.Time `json:"logged_at,omitempty"`
	OperationKey string     `json:"operation_key"`
}

// UpdateFoodEntryRequest changes an entry's serving and amount, or its
// quick-add values, under the entry's revision.
type UpdateFoodEntryRequest struct {
	Serving          *Serving  `json:"serving,omitempty"`
	Amount           *float64  `json:"amount,omitempty"`
	QuickAdd         *QuickAdd `json:"quick_add,omitempty"`
	ExpectedRevision int64     `json:"expected_revision"`
	OperationKey     string    `json:"operation_key"`
}

// SaveUserFoodRequest creates or replaces a private food typed per serving.
type SaveUserFoodRequest struct {
	Name             string  `json:"name"`
	Brand            *string `json:"brand,omitempty"`
	Barcode          *string `json:"barcode,omitempty"`
	Serving          Serving `json:"serving"`
	EnergyKcal       int     `json:"energy_kcal"`
	ProteinG         float64 `json:"protein_g"`
	CarbsG           float64 `json:"carbs_g"`
	FatG             float64 `json:"fat_g"`
	ExpectedRevision *int64  `json:"expected_revision,omitempty"`
	OperationKey     string  `json:"operation_key"`
}

// FoodEntryOutcome and UserFoodOutcome drive the handler status and are stored
// in the idempotency ledger so a verbatim retry replays the original status.
type FoodEntryOutcome string

const (
	FoodEntryCreatedOutcome  FoodEntryOutcome = "created"
	FoodEntryReplacedOutcome FoodEntryOutcome = "replaced"
	FoodEntryRemovedOutcome  FoodEntryOutcome = "removed"
)

type UserFoodOutcome string

const (
	UserFoodCreated  UserFoodOutcome = "created"
	UserFoodReplaced UserFoodOutcome = "replaced"
	UserFoodArchived UserFoodOutcome = "archived"
)

// DayHasFoodEntriesError refuses a typed save or remove on a day whose totals
// are the sum of its food entries. The handler maps it to 409
// DAY_HAS_FOOD_ENTRIES with the stored day as resource.
type DayHasFoodEntriesError struct {
	Day *NutritionDay
}

func (e *DayHasFoodEntriesError) Error() string { return "day totals are the sum of its food entries" }

// FoodLookupBudgetError says the fallback source's per-minute budget was spent
// before the code could be asked. The handler maps it to the shared 404 with
// details.retry_after_seconds.
type FoodLookupBudgetError struct {
	RetryAfterSeconds int
}

func (e *FoodLookupBudgetError) Error() string { return "food not found; fallback source budget spent" }

// HundredGramServing is appended to every food so it can be logged by weight.
var HundredGramServing = Serving{Label: "100 g", Grams: 100}

// NormalizeGTIN accepts 8, 12, 13, or 14 digit retail barcodes (spaces and
// hyphens allowed), verifies the GS1 check digit, and returns the zero-padded
// 14-digit form used for storage and comparison.
func NormalizeGTIN(input string) (string, error) {
	var digits strings.Builder
	for _, r := range strings.TrimSpace(input) {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == ' ' || r == '-':
		default:
			return "", &ValidationError{Message: "code must contain only digits", Field: "code"}
		}
	}
	code := digits.String()
	switch len(code) {
	case 8, 12, 13, 14:
	default:
		return "", &ValidationError{Message: "code must be 8, 12, 13, or 14 digits", Field: "code"}
	}
	padded := strings.Repeat("0", 14-len(code)) + code
	if !validGS1CheckDigit(padded) {
		return "", &ValidationError{Message: "code has an invalid check digit", Field: "code"}
	}
	return padded, nil
}

// validGS1CheckDigit applies the GS1 mod-10 rule: from the right, excluding
// the check digit, weights alternate 3, 1, 3, and so on.
func validGS1CheckDigit(gtin14 string) bool {
	sum := 0
	for i := range 13 {
		d := int(gtin14[12-i] - '0')
		if i%2 == 0 {
			sum += d * 3
		} else {
			sum += d
		}
	}
	check := (10 - sum%10) % 10
	return check == int(gtin14[13]-'0')
}

// DisplayGTIN returns the 13-digit form when the GTIN fits it, otherwise all 14.
func DisplayGTIN(gtin14 string) string {
	if len(gtin14) == 14 && gtin14[0] == '0' {
		return gtin14[1:]
	}
	return gtin14
}

// ShortGTINForms lists the forms a source may store a GTIN under, longest
// first: the 13-digit form, then the original 12- or 8-digit code.
func ShortGTINForms(gtin14 string) []string {
	forms := []string{DisplayGTIN(gtin14)}
	switch {
	case strings.HasPrefix(gtin14, "000000"):
		forms = append(forms, gtin14[6:])
	case strings.HasPrefix(gtin14, "00"):
		forms = append(forms, gtin14[2:])
	}
	return forms
}

// EnsureHundredGramServing appends the 100 g serving when absent and drops
// servings without a positive gram weight.
func EnsureHundredGramServing(servings []Serving) []Serving {
	out := make([]Serving, 0, len(servings)+1)
	hasHundred := false
	for _, s := range servings {
		if s.Grams <= 0 || math.IsNaN(s.Grams) || math.IsInf(s.Grams, 0) || strings.TrimSpace(s.Label) == "" {
			continue
		}
		if s.Grams == 100 && s.Label == HundredGramServing.Label {
			hasHundred = true
		}
		out = append(out, s)
	}
	if !hasHundred {
		out = append(out, HundredGramServing)
	}
	return out
}

// PlausiblePer100g rejects source values that cannot be real: negatives,
// energy above 900 kcal per 100 g, or a macro above 100 g per 100 g.
func PlausiblePer100g(p Per100g) bool {
	for _, v := range []float64{p.EnergyKcal, p.ProteinG, p.CarbsG, p.FatG} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return false
		}
	}
	return p.EnergyKcal <= 900 && p.ProteinG <= 100 && p.CarbsG <= 100 && p.FatG <= 100
}

// EntryValues computes an entry's snapshot: per 100 g × grams × amount / 100,
// energy to a whole kcal and macros to one decimal, matching the client.
func EntryValues(per Per100g, grams, amount float64) (energyKcal int, proteinG, carbsG, fatG float64) {
	factor := grams * amount / 100
	round1 := func(v float64) float64 { return math.Round(v*10) / 10 }
	return int(math.Round(per.EnergyKcal * factor)), round1(per.ProteinG * factor), round1(per.CarbsG * factor), round1(per.FatG * factor)
}

// ResolveServing accepts one of the food's servings (same label and grams) or
// a grams-only serving labelled "<n> g".
func ResolveServing(food *Food, serving Serving) (Serving, error) {
	for _, s := range food.Servings {
		if s.Label == serving.Label && s.Grams == serving.Grams {
			return s, nil
		}
	}
	if serving.Grams > 0 && serving.Grams <= 5000 && serving.Label == trimFloat(serving.Grams)+" g" {
		return serving, nil
	}
	return Serving{}, &ValidationError{Message: "serving must be one of the food's servings or a grams-only serving", Field: "serving"}
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}

// UserFoodPer100g derives per-100 g values from a label typed per serving.
func UserFoodPer100g(servingG float64, energyKcal int, proteinG, carbsG, fatG float64) Per100g {
	if servingG <= 0 {
		return Per100g{}
	}
	f := 100 / servingG
	round1 := func(v float64) float64 { return math.Round(v*10) / 10 }
	return Per100g{EnergyKcal: round1(float64(energyKcal) * f), ProteinG: round1(proteinG * f), CarbsG: round1(carbsG * f), FatG: round1(fatG * f)}
}

// OpenFoodFactsAttribution builds the credit block for an OFF product.
func OpenFoodFactsAttribution(code string) *Attribution {
	return &Attribution{Name: "Open Food Facts", URL: "https://world.openfoodfacts.org/product/" + code, License: "ODbL"}
}
