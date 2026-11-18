package model

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func layoutUnit(name string, sequence int, weekdays ...int) ProgramWorkout {
	reps := 8
	unit := ProgramWorkout{
		ID: uuid.New(), Kind: UnitKindExercises, Name: name, SequencePosition: sequence,
		Exercises: []ProgramExercise{{Name: name + " lift", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 3, TargetReps: &reps}},
	}
	if len(weekdays) > 0 {
		unit.Weekdays = weekdays
	}
	return unit
}

func layoutSport(name string, sequence int, weekdays ...int) ProgramWorkout {
	return ProgramWorkout{
		ID: uuid.New(), Kind: UnitKindSport, Name: name, SequencePosition: sequence, Weekdays: weekdays,
		Sport: &SportTarget{SportID: "volleyball", SportName: "Volleyball", TargetDurationMinutes: 90}, Exercises: []ProgramExercise{},
	}
}

func unitWeekdays(p *Program) map[string][]int {
	got := map[string][]int{}
	for _, unit := range p.Workouts {
		got[unit.Name] = []int(unit.Weekdays)
	}
	return got
}

func TestPlaceProgram(t *testing.T) {
	tests := []struct {
		name  string
		units []string
		days  []int
		from  string
		want  map[string][]int
	}{
		// 2026-11-16 is a Monday.
		{name: "units equal days", units: []string{"A", "B", "C"}, days: []int{1, 3, 5}, from: "2026-11-16", want: map[string][]int{"A": {1}, "B": {3}, "C": {5}}},
		{name: "fewer units than days repeat inside the week", units: []string{"Push", "Pull", "Legs"}, days: []int{1, 2, 4, 5}, from: "2026-11-16", want: map[string][]int{"Push": {1, 5}, "Pull": {2}, "Legs": {4}}},
		{name: "more units than days share weekdays", units: []string{"A", "B", "C", "D", "E"}, days: []int{1, 3, 5}, from: "2026-11-16", want: map[string][]int{"A": {1}, "B": {3}, "C": {5}, "D": {1}, "E": {3}}},
		{name: "start mid-week begins on the first available day on or after from", units: []string{"A", "B", "C"}, days: []int{1, 3, 5}, from: "2026-11-19", want: map[string][]int{"A": {5}, "B": {1}, "C": {3}}},
		{name: "one day holds every unit", units: []string{"A", "B"}, days: []int{6}, from: "2026-11-16", want: map[string][]int{"A": {6}, "B": {6}}},
		{name: "unsorted duplicate days are normalized", units: []string{"A", "B"}, days: []int{5, 1, 5}, from: "2026-11-16", want: map[string][]int{"A": {1}, "B": {5}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := &Program{ID: uuid.New()}
			for index, name := range tt.units {
				program.Workouts = append(program.Workouts, layoutUnit(name, index+1))
			}
			if err := PlaceProgram(program, tt.days, tt.from); err != nil {
				t.Fatalf("PlaceProgram() error = %v", err)
			}
			if got := unitWeekdays(program); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("PlaceProgram() weekdays = %v, want %v", got, tt.want)
			}
			if !program.Placed() {
				t.Fatal("Placed() = false after PlaceProgram")
			}
			for _, unit := range program.Workouts {
				if unit.PreferredWeekday == nil || *unit.PreferredWeekday != unit.Weekdays[0] {
					t.Fatalf("unit %s preferred_weekday = %v, want first weekday %d", unit.Name, unit.PreferredWeekday, unit.Weekdays[0])
				}
			}
		})
	}
}

