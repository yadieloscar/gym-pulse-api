package engine

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// fixtureCatalog mirrors migrations 008, 032, and 034: every real entry with
// its programming metadata, so engine tests run against the shipped catalog.
func fixtureCatalog() []model.CatalogExercise {
	compound, isolation := "compound", "isolation"
	_ = isolation
	var out []model.CatalogExercise
	add := func(i int, name, category, modality string, mechanic *string, sort int, pattern, group, equipment, impact string, sub string) {
		var subPattern *string
		if sub != "" {
			subPattern = &sub
		}
		out = append(out, model.CatalogExercise{ID: uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)), Name: name, Category: category, Modality: modality, Mechanic: mechanic, SortOrder: sort, Pattern: pattern, MuscleGroup: group, EquipmentRequired: []string{equipment}, Impact: impact, SubPattern: subPattern})
	}
	add(0, "Barbell Bench Press", "push", "strength", &compound, 1, "horizontal_push", "chest", "barbell", "low", "")
	add(1, "Incline Barbell Bench Press", "push", "strength", &compound, 2, "horizontal_push", "chest", "barbell", "low", "incline_push")
	add(2, "Dumbbell Bench Press", "push", "strength", &compound, 3, "horizontal_push", "chest", "dumbbells", "low", "")
	add(3, "Incline Dumbbell Press", "push", "strength", &compound, 4, "horizontal_push", "chest", "dumbbells", "low", "incline_push")
	add(4, "Overhead Press", "push", "strength", &compound, 5, "vertical_push", "shoulders", "barbell", "low", "")
	add(5, "Seated Dumbbell Shoulder Press", "push", "strength", &compound, 6, "vertical_push", "shoulders", "dumbbells", "low", "")
	add(6, "Close-Grip Bench Press", "push", "strength", &compound, 7, "horizontal_push", "arms", "barbell", "low", "")
	add(7, "Dips", "push", "strength", &compound, 8, "vertical_push", "chest", "full_gym", "low", "")
	add(8, "Push-Up", "push", "strength", &compound, 9, "horizontal_push", "chest", "bodyweight", "low", "")
	add(9, "Cable Fly", "push", "strength", &isolation, 10, "isolation", "chest", "machines", "low", "chest_isolation")
	add(10, "Dumbbell Fly", "push", "strength", &isolation, 11, "isolation", "chest", "dumbbells", "low", "chest_isolation")
	add(11, "Lateral Raise", "push", "strength", &isolation, 12, "isolation", "shoulders", "dumbbells", "low", "shoulder_isolation")
	add(12, "Triceps Pushdown", "push", "strength", &isolation, 13, "isolation", "arms", "machines", "low", "triceps")
	add(13, "Overhead Triceps Extension", "push", "strength", &isolation, 14, "isolation", "arms", "dumbbells", "low", "triceps")
	add(14, "Deadlift", "pull", "strength", &compound, 1, "hinge", "hamstrings", "barbell", "low", "")
	add(15, "Pull-Up", "pull", "strength", &compound, 2, "vertical_pull", "back", "full_gym", "low", "")
	add(16, "Chin-Up", "pull", "strength", &compound, 3, "vertical_pull", "back", "full_gym", "low", "")
	add(17, "Lat Pulldown", "pull", "strength", &compound, 4, "vertical_pull", "back", "machines", "low", "")
	add(18, "Barbell Row", "pull", "strength", &compound, 5, "horizontal_pull", "back", "barbell", "low", "")
	add(19, "Dumbbell Row", "pull", "strength", &compound, 6, "horizontal_pull", "back", "dumbbells", "low", "")
	add(20, "Seated Cable Row", "pull", "strength", &compound, 7, "horizontal_pull", "back", "machines", "low", "")
	add(21, "T-Bar Row", "pull", "strength", &compound, 8, "horizontal_pull", "back", "full_gym", "low", "")
	add(22, "Face Pull", "pull", "strength", &isolation, 9, "isolation", "shoulders", "machines", "low", "rear_delt")
	add(23, "Rear Delt Fly", "pull", "strength", &isolation, 10, "isolation", "shoulders", "dumbbells", "low", "rear_delt")
	add(24, "Barbell Curl", "pull", "strength", &isolation, 11, "isolation", "arms", "barbell", "low", "biceps")
	add(25, "Dumbbell Curl", "pull", "strength", &isolation, 12, "isolation", "arms", "dumbbells", "low", "biceps")
	add(26, "Hammer Curl", "pull", "strength", &isolation, 13, "isolation", "arms", "dumbbells", "low", "biceps")
	add(27, "Preacher Curl", "pull", "strength", &isolation, 14, "isolation", "arms", "machines", "low", "biceps")
	add(28, "Back Squat", "legs", "strength", &compound, 1, "squat", "quads", "barbell", "low", "")
	add(29, "Front Squat", "legs", "strength", &compound, 2, "squat", "quads", "barbell", "low", "")
	add(30, "Goblet Squat", "legs", "strength", &compound, 3, "squat", "quads", "dumbbells", "low", "")
	add(31, "Romanian Deadlift", "legs", "strength", &compound, 4, "hinge", "hamstrings", "barbell", "low", "")
	add(32, "Leg Press", "legs", "strength", &compound, 5, "squat", "quads", "machines", "low", "")
	add(33, "Walking Lunge", "legs", "strength", &compound, 6, "lunge", "quads", "bodyweight", "low", "")
	add(34, "Bulgarian Split Squat", "legs", "strength", &compound, 7, "lunge", "quads", "bodyweight", "low", "")
	add(35, "Hip Thrust", "legs", "strength", &compound, 8, "hinge", "glutes", "barbell", "low", "")
	add(36, "Step-Up", "legs", "strength", &compound, 9, "lunge", "quads", "bodyweight", "low", "")
	add(37, "Leg Extension", "legs", "strength", &isolation, 10, "isolation", "quads", "machines", "low", "quad_isolation")
	add(38, "Lying Leg Curl", "legs", "strength", &isolation, 11, "isolation", "hamstrings", "machines", "low", "hamstring_isolation")
	add(39, "Standing Calf Raise", "legs", "strength", &isolation, 12, "isolation", "calves", "bodyweight", "low", "calf")
	add(40, "Seated Calf Raise", "legs", "strength", &isolation, 13, "isolation", "calves", "machines", "low", "calf")
	add(41, "Hip Abduction", "legs", "strength", &isolation, 14, "isolation", "glutes", "machines", "low", "glute_isolation")
	add(42, "Sumo Deadlift", "lower", "strength", &compound, 1, "hinge", "hamstrings", "barbell", "low", "")
	add(43, "Hack Squat", "lower", "strength", &compound, 2, "squat", "quads", "machines", "low", "")
	add(44, "Good Morning", "lower", "strength", &compound, 3, "hinge", "hamstrings", "barbell", "low", "")
	add(45, "Single-Leg Press", "lower", "strength", &compound, 4, "squat", "quads", "machines", "low", "")
	add(46, "Nordic Hamstring Curl", "lower", "strength", &isolation, 5, "isolation", "hamstrings", "bodyweight", "low", "hamstring_isolation")
	add(47, "Arnold Press", "upper", "strength", &compound, 1, "vertical_push", "shoulders", "dumbbells", "low", "")
	add(48, "Machine Chest Press", "upper", "strength", &compound, 2, "horizontal_push", "chest", "machines", "low", "")
	add(49, "Machine Row", "upper", "strength", &compound, 3, "horizontal_pull", "back", "machines", "low", "")
	add(50, "Landmine Press", "upper", "strength", &compound, 4, "vertical_push", "shoulders", "barbell", "low", "")
	add(51, "Cable Crossover", "upper", "strength", &isolation, 5, "isolation", "chest", "machines", "low", "chest_isolation")
	add(52, "Plank", "core", "strength", &isolation, 1, "core", "core", "bodyweight", "low", "")
	add(53, "Side Plank", "core", "strength", &isolation, 2, "core", "core", "bodyweight", "low", "")
	add(54, "Hanging Leg Raise", "core", "strength", &compound, 3, "core", "core", "full_gym", "low", "")
	add(55, "Ab Wheel Rollout", "core", "strength", &compound, 4, "core", "core", "full_gym", "low", "")
	add(56, "Cable Crunch", "core", "strength", &isolation, 5, "core", "core", "machines", "low", "")
	add(57, "Russian Twist", "core", "strength", &isolation, 6, "core", "core", "bodyweight", "low", "")
	add(58, "Dead Bug", "core", "strength", &isolation, 7, "core", "core", "bodyweight", "low", "")
	add(59, "Bicycle Crunch", "core", "strength", &isolation, 8, "core", "core", "bodyweight", "low", "")
	add(60, "Clean and Press", "full", "strength", &compound, 1, "explosive", "full_body", "barbell", "high", "")
	add(61, "Power Clean", "full", "strength", &compound, 2, "explosive", "full_body", "barbell", "high", "")
	add(62, "Kettlebell Swing", "full", "strength", &compound, 3, "explosive", "glutes", "dumbbells", "low", "")
	add(63, "Thruster", "full", "strength", &compound, 4, "explosive", "full_body", "barbell", "low", "")
	add(64, "Burpee", "full", "strength", &compound, 5, "locomotion", "full_body", "bodyweight", "high", "")
	add(65, "Farmer's Carry", "full", "strength", &compound, 6, "carry", "full_body", "dumbbells", "low", "")
	add(66, "Turkish Get-Up", "full", "strength", &compound, 7, "carry", "full_body", "dumbbells", "low", "")
	add(67, "Treadmill Run", "cardio", "cardio", nil, 1, "locomotion", "full_body", "cardio_machine", "high", "")
	add(68, "Incline Treadmill Walk", "cardio", "cardio", nil, 2, "locomotion", "full_body", "cardio_machine", "low", "")
	add(69, "Outdoor Run", "cardio", "cardio", nil, 3, "locomotion", "full_body", "bodyweight", "high", "")
	add(70, "Stationary Bike", "cardio", "cardio", nil, 4, "locomotion", "full_body", "cardio_machine", "low", "")
	add(71, "Assault Bike", "cardio", "cardio", nil, 5, "locomotion", "full_body", "cardio_machine", "low", "")
	add(72, "Rowing Machine", "cardio", "cardio", nil, 6, "locomotion", "full_body", "cardio_machine", "low", "")
	add(73, "Elliptical", "cardio", "cardio", nil, 7, "locomotion", "full_body", "cardio_machine", "low", "")
	add(74, "Stairmaster", "cardio", "cardio", nil, 8, "locomotion", "full_body", "cardio_machine", "low", "")
	add(75, "Jump Rope", "cardio", "cardio", nil, 9, "locomotion", "full_body", "bodyweight", "high", "")
	add(76, "Swimming", "cardio", "cardio", nil, 10, "locomotion", "full_body", "full_gym", "low", "")
	add(77, "HIIT Circuit", "cardio", "cardio", nil, 11, "locomotion", "full_body", "bodyweight", "high", "")
	add(78, "Sled Push", "cardio", "cardio", nil, 12, "locomotion", "full_body", "full_gym", "low", "")
	add(79, "Box Jump", "full", "strength", &compound, 8, "explosive", "quads", "bodyweight", "high", "")
	add(80, "Broad Jump", "full", "strength", &compound, 9, "explosive", "glutes", "bodyweight", "high", "")
	add(81, "Medicine Ball Slam", "full", "strength", &compound, 10, "explosive", "full_body", "full_gym", "low", "")
	add(82, "Push Press", "push", "strength", &compound, 15, "explosive", "shoulders", "barbell", "low", "")
	return out
}
