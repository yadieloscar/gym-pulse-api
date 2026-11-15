package service

import (
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// importAliases maps common Strong and Hevy exercise names (normalized) to a
// catalog name when the generic rules below do not reach it. Both apps name
// exercises "Movement (Equipment)".
var importAliases = map[string]string{
	"squat barbell":                       "Back Squat",
	"squat":                               "Back Squat",
	"back squat barbell":                  "Back Squat",
	"full squat":                          "Back Squat",
	"bent over row barbell":               "Barbell Row",
	"bent over row":                       "Barbell Row",
	"bicep curl barbell":                  "Barbell Curl",
	"biceps curl barbell":                 "Barbell Curl",
	"bicep curl dumbbell":                 "Dumbbell Curl",
	"biceps curl dumbbell":                "Dumbbell Curl",
	"incline bench press barbell":         "Incline Barbell Bench Press",
	"incline bench press dumbbell":        "Incline Dumbbell Press",
	"bench press":                         "Barbell Bench Press",
	"bench press dumbbell":                "Dumbbell Bench Press",
	"shoulder press dumbbell":             "Seated Dumbbell Shoulder Press",
	"seated shoulder press dumbbell":      "Seated Dumbbell Shoulder Press",
	"overhead press dumbbell":             "Seated Dumbbell Shoulder Press",
	"strict military press barbell":       "Overhead Press",
	"seated row cable":                    "Seated Cable Row",
	"seated cable row v grip cable":       "Seated Cable Row",
	"chest fly dumbbell":                  "Dumbbell Fly",
	"chest fly cable":                     "Cable Fly",
	"dip":                                 "Dips",
	"triceps dip":                         "Dips",
	"chest dip":                           "Dips",
	"triceps pushdown cable straight bar": "Triceps Pushdown",
	"triceps pushdown rope":               "Triceps Pushdown",
	"triceps extension dumbbell":          "Overhead Triceps Extension",
	"running":                             "Outdoor Run",
	"running treadmill":                   "Treadmill Run",
	"treadmill":                           "Treadmill Run",
	"cycling":                             "Stationary Bike",
	"cycling indoor":                      "Stationary Bike",
	"rowing machine":                      "Rowing Machine",
	"rowing":                              "Rowing Machine",
	"elliptical trainer":                  "Elliptical",
	"stair machine":                       "Stairmaster",
	"stair stepper":                       "Stairmaster",
	"lunge dumbbell":                      "Walking Lunge",
	"walking lunge":                       "Walking Lunge",
	"calf raise standing":                 "Standing Calf Raise",
	"standing calf raise machine":         "Standing Calf Raise",
	"seated calf raise machine":           "Seated Calf Raise",
	"leg curl lying":                      "Lying Leg Curl",
	"lying leg curl machine":              "Lying Leg Curl",
	"hip thrust barbell":                  "Hip Thrust",
	"glute bridge barbell":                "Hip Thrust",
	"crunch cable":                        "Cable Crunch",
	"knee raise hanging":                  "Hanging Leg Raise",
	"leg raise hanging":                   "Hanging Leg Raise",
	"chest press machine":                 "Machine Chest Press",
	"iso lateral chest press machine":     "Machine Chest Press",
	"t bar row":                           "T-Bar Row",
	"rear delt reverse fly dumbbell":      "Rear Delt Fly",
	"reverse fly dumbbell":                "Rear Delt Fly",
	"skipping":                            "Jump Rope",
	"jump rope":                           "Jump Rope",
}

// normalizeExerciseName folds case, punctuation, and spacing so "Pull-Up",
// "Pull Up" and "pull up" compare equal.
func normalizeExerciseName(name string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
			continue
		}
		space = true
	}
	return b.String()
}

// splitEquipment splits "Bench Press (Barbell)" into its movement and
// equipment. A name without a trailing parenthetical has no equipment.
func splitEquipment(name string) (string, string) {
	name = strings.TrimSpace(name)
	if !strings.HasSuffix(name, ")") {
		return name, ""
	}
	open := strings.LastIndex(name, "(")
	if open <= 0 {
		return name, ""
	}
	return strings.TrimSpace(name[:open]), strings.TrimSpace(name[open+1 : len(name)-1])
}

// exerciseMatcher resolves source exercise names to the catalog and to the
// athlete's saved-workout exercises.
type exerciseMatcher struct {
	catalog map[string]model.CatalogExercise
	saved   map[string][]uuid.UUID
	cache   map[string]*model.CatalogExercise
}

func newExerciseMatcher(catalog []model.CatalogExercise, saved map[uuid.UUID]string) *exerciseMatcher {
	m := &exerciseMatcher{
		catalog: make(map[string]model.CatalogExercise, len(catalog)),
		saved:   map[string][]uuid.UUID{},
		cache:   map[string]*model.CatalogExercise{},
	}
	for _, item := range catalog {
		m.catalog[normalizeExerciseName(item.Name)] = item
	}
	for id, name := range saved {
		key := normalizeExerciseName(name)
		m.saved[key] = append(m.saved[key], id)
	}
	return m
}

// match returns the catalog exercise for a source name, or nil. Rules, in
// order: alias, exact name, "Equipment Movement", movement alone, alias of
// the movement alone.
func (m *exerciseMatcher) match(source string) *model.CatalogExercise {
	if cached, ok := m.cache[source]; ok {
		return cached
	}
	movement, equipment := splitEquipment(source)
	candidates := []string{normalizeExerciseName(source)}
	if equipment != "" {
		candidates = append(candidates, normalizeExerciseName(equipment+" "+movement), normalizeExerciseName(movement))
	}
	var found *model.CatalogExercise
	for _, candidate := range candidates {
		name := candidate
		if alias, ok := importAliases[candidate]; ok {
			name = normalizeExerciseName(alias)
		}
		if item, ok := m.catalog[name]; ok {
			found = &item
			break
		}
	}
	m.cache[source] = found
	return found
}

// savedExercise links a set to the athlete's saved-workout exercise when
// exactly one has the resolved or source name, so records and "last time"
// include imported history. Ambiguous names stay unlinked.
func (m *exerciseMatcher) savedExercise(names ...string) *uuid.UUID {
	var found *uuid.UUID
	for _, name := range names {
		ids := m.saved[normalizeExerciseName(name)]
		if len(ids) > 1 {
			return nil
		}
		if len(ids) == 1 {
			if found != nil && *found != ids[0] {
				return nil
			}
			id := ids[0]
			found = &id
		}
	}
	return found
}