// The first week of a placement must equal what the former sequence dealing
// produced, so flows of a released app keep their first week.
func TestPlaceProgramMatchesFormerDealingForTheFirstCycle(t *testing.T) {
	program := &Program{ID: uuid.New(), Workouts: []ProgramWorkout{
		layoutUnit("Tendon strength A", 1), layoutSport("Volleyball", 2), layoutUnit("Tendon strength B", 3),
		layoutUnit("Tendon strength A2", 4), layoutSport("Volleyball 2", 5),
	}}
	// Tuesday 2026-11-17, available Mon Tue Wed Fri Sat: dealing from Tuesday
	// gives Tue, Wed, Fri, Sat, Mon in sequence order.
	if err := PlaceProgram(program, []int{1, 2, 3, 5, 6}, "2026-11-17"); err != nil {
		t.Fatalf("PlaceProgram() error = %v", err)
	}
	want := map[string][]int{"Tendon strength A": {2}, "Volleyball": {3}, "Tendon strength B": {5}, "Tendon strength A2": {6}, "Volleyball 2": {1}}
	if got := unitWeekdays(program); !reflect.DeepEqual(got, want) {
		t.Fatalf("weekdays = %v, want %v", got, want)
	}
}

func TestPlaceProgramRejectsMissingDays(t *testing.T) {
	program := &Program{Workouts: []ProgramWorkout{layoutUnit("A", 1)}}
	for _, days := range [][]int{nil, {}, {0}, {8}} {
		err := PlaceProgram(program, days, "2026-11-16")
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Field != "available_days" {
			t.Fatalf("PlaceProgram(days=%v) error = %v, want available_days validation error", days, err)
		}
	}
}

func TestLayoutSessionsFollowsWeekdaysForTwelveWeeks(t *testing.T) {
	program := &Program{ID: uuid.New(), Workouts: []ProgramWorkout{
		layoutUnit("Push", 1, 1), layoutUnit("Pull", 2, 3), layoutUnit("Legs", 3, 5),
		layoutSport("Bike ride", 4, 4), layoutSport("Volleyball", 5, 6),
	}}
	sessions, err := LayoutSessions(program, "2026-11-16", "2027-02-07") // 12 full weeks
	if err != nil {
		t.Fatalf("LayoutSessions() error = %v", err)
	}
	if len(sessions) != 12*5 {
		t.Fatalf("len(sessions) = %d, want %d", len(sessions), 12*5)
	}
	want := map[string]time.Weekday{"Push": time.Monday, "Pull": time.Wednesday, "Legs": time.Friday, "Bike ride": time.Thursday, "Volleyball": time.Saturday}
	for _, session := range sessions {
		date, _ := ParseDate(session.Date)
		if date.Weekday() != want[session.Name] {
			t.Fatalf("%s scheduled on %s (%s), want %s", session.Name, session.Date, date.Weekday(), want[session.Name])
		}
		if session.SlotDate == nil || *session.SlotDate != session.Date {
			t.Fatalf("%s slot date = %v, want %s", session.Name, session.SlotDate, session.Date)
		}
		if session.Status != WorkoutStatusPlanned || session.ProgramID == nil || session.ProgramWorkoutID == nil {
			t.Fatalf("%s is not a planned session of the program: %+v", session.Name, session)
		}
		if session.IsSport() && (session.Sport == nil || len(session.RequiredSets) != 0) {
			t.Fatalf("sport session %s must carry a sport target and no sets", session.Name)
		}
		if !session.IsSport() && len(session.RequiredSets) != 3 {
			t.Fatalf("workout %s has %d required sets, want 3", session.Name, len(session.RequiredSets))
		}
	}
}

func TestLayoutSessionsRepeatsAUnitAndSharesADay(t *testing.T) {
	program := &Program{ID: uuid.New(), Workouts: []ProgramWorkout{layoutUnit("Full body", 1, 1, 5), layoutSport("Volleyball", 2, 5)}}
	sessions, err := LayoutSessions(program, "2026-11-16", "2026-11-22")
	if err != nil {
		t.Fatalf("LayoutSessions() error = %v", err)
	}
	got := []string{}
	for _, session := range sessions {
		got = append(got, session.Date+" "+session.Name)
	}
	want := []string{"2026-11-16 Full body", "2026-11-20 Full body", "2026-11-20 Volleyball"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions = %v, want %v", got, want)
	}
}

