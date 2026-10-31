package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func generationCatalog() []CatalogExercise {
	compound := "compound"
	return []CatalogExercise{
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Name: "Barbell Bench Press", Category: "push", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), Name: "Barbell Row", Category: "pull", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000003"), Name: "Back Squat", Category: "legs", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000004"), Name: "Plank", Category: "core", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000005"), Name: "Rowing", Category: "cardio", Modality: "cardio"},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000006"), Name: "Deadlift", Category: "pull", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000007"), Name: "Pull-Up", Category: "pull", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000008"), Name: "Dips", Category: "push", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000009"), Name: "Lunge", Category: "legs", Modality: "strength", Mechanic: &compound},
	}
}

func generationProfile(days ...int) *TrainingProfile {
	return &TrainingProfile{PrimaryGoal: GoalHypertrophy, AvailableDays: days, UsualActivity: "moderate", Experience: "intermediate", Equipment: []string{"barbell"}, SessionDurationMinutes: 60, Timezone: "America/New_York"}
}

func rawExercise(id string, sets int, reps *int) RawGeneratedExercise {
	return RawGeneratedExercise{CatalogID: "00000000-0000-0000-0000-00000000000" + id, TargetSets: sets, TargetReps: reps}
}

func intPtr(v int) *int { return &v }

func validRaw() *RawGeneratedDraft {
	return &RawGeneratedDraft{
		Name: "Three-day split", Rationale: "Push, pull, legs across your three days.",
		Workouts: []RawGeneratedWorkout{
			{Name: "Push", Exercises: []RawGeneratedExercise{rawExercise("1", 4, intPtr(8)), rawExercise("8", 3, intPtr(10)), rawExercise("4", 3, intPtr(12))}},
			{Name: "Pull", Exercises: []RawGeneratedExercise{rawExercise("2", 4, intPtr(8)), rawExercise("7", 3, intPtr(6)), rawExercise("6", 3, intPtr(5))}},
			{Name: "Legs", Exercises: []RawGeneratedExercise{rawExercise("3", 4, intPtr(6)), rawExercise("9", 3, intPtr(10)), rawExercise("5", 1, nil)}},
		},
	}
}

func TestRepairGeneratedDraftHappyPath(t *testing.T) {
	request := "no machines"
	got, err := RepairGeneratedDraft(generationProfile(5, 1, 3), generationCatalog(), validRaw(), "claude-opus-5-5", &request)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	draft := got.Draft
	if draft.Name != "Three-day split" || draft.PrimaryGoal != GoalHypertrophy || got.Rationale == "" {
		t.Fatalf("unexpected draft header: %+v / %q", draft, got.Rationale)
	}
	if draft.Roadmap["origin"] != ProgramGenerationOrigin || draft.Roadmap["model"] != "claude-opus-5-5" || draft.Roadmap["request"] != "no machines" {
		t.Fatalf("roadmap provenance missing: %v", draft.Roadmap)
	}
	if len(draft.Workouts) != 3 {
		t.Fatalf("want 3 workouts, got %d", len(draft.Workouts))
	}
	for i, w := range draft.Workouts {
		wantDay := []int{1, 3, 5}[i]
		if w.Kind != UnitKindExercises || w.SequencePosition != i+1 || w.PreferredWeekday != wantDay {
			t.Fatalf("workout %d placement wrong: %+v", i, w)
		}
		for j, e := range w.Exercises {
			if e.ExerciseOrder != j+1 || e.TargetWeight != nil {
				t.Fatalf("exercise %d/%d order or weight wrong: %+v", i, j, e)
			}
		}
	}
	rowing := draft.Workouts[2].Exercises[2]
	if rowing.Modality != "cardio" || rowing.TargetReps != nil || rowing.TargetDurationSeconds == nil || *rowing.TargetDurationSeconds != defaultDraftDurationSecs || rowing.RestSeconds != defaultDraftRestSecs {
		t.Fatalf("cardio defaults wrong: %+v", rowing)
	}
	bench := draft.Workouts[0].Exercises[0]
	if bench.Name != "Barbell Bench Press" || bench.Category != "push" || bench.TargetDurationSeconds != nil || *bench.TargetReps != 8 {
		t.Fatalf("strength fields not copied from the catalog: %+v", bench)
	}
}

