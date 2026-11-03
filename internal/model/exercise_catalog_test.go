package model

import "testing"

func TestCatalogExerciseEligibleFor(t *testing.T) {
	barbell := CatalogExercise{EquipmentRequired: []string{"barbell"}}
	bodyweight := CatalogExercise{EquipmentRequired: []string{"bodyweight"}}
	unseeded := CatalogExercise{}
	cases := []struct {
		name      string
		entry     CatalogExercise
		equipment []string
		want      bool
	}{
		{"required present", barbell, []string{"dumbbells", "barbell"}, true},
		{"required absent", barbell, []string{"dumbbells"}, false},
		{"full gym opens everything", barbell, []string{"full_gym"}, true},
		{"bodyweight always eligible", bodyweight, []string{"machines"}, true},
		{"bodyweight with nothing listed", bodyweight, nil, true},
		{"unseeded never eligible", unseeded, []string{"full_gym"}, false},
	}
	for _, tc := range cases {
		if got := tc.entry.EligibleFor(tc.equipment); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	for _, pattern := range MovementPatterns {
		if !IsMovementPattern(pattern) {
			t.Fatalf("%s must be a movement pattern", pattern)
		}
	}
	if IsMovementPattern("yoga") {
		t.Fatal("unknown pattern accepted")
	}
}
