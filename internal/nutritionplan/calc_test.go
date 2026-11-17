package nutritionplan

import (
	"errors"
	"slices"
	"testing"
)

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
func mustResolve(t *testing.T, in Input) Input {
	t.Helper()
	out, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve(%+v) = %v, want nil", in, err)
	}
	return out
}

// imperialMale is the spec's worked example: 180 lb, 70 in, 30 years, male,
// 4 strength days and 1 cardio day, maintenance.
func imperialMale() Input {
	return Input{
		UnitSystem: UnitImperial, Weight: 180, Height: 70, AgeYears: 30, Sex: SexMale,
		StrengthDays: 4, CardioDays: 1, Goal: GoalMaintenance,
	}
}

func TestCalculateWorkedExamples(t *testing.T) {
	female := imperialMale()
	female.Sex = SexFemale
	female.Weight, female.Height = 140, 65
	female.Goal = GoalFatLoss

	tests := []struct {
		name string
		in   Input
		want Targets
	}{
		{
			// BMR 4.536·180 + 15.875·70 − 150 + 5 = 1782.73; ×1.55 = 2763.2;
			// protein 0.85·180 = 153; fat 0.33·180 = 59.4;
			// carbs (2763 − 612 − 531) / 4 = 405.
			name: "imperial male maintenance",
			in:   imperialMale(),
			want: Targets{BMRKcal: 1783, TDEEKcal: 2763, AdjustmentKcal: 0, TargetKcal: 2763, ProteinG: 153, CarbsG: 405, FatG: 59},
		},
		{
			// BMR 4.536·140 + 15.875·65 − 150 − 161 = 1355.915; ×1.55 = 2101.7;
			// target 2102 − 400 = 1702; protein 0.95·140 = 133; fat 0.33·140 = 46.2;
			// carbs (1702 − 532 − 414) / 4 = 189.
			name: "imperial female fat loss",
			in:   female,
			want: Targets{BMRKcal: 1356, TDEEKcal: 2102, AdjustmentKcal: -400, TargetKcal: 1702, ProteinG: 133, CarbsG: 189, FatG: 46},
		},
		{
			// BMR 10·80 + 6.25·180 − 125 + 5 = 1805; light ×1.375 = 2481.9;
			// target 2482 + 300 = 2782; 80 kg = 176.37 lb; protein 149.9 → 150;
			// fat 58.2 → 58; carbs (2782 − 600 − 522) / 4 = 415.
			name: "metric male muscle gain",
			in: Input{
				UnitSystem: UnitMetric, Weight: 80, Height: 180, AgeYears: 25, Sex: SexMale,
				StrengthDays: 3, CardioDays: 0, Goal: GoalMuscleGain,
			},
			want: Targets{BMRKcal: 1805, TDEEKcal: 2482, AdjustmentKcal: 300, TargetKcal: 2782, ProteinG: 150, CarbsG: 415, FatG: 58},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Calculate(mustResolve(t, tt.in))
			if got != tt.want {
				t.Errorf("Calculate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCalculateMetricMatchesImperial(t *testing.T) {
	imperial := Calculate(mustResolve(t, imperialMale()))
	metric := imperialMale()
	metric.UnitSystem = UnitMetric
	metric.Weight, metric.Height = 81.6, 177.8 // 180 lb = 81.65 kg, 70 in = 177.8 cm
	got := Calculate(mustResolve(t, metric))
	for _, pair := range []struct {
		name      string
		got, want int
		tolerance int
	}{
		{"bmr", got.BMRKcal, imperial.BMRKcal, 1},
		{"tdee", got.TDEEKcal, imperial.TDEEKcal, 2},
		{"protein", got.ProteinG, imperial.ProteinG, 1},
		{"fat", got.FatG, imperial.FatG, 1},
	} {
		if d := pair.got - pair.want; d < -pair.tolerance || d > pair.tolerance {
			t.Errorf("%s: metric %d, imperial %d, want within %d", pair.name, pair.got, pair.want, pair.tolerance)
		}
	}
}

func TestGoalAdjustments(t *testing.T) {
	base := Calculate(mustResolve(t, imperialMale()))
	for _, goal := range Goals {
		t.Run(goal.ID, func(t *testing.T) {
			in := imperialMale()
			in.Goal = goal.ID
			got := Calculate(mustResolve(t, in))
			if got.AdjustmentKcal != goal.DefaultAdjustment {
				t.Errorf("default adjustment = %d, want %d", got.AdjustmentKcal, goal.DefaultAdjustment)
			}
			if got.TargetKcal != base.TDEEKcal+goal.DefaultAdjustment {
				t.Errorf("target = %d, want %d", got.TargetKcal, base.TDEEKcal+goal.DefaultAdjustment)
			}
			if want := int(180*goal.DefaultProteinPerLb + 0.5); got.ProteinG != want {
				t.Errorf("protein = %d, want %d", got.ProteinG, want)
			}
			for _, edge := range []int{goal.MinAdjustmentKcal, goal.MaxAdjustmentKcal} {
				in.AdjustmentKcal = intPtr(edge)
				if _, err := Resolve(in); err != nil {
					t.Errorf("adjustment %d rejected: %v", edge, err)
				}
			}
			for _, outside := range []int{goal.MinAdjustmentKcal - 1, goal.MaxAdjustmentKcal + 1} {
				in.AdjustmentKcal = intPtr(outside)
				assertField(t, in, "calorie_adjustment_kcal")
			}
		})
	}
}

func TestActivitySuggestionAndOverride(t *testing.T) {
	for sessions, want := range map[int]string{0: LevelLight, 1: LevelLight, 3: LevelLight, 4: LevelModerate, 5: LevelModerate, 6: LevelVery, 7: LevelVery, 14: LevelVery} {
		strength := min(sessions, 7)
		if got := SuggestLevel(strength, sessions-strength).ID; got != want {
			t.Errorf("SuggestLevel(%d sessions) = %s, want %s", sessions, got, want)
		}
	}
	in := imperialMale()
	in.ActivityLevel = LevelVery
	got := Calculate(mustResolve(t, in))
	if want := 3075; got.TDEEKcal != want { // 1782.73 × 1.725 = 3075.2
		t.Errorf("override TDEE = %d, want %d", got.TDEEKcal, want)
	}
	if SuggestLevel(in.StrengthDays, in.CardioDays).ID != LevelModerate {
		t.Error("override must not change the suggestion")
	}
}

func TestMacroRates(t *testing.T) {
	in := imperialMale()
	in.ProteinPerLb, in.FatPerLb = floatPtr(1.1), floatPtr(0.45)
	got := Calculate(mustResolve(t, in))
	if got.ProteinG != 198 || got.FatG != 81 {
		t.Errorf("protein, fat = %d, %d, want 198, 81", got.ProteinG, got.FatG)
	}
	if want := (2763 - 4*198 - 9*81 + 2) / 4; got.CarbsG != want {
		t.Errorf("carbs = %d, want %d", got.CarbsG, want)
	}
	for field, mutate := range map[string]func(*Input){
		"protein_g_per_lb": func(i *Input) { i.ProteinPerLb = floatPtr(1.11) },
		"fat_g_per_lb":     func(i *Input) { i.FatPerLb = floatPtr(0.24) },
	} {
		bad := imperialMale()
		mutate(&bad)
		assertField(t, bad, field)
	}
	bad := imperialMale()
	bad.ProteinPerLb = floatPtr(0.855)
	assertField(t, bad, "protein_g_per_lb")
}

func TestCarbsFloorAndBelowBMRNotices(t *testing.T) {
	// A heavy, short, older athlete on the largest reduction with the highest
	// protein and fat rates: protein and fat alone exceed the target.
	in := Input{
		UnitSystem: UnitImperial, Weight: 400, Height: 60, AgeYears: 70, Sex: SexFemale,
		StrengthDays: 0, CardioDays: 0, Goal: GoalFatLoss, AdjustmentKcal: intPtr(-500),
		ProteinPerLb: floatPtr(1.1), FatPerLb: floatPtr(0.45),
	}
	in = mustResolve(t, in)
	got := Calculate(in)
	if got.CarbsG != 0 {
		t.Errorf("carbs = %d, want 0", got.CarbsG)
	}
	if n := notices(in, got, nil); !slices.Contains(n, NoticeCarbsFloor) {
		t.Errorf("notices = %v, want %s", n, NoticeCarbsFloor)
	}

	low := imperialMale()
	low.Goal, low.StrengthDays, low.CardioDays = GoalFatLoss, 0, 0
	low.Weight, low.AdjustmentKcal = 100, intPtr(-500)
	low = mustResolve(t, low)
	lowTargets := Calculate(low) // BMR 1420 × 1.375 = 1952 − 500 = 1452 ≥ BMR
	if n := notices(low, lowTargets, nil); slices.Contains(n, NoticeBelowBMR) {
		t.Errorf("notices = %v, want no %s at target %d over BMR %d", n, NoticeBelowBMR, lowTargets.TargetKcal, lowTargets.BMRKcal)
	}
	if n := notices(low, Targets{BMRKcal: 1500, TargetKcal: 1499, ProteinG: 100, FatG: 40}, nil); !slices.Contains(n, NoticeBelowBMR) {
		t.Errorf("notices = %v, want %s", n, NoticeBelowBMR)
	}
	if n := notices(mustResolve(t, imperialMale()), Calculate(mustResolve(t, imperialMale())), nil); len(n) != 0 || n == nil {
		t.Errorf("notices = %#v, want empty non-nil", n)
	}
}

func TestCycleKeepsTheWeek(t *testing.T) {
	for training := 1; training <= 6; training++ {
		for carbs := range 701 {
			in := imperialMale()
			in.CarbCycling = true
			in.StrengthDays, in.CardioDays = training, 0
			in = mustResolve(t, in)
			targets := Targets{TargetKcal: 3000, ProteinG: 150, CarbsG: carbs, FatG: 60}
			c := cycle(in, targets)
			if c == nil {
				t.Fatalf("training=%d carbs=%d: cycle = nil", training, carbs)
			}
			if got, want := c.TrainingDays*c.Training.CarbsG+c.RestDays*c.Rest.CarbsG, 7*carbs; got != want {
				t.Fatalf("training=%d carbs=%d: week carbs = %d, want %d", training, carbs, got, want)
			}
			standard := newDayMacros(targets.ProteinG, carbs, targets.FatG)
			if c.WeeklyAverageKcal != standard.EnergyKcal {
				t.Fatalf("training=%d carbs=%d: weekly average = %d, want %d", training, carbs, c.WeeklyAverageKcal, standard.EnergyKcal)
			}
			if c.Training.ProteinG != 150 || c.Rest.ProteinG != 150 || c.Training.FatG != 60 || c.Rest.FatG != 60 {
				t.Fatalf("training=%d carbs=%d: protein or fat changed: %+v", training, carbs, c)
			}
			if c.Rest.CarbsG > carbs || c.Training.CarbsG < carbs {
				t.Fatalf("training=%d carbs=%d: rest %d, training %d", training, carbs, c.Rest.CarbsG, c.Training.CarbsG)
			}
			if carbs >= 200 {
				shift := float64(carbs-c.Rest.CarbsG) / float64(carbs)
				if shift < 0.15 || shift > 0.20 {
					t.Fatalf("training=%d carbs=%d: rest shift = %.3f, want 0.15–0.20", training, carbs, shift)
				}
			}
		}
	}
}

func TestCycleNotApplied(t *testing.T) {
	for _, days := range [][2]int{{0, 0}, {7, 0}, {4, 3}, {7, 7}} {
		in := imperialMale()
		in.CarbCycling = true
		in.StrengthDays, in.CardioDays = days[0], days[1]
		in = mustResolve(t, in)
		targets := Calculate(in)
		c := cycle(in, targets)
		if c != nil {
			t.Errorf("days %v: cycle = %+v, want nil", days, c)
		}
		if n := notices(in, targets, c); !slices.Contains(n, NoticeCyclingNotApplied) {
			t.Errorf("days %v: notices = %v, want %s", days, n, NoticeCyclingNotApplied)
		}
	}
	off := mustResolve(t, imperialMale())
	if cycle(off, Calculate(off)) != nil {
		t.Error("cycle with cycling off must be nil")
	}
}

func TestResolveRejectsOutOfRange(t *testing.T) {
	for field, mutate := range map[string]func(*Input){
		"unit_system":    func(i *Input) { i.UnitSystem = "stone" },
		"weight":         func(i *Input) { i.Weight = 65.9 },
		"height":         func(i *Input) { i.Height = 96.5 },
		"age_years":      func(i *Input) { i.AgeYears = 17 },
		"sex":            func(i *Input) { i.Sex = "" },
		"strength_days":  func(i *Input) { i.StrengthDays = 8 },
		"cardio_days":    func(i *Input) { i.CardioDays = -1 },
		"activity_level": func(i *Input) { i.ActivityLevel = "sedentary" },
		"goal":           func(i *Input) { i.Goal = "bulk" },
	} {
		in := imperialMale()
		mutate(&in)
		assertField(t, in, field)
	}
	twoDecimals := imperialMale()
	twoDecimals.Weight = 180.25
	assertField(t, twoDecimals, "weight")

	metric := imperialMale()
	metric.UnitSystem, metric.Weight, metric.Height = UnitMetric, 29.9, 180
	assertField(t, metric, "weight")
	metric.Weight, metric.Height = 80, 121
	assertField(t, metric, "height")
}

func TestSplitByWeightsSumsExactly(t *testing.T) {
	for total := range 501 {
		for _, weights := range [][]int{{25, 25, 25, 25}, {25, 20, 35, 20}, {30, 30, 10, 30}} {
			parts := splitByWeights(total, weights)
			sum := 0
			for i, p := range parts {
				sum += p
				exact := float64(total) * float64(weights[i]) / 100
				if d := float64(p) - exact; d <= -1 || d >= 1 {
					t.Fatalf("total=%d weights=%v part %d = %d, exact %.2f", total, weights, i, p, exact)
				}
			}
			if sum != total {
				t.Fatalf("total=%d weights=%v sum = %d", total, weights, sum)
			}
		}
	}
}

func assertField(t *testing.T, in Input, field string) {
	t.Helper()
	_, err := Resolve(in)
	var fe *FieldError
	if !errors.As(err, &fe) {
		t.Errorf("Resolve() error = %v, want FieldError on %s", err, field)
		return
	}
	if fe.Field != field {
		t.Errorf("Resolve() field = %s, want %s (%s)", fe.Field, field, fe.Message)
	}
}