func TestRepairGeneratedDraftRepairsWhatItSafelyCan(t *testing.T) {
	raw := validRaw()
	// Unknown id, a duplicate, and a malformed id are dropped; nine valid
	// exercises are capped at eight.
	raw.Workouts[0].Exercises = append(raw.Workouts[0].Exercises,
		RawGeneratedExercise{CatalogID: "00000000-0000-0000-0000-0000000000ff", TargetSets: 3},
		rawExercise("1", 3, intPtr(8)),
		RawGeneratedExercise{CatalogID: "not-a-uuid", TargetSets: 3},
		rawExercise("2", 3, intPtr(8)), rawExercise("3", 3, intPtr(8)), rawExercise("5", 3, nil),
		rawExercise("6", 3, intPtr(8)), rawExercise("7", 3, intPtr(8)), rawExercise("9", 3, intPtr(8)),
	)
	// A fourth workout exceeds the three available days.
	raw.Workouts = append(raw.Workouts, raw.Workouts[0])
	// Out-of-range targets are clamped; a cardio duration is clamped too.
	raw.Workouts[1].Exercises[0].TargetSets = 40
	raw.Workouts[1].Exercises[0].TargetReps = intPtr(900)
	raw.Workouts[1].Exercises[0].RestSeconds = intPtr(5000)
	raw.Workouts[2].Exercises[2].TargetDurationSeconds = intPtr(5)
	raw.Workouts[2].Exercises[2].TargetReps = intPtr(12)
	raw.Name = strings.Repeat("n", 200)
	raw.Rationale = strings.Repeat("r", 900)
	raw.Workouts[1].Name = "  "
	notes := "  " + strings.Repeat("x", 300)
	raw.Workouts[0].Exercises[0].Notes = &notes

	got, err := RepairGeneratedDraft(generationProfile(1, 3, 5), generationCatalog(), raw, "m", nil)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	d := got.Draft
	if len(d.Workouts) != 3 {
		t.Fatalf("surplus workout not dropped: %d", len(d.Workouts))
	}
	if n := len(d.Workouts[0].Exercises); n != maxDraftExercises {
		t.Fatalf("want %d exercises after repair, got %d", maxDraftExercises, n)
	}
	ids := map[uuid.UUID]int{}
	for _, e := range d.Workouts[0].Exercises {
		ids[e.CatalogID]++
	}
	for id, n := range ids {
		if n != 1 {
			t.Fatalf("duplicate %s survived", id)
		}
	}
	pull := d.Workouts[1].Exercises[0]
	if pull.TargetSets != 10 || *pull.TargetReps != 50 || pull.RestSeconds != 600 {
		t.Fatalf("clamps not applied: %+v", pull)
	}
	rowing := d.Workouts[2].Exercises[2]
	if *rowing.TargetDurationSeconds != 30 || rowing.TargetReps != nil {
		t.Fatalf("cardio clamp or reps clearing wrong: %+v", rowing)
	}
	if len([]rune(d.Name)) != maxDraftNameRunes || len([]rune(got.Rationale)) != maxDraftRationaleRunes {
		t.Fatalf("truncation wrong: %d / %d", len([]rune(d.Name)), len([]rune(got.Rationale)))
	}
	if d.Workouts[1].Name != "Workout 2" {
		t.Fatalf("blank workout name not defaulted: %q", d.Workouts[1].Name)
	}
	if n := d.Workouts[0].Exercises[0].Notes; n == nil || len([]rune(*n)) != maxDraftNotesRunes {
		t.Fatalf("notes not truncated: %v", n)
	}
	if d.Roadmap["request"] != nil {
		t.Fatalf("request must be null when omitted: %v", d.Roadmap["request"])
	}
}

func TestRepairGeneratedDraftRejectsWhatItCannotRepair(t *testing.T) {
	cases := map[string]func(*RawGeneratedDraft){
		"no workouts":       func(r *RawGeneratedDraft) { r.Workouts = nil },
		"too few exercises": func(r *RawGeneratedDraft) { r.Workouts[0].Exercises = r.Workouts[0].Exercises[:2] },
		"all unknown": func(r *RawGeneratedDraft) {
			r.Workouts[1].Exercises[0].CatalogID = "00000000-0000-0000-0000-0000000000aa"
		},
		"duplicates collapse": func(r *RawGeneratedDraft) {
			r.Workouts[2].Exercises = []RawGeneratedExercise{rawExercise("3", 3, intPtr(5)), rawExercise("3", 3, intPtr(5)), rawExercise("3", 3, intPtr(5))}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			raw := validRaw()
			mutate(raw)
			_, err := RepairGeneratedDraft(generationProfile(1, 3, 5), generationCatalog(), raw, "m", nil)
			var invalid *InvalidDraftError
			if !errors.As(err, &invalid) {
				t.Fatalf("want InvalidDraftError, got %v", err)
			}
		})
	}
	if _, err := RepairGeneratedDraft(nil, generationCatalog(), validRaw(), "m", nil); err == nil {
		t.Fatal("nil profile must be rejected")
	}
}

