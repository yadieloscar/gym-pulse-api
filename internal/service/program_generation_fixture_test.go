package service

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// seededCatalog rebuilds the exercise catalog from migration 008 with fresh
// ids so fixtures written by name resolve exactly as the live seed would.
func seededCatalog(t *testing.T) ([]model.CatalogExercise, map[string]uuid.UUID) {
	t.Helper()
	body, err := os.ReadFile("../../migrations/008_create_exercise_catalog.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile(`\('([^']+)',\s*'([^']+)',\s*'([^']+)',\s*('[^']+'|NULL),\s*(\d+)\)`)
	var catalog []model.CatalogExercise
	ids := map[string]uuid.UUID{}
	for _, match := range row.FindAllStringSubmatch(string(body), -1) {
		id := uuid.New()
		entry := model.CatalogExercise{ID: id, Name: match[1], Category: match[2], Modality: match[3]}
		if match[4] != "NULL" {
			mechanic := match[4][1 : len(match[4])-1]
			entry.Mechanic = &mechanic
		}
		catalog = append(catalog, entry)
		ids[match[1]] = id
	}
	if len(catalog) < 50 {
		t.Fatalf("catalog seed parse found only %d rows", len(catalog))
	}
	return catalog, ids
}

type fixtureExercise struct {
	Name                  string  `json:"name"`
	TargetSets            int     `json:"target_sets"`
	TargetReps            *int    `json:"target_reps"`
	TargetDurationSeconds *int    `json:"target_duration_seconds"`
	RestSeconds           *int    `json:"rest_seconds"`
	Notes                 *string `json:"notes"`
}

type fixtureDraft struct {
	Goal          string `json:"goal"`
	AvailableDays []int  `json:"available_days"`
	Draft         struct {
		Name      string `json:"name"`
		Rationale string `json:"rationale"`
		Workouts  []struct {
			Name      string            `json:"name"`
			Exercises []fixtureExercise `json:"exercises"`
		} `json:"workouts"`
	} `json:"draft"`
}

// TestRecordedDraftsRepairAndCreateCleanly covers SC-001 and SC-002: every
// recorded draft across the six goals and 1, 3, and 6 training days repairs,
// resolves every exercise to the seeded catalog, and passes the custom-program
// shape validation unchanged.
func TestRecordedDraftsRepairAndCreateCleanly(t *testing.T) {
	catalog, ids := seededCatalog(t)
	body, err := os.ReadFile("testdata/program_generation/drafts.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []fixtureDraft
	if err := json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 18 {
		t.Fatalf("want 18 fixtures (6 goals x 3 day counts), got %d", len(fixtures))
	}
	seenGoals := map[string]bool{}
	for _, fixture := range fixtures {
		seenGoals[fixture.Goal] = true
		raw := &model.RawGeneratedDraft{Name: fixture.Draft.Name, Rationale: fixture.Draft.Rationale}
		for _, w := range fixture.Draft.Workouts {
			rw := model.RawGeneratedWorkout{Name: w.Name}
			for _, e := range w.Exercises {
				id, ok := ids[e.Name]
				if !ok {
					t.Fatalf("%s: fixture names unknown exercise %q", fixture.Draft.Name, e.Name)
				}
				rw.Exercises = append(rw.Exercises, model.RawGeneratedExercise{CatalogID: id.String(), TargetSets: e.TargetSets, TargetReps: e.TargetReps, TargetDurationSeconds: e.TargetDurationSeconds, RestSeconds: e.RestSeconds, Notes: e.Notes})
			}
			raw.Workouts = append(raw.Workouts, rw)
		}
		profile := &model.TrainingProfile{PrimaryGoal: fixture.Goal, AvailableDays: fixture.AvailableDays, UsualActivity: "moderate", Experience: "intermediate", Equipment: []string{"full_gym"}, SessionDurationMinutes: 60, Timezone: "UTC"}
		generated, err := model.RepairGeneratedDraft(profile, catalog, raw, "fixture", nil)
		if err != nil {
			t.Fatalf("%s: repair failed: %v", fixture.Draft.Name, err)
		}
		if n := len(generated.Draft.Workouts); n != min(len(fixture.AvailableDays), 6) {
			t.Fatalf("%s: want %d workouts, got %d", fixture.Draft.Name, min(len(fixture.AvailableDays), 6), n)
		}
		for _, w := range generated.Draft.Workouts {
			for _, e := range w.Exercises {
				if _, ok := ids[e.Name]; !ok || e.CatalogID == uuid.Nil {
					t.Fatalf("%s: exercise %q did not resolve to the catalog", fixture.Draft.Name, e.Name)
				}
			}
		}
		if err := validateProgramShape(generated.Draft.PrimaryGoal, generated.Draft.ProgramWorkouts()); err != nil {
			t.Fatalf("%s: custom-program validation rejected the draft: %v", fixture.Draft.Name, err)
		}
	}
	for _, goal := range model.ValidTrainingGoals {
		if !seenGoals[goal] {
			t.Errorf("no fixture for goal %s", goal)
		}
	}
}
