package engine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func input(goal string, days []int, experience string, equipment []string, minutes int) Input {
	return Input{PrimaryGoal: goal, AvailableDays: days, Experience: experience, UsualActivity: "moderate", Equipment: equipment,
		SessionMinutes: minutes, WorkoutCap: min(len(days), 6), Catalog: fixtureCatalog()}
}

func names(w Workout) []string {
	out := make([]string, 0, len(w.Exercises))
	for _, e := range w.Exercises {
		out = append(out, e.Entry.Name)
	}
	return out
}

func mustBuild(t *testing.T, in Input) (Draft, Explanation) {
	t.Helper()
	d, x, err := Build(in)
	if err != nil {
		t.Fatalf("build %s: %v", in.PrimaryGoal, err)
	}
	return d, x
}

func TestBuildHypertrophyThreeDaysIntermediate(t *testing.T) {
	in := input(model.GoalHypertrophy, []int{1, 3, 5}, "intermediate", []string{"barbell", "dumbbells"}, 60)
	d, x := mustBuild(t, in)
	if d.Name != "3-day hypertrophy plan" || len(d.Workouts) != 3 {
		t.Fatalf("draft: %s %d", d.Name, len(d.Workouts))
	}
	for i, w := range d.Workouts {
		if w.Kind != KindFull || w.Weekday != []int{1, 3, 5}[i] || len(w.Exercises) < 5 || len(w.Exercises) > 7 {
			t.Fatalf("workout %d: %s day %d with %d exercises", i, w.Kind, w.Weekday, len(w.Exercises))
		}
		seen := map[string]bool{}
		for _, e := range w.Exercises {
			if seen[e.Entry.Name] {
				t.Fatalf("duplicate %s in workout %d", e.Entry.Name, i)
			}
			seen[e.Entry.Name] = true
			if !e.Entry.EligibleFor(in.Equipment) {
				t.Fatalf("%s needs %v", e.Entry.Name, e.Entry.EquipmentRequired)
			}
			if e.Entry.Modality == "strength" && (e.Reps < 6 || e.Reps > 20 || e.RestSeconds < 60 || e.RestSeconds > 180 || e.Note == "") {
				t.Fatalf("%s targets out of the hypertrophy range or without a note: %+v", e.Entry.Name, e)
			}
		}
		// Heavy days (first, third) use 6–8; the moderate day uses 10–12 (FR-015).
		first := w.Exercises[0]
		if first.Entry.Pattern != model.PatternSquat {
			t.Fatalf("first slot must be a squat: %+v", first)
		}
		if i%2 == 0 && (first.Reps != 6 || first.Sets != 4 || first.RestSeconds != 150 || first.Note != "6 to 8 reps, leave 1 to 2 in reserve.") {
			t.Fatalf("heavy day squat: %+v", first)
		}
		if i%2 == 1 && (first.Reps != 10 || first.Sets != 3 || first.RestSeconds != 90 || first.Note != "10 to 12 reps, leave 1 to 2 in reserve.") {
			t.Fatalf("moderate day squat: %+v", first)
		}
	}
	// Full-body days alternate A and B: day 1 and day 3 share a first lift, day 2 differs.
	if d.Workouts[0].Name != "Full body A" || d.Workouts[1].Name != "Full body B" || d.Workouts[2].Name != "Full body A" {
		t.Fatalf("A/B names: %s %s %s", d.Workouts[0].Name, d.Workouts[1].Name, d.Workouts[2].Name)
	}
	if names(d.Workouts[0])[0] != names(d.Workouts[2])[0] || names(d.Workouts[0])[0] == names(d.Workouts[1])[0] {
		t.Fatalf("A/B alternation: %v %v %v", names(d.Workouts[0]), names(d.Workouts[1]), names(d.Workouts[2]))
	}
	if x.Split != "full-body days" || len(x.MainLifts) == 0 || x.UndulationNote == "" || x.ProgressionLine != progressionLine {
		t.Fatalf("explanation: %+v", x)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	in := input(model.GoalStrength, []int{2, 4}, "advanced", []string{"full_gym"}, 75)
	a, _ := mustBuild(t, in)
	b, _ := mustBuild(t, in)
	if !equalDrafts(a, b) {
		t.Fatal("same input must give the same draft")
	}
}

func equalDrafts(a, b Draft) bool {
	if a.Name != b.Name || len(a.Workouts) != len(b.Workouts) {
		return false
	}
	for i := range a.Workouts {
		if !slices.Equal(names(a.Workouts[i]), names(b.Workouts[i])) {
			return false
		}
		for j := range a.Workouts[i].Exercises {
			x, y := a.Workouts[i].Exercises[j], b.Workouts[i].Exercises[j]
			if x.Entry.ID != y.Entry.ID || x.Sets != y.Sets || x.Reps != y.Reps || x.DurationSeconds != y.DurationSeconds || x.RestSeconds != y.RestSeconds || x.Note != y.Note {
				return false
			}
		}
	}
	return true
}

func TestBeginnerGetsFewerSimplerExercisesAndNoUndulation(t *testing.T) {
	in := input(model.GoalPower, []int{1, 2, 3, 4}, "beginner", []string{"full_gym"}, 90)
	d, x := mustBuild(t, in)
	for _, w := range d.Workouts {
		if len(w.Exercises) > 5 || !w.Heavy {
			t.Fatalf("beginner cap is 5 and every day uses the same targets: %d heavy=%v", len(w.Exercises), w.Heavy)
		}
		for _, e := range w.Exercises {
			if e.Entry.Pattern == model.PatternExplosive {
				t.Fatalf("beginners never get explosive work: %s", e.Entry.Name)
			}
			if e.Entry.Modality == "strength" && !e.Entry.IsCompound() && e.Entry.Pattern != model.PatternCore {
				t.Fatalf("beginners get compounds plus core only: %s", e.Entry.Name)
			}
			if e.Entry.Modality == "strength" && !strings.Contains(e.Note, "2 to 3 in reserve") {
				t.Fatalf("beginner effort cue: %+v", e)
			}
		}
	}
	if !strings.Contains(x.BeginnerNote, "beginner") || x.UndulationNote != "" {
		t.Fatalf("beginner explanation: %+v", x)
	}
	in.PrimaryGoal, in.UsualActivity = model.GoalConditioning, "sedentary"
	in.Equipment = []string{"bodyweight"}
	d, _ = mustBuild(t, in)
	for _, w := range d.Workouts {
		for _, e := range w.Exercises {
			if e.Entry.Impact == model.ImpactHigh {
				t.Fatalf("sedentary beginner got high-impact %s", e.Entry.Name)
			}
		}
	}
}

func TestBodyweightOnly(t *testing.T) {
	in := input(model.GoalGeneralHealth, []int{1, 4}, "intermediate", []string{"bodyweight"}, 45)
	d, x := mustBuild(t, in)
	for _, w := range d.Workouts {
		for _, e := range w.Exercises {
			if !slices.Equal(e.Entry.EquipmentRequired, []string{"bodyweight"}) {
				t.Fatalf("%s is not a bodyweight entry", e.Entry.Name)
			}
		}
		last := w.Exercises[len(w.Exercises)-1]
		if last.Entry.Modality != "cardio" || last.DurationSeconds != 900 || last.Note != noteModerate {
			t.Fatalf("general health ends with a 15-minute finisher with an intensity line: %+v", last)
		}
	}
	if x.EquipmentNote == "" {
		t.Fatal("bodyweight note missing")
	}
}

func TestShortAndMinimalSessions(t *testing.T) {
	in := input(model.GoalBodyComposition, []int{1, 3, 5}, "intermediate", []string{"full_gym"}, 20)
	d, _ := mustBuild(t, in)
	for _, w := range d.Workouts {
		if len(w.Exercises) != 3 {
			t.Fatalf("20-minute session holds 3 exercises, got %d", len(w.Exercises))
		}
	}
	if d.Workouts[0].Exercises[2].Sets != 4 || d.Workouts[0].Exercises[2].DurationSeconds != 180 || !strings.HasPrefix(d.Workouts[0].Exercises[2].Note, "Hard efforts") {
		t.Fatalf("body composition first day uses intervals with a line: %+v", d.Workouts[0].Exercises[2])
	}
	if d.Workouts[1].Exercises[2].DurationSeconds != 900 || d.Workouts[2].Exercises[2].Entry.Modality != "strength" {
		t.Fatalf("second finisher steady, third day none: %+v %+v", d.Workouts[1].Exercises[2], d.Workouts[2].Exercises[2])
	}
	// Under 20 minutes: three compounds at two sets each (FR-017).
	in = input(model.GoalStrength, []int{1, 3}, "intermediate", []string{"barbell"}, 15)
	d, _ = mustBuild(t, in)
	for _, w := range d.Workouts {
		if len(w.Exercises) != 3 {
			t.Fatalf("minimal session holds 3: %v", names(w))
		}
		for _, e := range w.Exercises {
			if e.Sets != minimalSets || !e.Entry.IsCompound() {
				t.Fatalf("minimal session: two sets of compounds, got %+v", e)
			}
		}
	}
}

func TestConditioningDays(t *testing.T) {
	in := input(model.GoalConditioning, []int{1, 2, 3, 4}, "intermediate", []string{"cardio_machine", "dumbbells"}, 45)
	d, x := mustBuild(t, in)
	if x.Split != "cardio and circuit days" || x.ProgressionLine != "" {
		t.Fatalf("conditioning explanation: %+v", x)
	}
	cardio := d.Workouts[0]
	if cardio.Kind != KindCardio || cardio.Exercises[0].Entry.Modality != "cardio" || cardio.Exercises[0].Sets != 4 || cardio.Exercises[0].DurationSeconds != 240 || !strings.HasPrefix(cardio.Exercises[0].Note, "Hard efforts: 4 × 4") {
		t.Fatalf("first cardio day opens with intervals: %+v", cardio.Exercises[0])
	}
	circuit := d.Workouts[1]
	for _, e := range circuit.Exercises {
		if e.Entry.Modality == "strength" && (e.Reps != 15 || e.RestSeconds != 45 || !strings.Contains(e.Note, "keep moving")) {
			t.Fatalf("circuit targets 15 reps rest 45 with the circuit cue: %+v", e)
		}
	}
	if last := circuit.Exercises[len(circuit.Exercises)-1]; last.Entry.Modality != "cardio" || last.DurationSeconds != 600 {
		t.Fatalf("circuit ends with a 10-minute finisher: %+v", last)
	}
	in.Equipment = []string{"bodyweight"}
	d, _ = mustBuild(t, in)
	if d.Workouts[0].Exercises[0].Entry.Modality != "cardio" || !slices.Equal(d.Workouts[0].Exercises[0].Entry.EquipmentRequired, []string{"bodyweight"}) {
		t.Fatalf("bodyweight cardio expected: %+v", d.Workouts[0].Exercises[0].Entry)
	}
}

func TestPowerNonBeginnerLeadsWithExplosiveWork(t *testing.T) {
	in := input(model.GoalPower, []int{1, 3, 5}, "advanced", []string{"barbell", "dumbbells"}, 60)
	d, _ := mustBuild(t, in)
	for i, w := range d.Workouts {
		first := w.Exercises[0]
		if first.Entry.Pattern != model.PatternExplosive || first.Reps != 2 || first.Sets != 5 || first.RestSeconds != 240 || !strings.Contains(first.Note, "fast reps") {
			t.Fatalf("explosive first at 2 × 5 rest 240 with the fast cue: %+v", first)
		}
		// Heavy and moderate strength work alternate behind the opener.
		wantReps := 3
		if i%2 == 1 {
			wantReps = 5
		}
		if w.Exercises[1].Entry.IsCompound() && w.Exercises[1].Reps != wantReps {
			t.Fatalf("day %d main strength work at %d reps: %+v", i, wantReps, w.Exercises[1])
		}
		for _, e := range w.Exercises {
			if e.Entry.Modality == "cardio" {
				t.Fatal("no cardio inside a power session")
			}
		}
	}
	// Bodyweight power athletes now get jumps instead of nothing (migration 034 entries).
	in.Equipment = []string{"bodyweight"}
	d, _ = mustBuild(t, in)
	if first := d.Workouts[0].Exercises[0]; first.Entry.Pattern != model.PatternExplosive || !strings.Contains(first.Entry.Name, "Jump") {
		t.Fatalf("bodyweight power opener should be a jump: %+v", first.Entry)
	}
}

func TestSevenDaysCapAtSix(t *testing.T) {
	in := input(model.GoalHypertrophy, []int{1, 2, 3, 4, 5, 6, 7}, "intermediate", []string{"full_gym"}, 60)
	d, _ := mustBuild(t, in)
	if len(d.Workouts) != 6 || d.Workouts[5].Weekday != 6 {
		t.Fatalf("six workouts on the first six days: %d last day %d", len(d.Workouts), d.Workouts[len(d.Workouts)-1].Weekday)
	}
}

// US2: structured constraints.
func TestAvoidAndFocus(t *testing.T) {
	in := input(model.GoalStrength, []int{1, 3, 5}, "intermediate", []string{"full_gym"}, 60)
	in.Avoid = []string{model.PatternHinge, model.PatternVerticalPush}
	in.Focus = AreaUpper
	d, x := mustBuild(t, in)
	banned := []string{"Deadlift", "Romanian Deadlift", "Good Morning", "Kettlebell Swing", "Overhead Press", "Landmine Press", "Sumo Deadlift", "Hip Thrust", "Dips"}
	for _, w := range d.Workouts {
		upper, lower, strength := 0, 0, 0
		for _, e := range w.Exercises {
			if slices.Contains(banned, e.Entry.Name) || e.Entry.Pattern == model.PatternHinge || e.Entry.Pattern == model.PatternVerticalPush {
				t.Fatalf("avoided pattern appeared: %s", e.Entry.Name)
			}
			if e.Entry.Modality != "strength" {
				continue
			}
			strength++
			switch focusAreaOf(e.Entry.MuscleGroup) {
			case AreaUpper:
				upper++
			case AreaLower:
				lower++
			}
		}
		if upper*3 < strength*2 || lower < 1 {
			t.Fatalf("upper focus wants about two thirds upper with at least one lower: upper %d lower %d of %d (%v)", upper, lower, strength, names(w))
		}
	}
	if !slices.Contains(x.Honoured, "hinges") || !slices.Contains(x.Honoured, "overhead pressing") || !strings.Contains(x.FocusNote, "Upper body") {
		t.Fatalf("explanation must record the honoured limits: %+v", x)
	}
	in.Avoid, in.Focus = nil, AreaLower
	d, _ = mustBuild(t, in)
	for _, w := range d.Workouts {
		lower, strength := 0, 0
		for _, e := range w.Exercises {
			if e.Entry.Modality == "strength" {
				strength++
				if focusAreaOf(e.Entry.MuscleGroup) == AreaLower {
					lower++
				}
			}
		}
		if lower*3 < strength*2 {
			t.Fatalf("lower focus share too small: %d of %d (%v)", lower, strength, names(w))
		}
	}
	in.Focus = AreaCore
	d, _ = mustBuild(t, in)
	if d.Workouts[0].Exercises[0].Entry.Pattern != model.PatternCore {
		t.Fatalf("core focus leads with core: %v", names(d.Workouts[0]))
	}
}

// FR-016: under hypertrophy with a focus, an isolation exercise opens the session.
func TestHypertrophyFocusOpensWithIsolation(t *testing.T) {
	in := input(model.GoalHypertrophy, []int{1, 3, 5}, "intermediate", []string{"full_gym"}, 60)
	in.Focus = AreaUpper
	d, _ := mustBuild(t, in)
	first := d.Workouts[0].Exercises[0]
	if first.Entry.Pattern != model.PatternIsolation || focusAreaOf(first.Entry.MuscleGroup) != AreaUpper {
		t.Fatalf("hypertrophy upper focus opens with an upper isolation: %v", names(d.Workouts[0]))
	}
	// Strength keeps compounds first.
	in.PrimaryGoal = model.GoalStrength
	d, _ = mustBuild(t, in)
	if !d.Workouts[0].Exercises[0].Entry.IsCompound() {
		t.Fatalf("strength focus keeps compounds first: %v", names(d.Workouts[0]))
	}
}

func TestTooConstrained(t *testing.T) {
	in := input(model.GoalStrength, []int{1}, "intermediate", []string{"bodyweight"}, 20)
	in.Avoid = []string{model.PatternHorizontalPush, model.PatternLunge, model.PatternIsolation, model.PatternCore, model.PatternLocomotion, model.PatternSquat}
	_, _, err := Build(in)
	if !errors.Is(err, ErrTooConstrained) {
		t.Fatalf("want ErrTooConstrained, got %v", err)
	}
	in.Equipment = []string{"barbell"}
	if _, _, err := Build(in); err != nil {
		t.Fatalf("barbell with six avoided patterns must still build: %v", err)
	}
	in.Avoid = []string{model.PatternHinge}
	in.Equipment = []string{"bands"}
	in.SessionMinutes = 60
	d, x := mustBuild(t, in)
	if len(d.Workouts[0].Exercises) < 3 || len(x.Dropped) == 0 {
		t.Fatalf("bands fall back to bodyweight and note what was dropped: %v %+v", names(d.Workouts[0]), x)
	}
}

// US3: variants.
func TestVariantsRotateWithinPatterns(t *testing.T) {
	in := input(model.GoalHypertrophy, []int{1, 3, 5}, "intermediate", []string{"full_gym"}, 60)
	base, _ := mustBuild(t, in)
	seen := map[string]bool{strings.Join(names(base.Workouts[0]), "|"): true}
	for v := 1; v <= 3; v++ {
		in.Variant = v
		d, _ := mustBuild(t, in)
		for i := range d.Workouts {
			if d.Workouts[i].Kind != base.Workouts[i].Kind || len(d.Workouts[i].Exercises) != len(base.Workouts[i].Exercises) {
				t.Fatalf("variant %d changed the structure", v)
			}
			if d.Workouts[i].Exercises[0].Reps != base.Workouts[i].Exercises[0].Reps {
				t.Fatalf("variant %d changed the targets", v)
			}
			if slices.Equal(names(d.Workouts[i]), names(base.Workouts[i])) {
				t.Fatalf("variant %d workout %d repeats variant 0: %v", v, i, names(d.Workouts[i]))
			}
		}
		key := strings.Join(names(d.Workouts[0]), "|")
		if seen[key] {
			t.Fatalf("variant %d repeats an earlier variant", v)
		}
		seen[key] = true
	}
	in = input(model.GoalStrength, []int{1}, "intermediate", []string{"dumbbells"}, 60)
	in.Avoid = []string{model.PatternSquat}
	a, _ := mustBuild(t, in)
	in.Variant = 5
	b, _ := mustBuild(t, in)
	if len(a.Workouts[0].Exercises) != len(b.Workouts[0].Exercises) {
		t.Fatal("variant must not change the exercise count")
	}
}

// US4: secondary goals.
func TestSecondaryGoals(t *testing.T) {
	in := input(model.GoalStrength, []int{1, 3, 5}, "intermediate", []string{"barbell", "cardio_machine"}, 60)
	in.SecondaryGoals = []string{model.GoalConditioning}
	d, x := mustBuild(t, in)
	finishers := 0
	for i, w := range d.Workouts {
		for _, e := range w.Exercises {
			if e.Entry.Modality == "cardio" {
				finishers++
				if e.DurationSeconds != 600 || e.Note != noteModerate {
					t.Fatalf("secondary conditioning finisher is 10 minutes with a line: %+v", e)
				}
			} else if e.Entry.IsCompound() && e.Entry.Pattern != model.PatternCore {
				want := 5
				if i%2 == 1 {
					want = 8
				}
				if e.Reps != want {
					t.Fatalf("primary strength targets untouched (day %d wants %d): %+v", i, want, e)
				}
			}
		}
	}
	if finishers != 2 || !strings.Contains(x.SecondaryNote, "Conditioning") {
		t.Fatalf("two finishers and a note: %d %+v", finishers, x)
	}

	in.SecondaryGoals = []string{model.GoalHypertrophy, model.GoalPower}
	d, _ = mustBuild(t, in)
	w := d.Workouts[0]
	if w.Exercises[0].Entry.Pattern != model.PatternExplosive || w.Exercises[0].Reps != 3 {
		t.Fatalf("secondary power adds an explosive opener: %+v", w.Exercises[0])
	}
	if last := w.Exercises[len(w.Exercises)-1]; last.Entry.Pattern != model.PatternIsolation || last.Reps != 12 {
		t.Fatalf("secondary hypertrophy adds an isolation slot at 12 reps: %+v", last)
	}

	in = input(model.GoalHypertrophy, []int{1, 3}, "intermediate", []string{"barbell"}, 60)
	in.SecondaryGoals = []string{model.GoalStrength}
	d, _ = mustBuild(t, in)
	first := d.Workouts[0].Exercises[0]
	if first.Reps != 5 || first.Sets != 4 || first.RestSeconds != 180 || d.Workouts[0].Exercises[1].Reps != 6 {
		t.Fatalf("secondary strength makes the first compound heavier and leaves the rest: %+v %+v", first, d.Workouts[0].Exercises[1])
	}

	in.SecondaryGoals = nil
	d, x = mustBuild(t, in)
	for _, w := range d.Workouts {
		for _, e := range w.Exercises {
			if e.Entry.Modality == "cardio" {
				t.Fatal("no secondary goal, no cardio in a hypertrophy draft")
			}
		}
	}
	if x.SecondaryNote != "" {
		t.Fatal("no secondary note without secondary goals")
	}
}

// FR-014: weekly sets per muscle group stay inside the goal's band.
func TestWeeklySetBands(t *testing.T) {
	in := input(model.GoalHypertrophy, []int{1, 2, 3, 4, 5, 6}, "advanced", []string{"full_gym"}, 90)
	d, _ := mustBuild(t, in)
	band := weeklySetBands[model.GoalHypertrophy]
	for group, sets := range weeklySets(d) {
		if sets > band.Max {
			t.Fatalf("%s has %d weekly sets, above %d", group, sets, band.Max)
		}
	}
	// The rule is covered alone: an over-served group loses an isolation exercise.
	b := newBuilder(in, 6)
	byName := map[string]model.CatalogExercise{}
	for _, e := range fixtureCatalog() {
		byName[e.Name] = e
	}
	over := Draft{Workouts: []Workout{{Kind: KindUpper, Exercises: []Exercise{
		{Entry: byName["Barbell Bench Press"], Sets: 8}, {Entry: byName["Incline Barbell Bench Press"], Sets: 8}, {Entry: byName["Dumbbell Bench Press"], Sets: 8},
		{Entry: byName["Cable Fly"], Sets: 8},
	}}}}
	ruleWeeklySetBands(b, &over)
	if slices.Contains(names(over.Workouts[0]), "Cable Fly") || weeklySets(over)["chest"] > band.Max {
		t.Fatalf("over-served chest should lose its isolation exercise first and end inside the band: %v sets %d", names(over.Workouts[0]), weeklySets(over)["chest"])
	}
	// A focus area may carry half again its band.
	in.Focus = AreaUpper
	fb := newBuilder(in, 6)
	if fb.bandMax(band, "chest") != band.Max+band.Max/2 || fb.bandMax(band, "quads") != band.Max {
		t.Fatalf("focus relaxes the band for its area only: %d %d", fb.bandMax(band, "chest"), fb.bandMax(band, "quads"))
	}
}

// FR-020: every rule has a name and the order reads top to bottom.
func TestRuleListIsNamedAndOrdered(t *testing.T) {
	want := []string{"cardio day", "explosive opener", "focus opener", "fill slots", "circuit finisher", "secondary strength", "secondary hypertrophy", "cardio finisher", "top up"}
	for i, rule := range workoutRules {
		if rule.name != want[i] || rule.apply == nil {
			t.Fatalf("workout rule %d: %q", i, rule.name)
		}
	}
	if len(weeklyRules) != 1 || weeklyRules[0].name != "weekly set bands" {
		t.Fatalf("weekly rules: %+v", weeklyRules)
	}
}