func TestLayoutSessionsRejectsAnUnplacedProgram(t *testing.T) {
	program := &Program{ID: uuid.New(), Workouts: []ProgramWorkout{layoutUnit("A", 1)}}
	if _, err := LayoutSessions(program, "2026-11-16", "2026-11-22"); err == nil {
		t.Fatal("LayoutSessions(unplaced) error = nil, want error")
	}
}

func TestLayoutSessionsEmptyRange(t *testing.T) {
	program := &Program{ID: uuid.New(), Workouts: []ProgramWorkout{layoutUnit("A", 1, 1)}}
	sessions, err := LayoutSessions(program, "2026-11-17", "2026-11-16")
	if err != nil || sessions == nil || len(sessions) != 0 {
		t.Fatalf("LayoutSessions(to before from) = %v, %v; want empty slice, nil", sessions, err)
	}
}

func TestValidateUnitWeekdays(t *testing.T) {
	tests := []struct {
		name    string
		units   []ProgramWorkout
		wantErr bool
		want    [][]int
	}{
		{name: "all unplaced", units: []ProgramWorkout{layoutUnit("A", 1), layoutUnit("B", 2)}, want: [][]int{nil, nil}},
		{name: "all placed and sorted", units: []ProgramWorkout{layoutUnit("A", 1, 5, 1), layoutUnit("B", 2, 3)}, want: [][]int{{1, 5}, {3}}},
		{name: "mixed placed and unplaced", units: []ProgramWorkout{layoutUnit("A", 1, 1), layoutUnit("B", 2)}, wantErr: true},
		{name: "weekday below range", units: []ProgramWorkout{layoutUnit("A", 1, 0)}, wantErr: true},
		{name: "weekday above range", units: []ProgramWorkout{layoutUnit("A", 1, 8)}, wantErr: true},
		{name: "duplicate weekday", units: []ProgramWorkout{layoutUnit("A", 1, 2, 2)}, wantErr: true},
		{name: "an empty list is unplaced, as it reads back", units: []ProgramWorkout{{Name: "A", SequencePosition: 1, Weekdays: WeekdayList{}}}, want: [][]int{nil}},
		{name: "more than seven days", units: []ProgramWorkout{layoutUnit("A", 1, 1, 2, 3, 4, 5, 6, 7, 1)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUnitWeekdays(tt.units)
			if tt.wantErr {
				var validation *ValidationError
				if !errors.As(err, &validation) || validation.Field != "workouts" {
					t.Fatalf("ValidateUnitWeekdays() error = %v, want workouts validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateUnitWeekdays() error = %v", err)
			}
			for index, unit := range tt.units {
				if !reflect.DeepEqual([]int(unit.Weekdays), tt.want[index]) {
					t.Fatalf("unit %d weekdays = %v, want %v", index, unit.Weekdays, tt.want[index])
				}
				if len(unit.Weekdays) > 0 && (unit.PreferredWeekday == nil || *unit.PreferredWeekday != unit.Weekdays[0]) {
					t.Fatalf("unit %d preferred_weekday = %v, want %d", index, unit.PreferredWeekday, unit.Weekdays[0])
				}
			}
		})
	}
}

func TestWeekdayListIsAlwaysAJSONArray(t *testing.T) {
	body, err := json.Marshal(ProgramWorkout{Name: "A", SequencePosition: 1, Exercises: []ProgramExercise{}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if string(decoded["weekdays"]) != "[]" {
		t.Fatalf("unplaced weekdays JSON = %s, want []", decoded["weekdays"])
	}
	var absent, empty ProgramWorkout
	if err := json.Unmarshal([]byte(`{"name":"A"}`), &absent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"name":"A","weekdays":[]}`), &empty); err != nil {
		t.Fatal(err)
	}
	if absent.Weekdays != nil || empty.Weekdays == nil {
		t.Fatalf("absent weekdays = %v (want nil), empty weekdays = %v (want non-nil empty)", absent.Weekdays, empty.Weekdays)
	}
}

func TestDiffUnits(t *testing.T) {
	push, pull, legs, ride := layoutUnit("Push", 1, 1), layoutUnit("Pull", 2, 3), layoutUnit("Legs", 3, 5), layoutSport("Bike ride", 4, 4)
	before := []ProgramWorkout{push, pull, legs, ride}

	movedPush := push
	movedPush.Weekdays = WeekdayList{2}
	heavierPull := pull
	heavierPull.Exercises = append([]ProgramExercise(nil), pull.Exercises...)
	heavierPull.Exercises[0].TargetSets = 4
	// Exercise rows are re-inserted on every save, so a new exercise ID alone
	// is not a content change.
	sameLegs := legs
	sameLegs.Exercises = append([]ProgramExercise(nil), legs.Exercises...)
	sameLegs.Exercises[0].ID = uuid.New()
	volleyball := layoutSport("Volleyball", 5, 6)
	volleyball.ID = uuid.Nil
	after := []ProgramWorkout{movedPush, heavierPull, sameLegs, volleyball}

	diff := DiffUnits(before, after)
	if !reflect.DeepEqual(diff.Removed, []uuid.UUID{ride.ID}) {
		t.Fatalf("Removed = %v, want [%s]", diff.Removed, ride.ID)
	}
	if !reflect.DeepEqual(diff.Added, []int{3}) {
		t.Fatalf("Added = %v, want [3]", diff.Added)
	}
	if !reflect.DeepEqual(diff.Replaced, []int{0}) {
		t.Fatalf("Replaced = %v, want [0]", diff.Replaced)
	}
	if !reflect.DeepEqual(diff.Resnapshot, []int{1}) {
		t.Fatalf("Resnapshot = %v, want [1]", diff.Resnapshot)
	}
}

func TestDiffUnitsTreatsAnUnknownIDAsAdded(t *testing.T) {
	before := []ProgramWorkout{layoutUnit("Push", 1, 1)}
	stranger := layoutUnit("Pull", 1, 3)
	diff := DiffUnits(before, []ProgramWorkout{stranger})
	if len(diff.Removed) != 1 || !reflect.DeepEqual(diff.Added, []int{0}) {
		t.Fatalf("diff = %+v, want one removed and index 0 added", diff)
	}
}

func TestOffAvailability(t *testing.T) {
	program := &Program{Workouts: []ProgramWorkout{layoutUnit("Push", 1, 1), layoutUnit("Pull", 2, 3, 6), layoutUnit("Legs", 3, 5)}}
	off := OffAvailability(program, []int{1, 3, 5})
	if len(off) != 1 || off[0].Name != "Pull" || !reflect.DeepEqual(off[0].Weekdays, []int{6}) {
		t.Fatalf("OffAvailability() = %+v, want Pull off on [6]", off)
	}
	if got := OffAvailability(&Program{Workouts: []ProgramWorkout{layoutUnit("A", 1)}}, []int{1}); got == nil || len(got) != 0 {
		t.Fatalf("OffAvailability(unplaced) = %v, want empty non-nil slice", got)
	}
}

// A unit's stretching routines are not part of what a session snapshots, so
// changing only them leaves every dated session alone (spec 036).
func TestDiffUnitsIgnoresRoutineChoices(t *testing.T) {
	reps := 8
	unit := ProgramWorkout{
		ID: uuid.New(), Kind: UnitKindExercises, Name: "Push", SequencePosition: 1, Weekdays: WeekdayList{1},
		Exercises: []ProgramExercise{{Name: "Press", Category: "chest", Modality: "strength", ExerciseOrder: 1, TargetSets: 3, TargetReps: &reps}},
	}
	routine := "knee-kit"
	changed := unit
	changed.WarmupRoutineID = OptionalText{Present: true, Value: &routine}
	changed.CooldownRoutineID = OptionalText{Present: true}

	diff := DiffUnits([]ProgramWorkout{unit}, []ProgramWorkout{changed})
	if len(diff.Added)+len(diff.Removed)+len(diff.Replaced)+len(diff.Resnapshot) != 0 {
		t.Errorf("DiffUnits() = %+v, want no session change for a routine-only edit", diff)
	}
}