func TestRepairGeneratedDraftOneAndSevenDays(t *testing.T) {
	raw := validRaw()
	got, err := RepairGeneratedDraft(generationProfile(4), generationCatalog(), raw, "m", nil)
	if err != nil || len(got.Draft.Workouts) != 1 || got.Draft.Workouts[0].PreferredWeekday != 4 {
		t.Fatalf("one-day profile must keep one workout on that day: %v %+v", err, got)
	}
	if DraftWorkoutCap([]int{1, 2, 3, 4, 5, 6, 7}) != maxDraftWorkouts {
		t.Fatal("seven days must cap at six workouts")
	}
}

func TestProgramWorkoutsConvertsToCreateShape(t *testing.T) {
	got, err := RepairGeneratedDraft(generationProfile(1, 3, 5), generationCatalog(), validRaw(), "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	workouts := got.Draft.ProgramWorkouts()
	if len(workouts) != 3 || workouts[0].Kind != UnitKindExercises || *workouts[0].PreferredWeekday != 1 || workouts[0].Exercises[0].CatalogID == nil || *workouts[0].Exercises[0].RestSeconds != defaultDraftRestSecs {
		t.Fatalf("conversion lost fields: %+v", workouts)
	}
	for _, w := range workouts {
		for _, e := range w.Exercises {
			if e.Name == "" || e.ExerciseOrder < 1 || e.TargetSets < 1 || (e.Modality != "strength" && e.Modality != "cardio") {
				t.Fatalf("converted exercise would fail program validation: %+v", e)
			}
		}
	}
}

func TestGenerationJSONShapes(t *testing.T) {
	body, _ := json.Marshal(GenerationStatus{Enabled: false, Limit: 5, RemainingToday: 5})
	if string(body) != `{"enabled":false,"limit":5,"remaining_today":5,"resets_at":null}` {
		t.Fatalf("status shape: %s", body)
	}
	got, err := RepairGeneratedDraft(generationProfile(1), generationCatalog(), validRaw(), "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := json.Marshal(GenerateProgramResponse{Draft: got.Draft, Rationale: got.Rationale, RemainingToday: 4, ResetsAt: time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)})
	for _, want := range []string{`"request":null`, `"target_weight":null`, `"target_duration_seconds":null`, `"notes":null`, `"kind":"exercises"`, `"catalog_id":"00000000-0000-0000-0000-000000000001"`, `"roadmap":{"model":"m","origin":"generated","request":null}`} {
		if !strings.Contains(string(resp), want) {
			t.Errorf("response missing %s: %s", want, resp)
		}
	}
	if strings.Contains(string(resp), `"id"`) {
		t.Fatalf("draft must not carry ids: %s", resp)
	}
	if (&GenerationLimitError{}).Error() != "daily program generation limit reached" || (&GenerationUnavailableError{}).Error() != "program generation is unavailable" {
		t.Fatal("stable error messages changed")
	}
}

func TestGenerationSchemaEnumeratesCatalog(t *testing.T) {
	schema := GenerationSchema([]string{"a", "b"}, 3)
	body, _ := json.Marshal(schema)
	for _, want := range []string{`"enum":["a","b"]`, `"maxItems":3`, `"minItems":3`, `"maxItems":8`, `"additionalProperties":false`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("schema missing %s: %s", want, body)
		}
	}
}

func TestNormalizeGenerationRequest(t *testing.T) {
	if got, err := NormalizeGenerationRequest(nil); got != nil || err != nil {
		t.Fatal("nil must pass through")
	}
	s := "  no deadlifts  "
	if got, err := NormalizeGenerationRequest(&s); err != nil || *got != "no deadlifts" {
		t.Fatalf("trim failed: %v %v", got, err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("é", 201)} {
		b := bad
		_, err := NormalizeGenerationRequest(&b)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Field != "request" {
			t.Fatalf("%q must be a request validation error, got %v", bad, err)
		}
	}
	ok := strings.Repeat("é", 200)
	if _, err := NormalizeGenerationRequest(&ok); err != nil {
		t.Fatal("200 runes must be accepted")
	}
}

func TestLocalDateAndNextLocalMidnight(t *testing.T) {
	now := time.Date(2026, 10, 31, 3, 30, 0, 0, time.UTC) // 2026-10-30 23:30 in New York
	date, err := LocalDate(now, "America/New_York")
	if err != nil || date != "2026-10-30" {
		t.Fatalf("local date: %s %v", date, err)
	}
	reset, err := NextLocalMidnight(now, "America/New_York")
	if err != nil || !reset.Equal(time.Date(2026, 10, 31, 4, 0, 0, 0, time.UTC)) {
		t.Fatalf("next midnight: %v %v", reset, err)
	}
	if _, err := NextLocalMidnight(now, "Mars/Olympus"); err == nil {
		t.Fatal("bad timezone must error")
	}
}
