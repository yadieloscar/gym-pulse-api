package engine

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

var denyList = regexp.MustCompile(`(?i)\b(diet|weight|lose|burn|medical|injury|calorie|fat)\b`)

func TestRationaleIsShortPlainAndComplete(t *testing.T) {
	x := Explanation{Split: "full-body days", Days: []int{1, 3, 5}, MainLifts: []string{"Back Squat", "Barbell Bench Press", "Barbell Row"},
		SecondaryNote: "Conditioning gets a smaller share so your main goal keeps the structure.", FocusNote: focusNote(AreaUpper),
		UndulationNote: "Heavier and moderate days alternate through the week.", ProgressionLine: progressionLine,
		Honoured: []string{"hinges"}, Dropped: []string{"carry"}}
	text := Rationale(x)
	for _, want := range []string{"Full-body days on Monday, Wednesday, and Friday.", "Built around back squat, barbell bench press, and barbell row.", "Heavier and moderate days alternate", "Conditioning gets", "Upper body", "No hinges, as you asked.", "Could not fit carry"} {
		if !strings.Contains(text, want) {
			t.Errorf("rationale missing %q: %s", want, text)
		}
	}
	if !strings.HasSuffix(text, progressionLine) {
		t.Fatalf("rationale must end with the progression line: %s", text)
	}
	if denyList.MatchString(text) {
		t.Fatalf("rationale uses a denied word: %s", text)
	}
	if len([]rune(text)) > maxRationaleRunes {
		t.Fatalf("rationale too long: %d", len([]rune(text)))
	}
	long := x
	long.Dropped = make([]string, 60)
	for i := range long.Dropped {
		long.Dropped[i] = "a very long slot name that keeps going"
	}
	if got := Rationale(long); len([]rune(got)) > maxRationaleRunes || !strings.HasSuffix(got, progressionLine) {
		t.Fatalf("rationale must be capped and keep the progression line: %d %s", len([]rune(got)), got)
	}
	for _, goal := range model.ValidTrainingGoals {
		in := input(goal, []int{1, 2, 4}, "beginner", []string{"bodyweight"}, 30)
		in.SecondaryGoals = []string{model.GoalConditioning}
		if goal == model.GoalConditioning {
			in.SecondaryGoals = []string{model.GoalStrength}
		}
		_, explanation, err := Build(in)
		if err != nil {
			t.Fatal(err)
		}
		if text := Rationale(explanation); denyList.MatchString(text) || len([]rune(text)) > maxRationaleRunes {
			t.Fatalf("%s rationale: %s", goal, text)
		}
	}
}

// Every exercise note is short, plain, and present (FR-013).
func TestNotesArePlainAndPresent(t *testing.T) {
	for _, goal := range model.ValidTrainingGoals {
		d, _ := mustBuild(t, input(goal, []int{1, 3, 5}, "intermediate", []string{"full_gym"}, 60))
		for _, w := range d.Workouts {
			for _, e := range w.Exercises {
				if e.Note == "" || len([]rune(e.Note)) > 200 || denyList.MatchString(e.Note) {
					t.Fatalf("%s: bad note on %s: %q", goal, e.Entry.Name, e.Note)
				}
				if e.Entry.Modality == "strength" && e.Reps > 0 && !strings.Contains(e.Note, "reps") {
					t.Fatalf("%s: strength note names the rep range: %q", goal, e.Note)
				}
			}
		}
	}
}
