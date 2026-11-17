package model

import (
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
)

// isoWeekday returns the ISO weekday of a date, Monday=1 through Sunday=7.
func isoWeekday(date time.Time) int {
	day := int(date.Weekday())
	if day == 0 {
		return 7
	}
	return day
}

// normalizeAvailableDays returns the sorted unique ISO weekdays, or an
// available_days validation error when there is nothing to place on.
func normalizeAvailableDays(days []int) ([]int, error) {
	seen := make(map[int]bool, len(days))
	out := make([]int, 0, len(days))
	for _, day := range days {
		if day < 1 || day > 7 {
			return nil, validationError("available_days", "available_days must use ISO weekdays 1 through 7")
		}
		if !seen[day] {
			seen[day] = true
			out = append(out, day)
		}
	}
	if len(out) == 0 {
		return nil, validationError("available_days", "at least one available day is required")
	}
	sort.Ints(out)
	return out, nil
}

// PlaceProgram gives every unit of an unplaced program its weekdays. Walking
// forward from the from date, it deals units in sequence order onto the
// athlete's available days until every unit and every day has been used once:
// max(units, days) slots. The first week therefore matches the sequence
// dealing the schedule used before programs owned their weekdays, a program
// with fewer units than days repeats units inside the week, and one with more
// units than days puts several units on one day. Nothing is dropped.
func PlaceProgram(p *Program, availableDays []int, from string) error {
	start, err := ParseDate(from)
	if err != nil {
		return validationError("from", "from must be YYYY-MM-DD")
	}
	days, err := normalizeAvailableDays(availableDays)
	if err != nil {
		return err
	}
	if len(p.Workouts) == 0 {
		return nil
	}
	order := make([]int, len(p.Workouts))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(i, j int) bool {
		return p.Workouts[order[i]].SequencePosition < p.Workouts[order[j]].SequencePosition
	})
	placed := make([][]int, len(p.Workouts))
	slots := max(len(days), len(order))
	for date, slot := start, 0; slot < slots; date = date.AddDate(0, 0, 1) {
		weekday := isoWeekday(date)
		if !slices.Contains(days, weekday) {
			continue
		}
		unit := order[slot%len(order)]
		if !slices.Contains(placed[unit], weekday) {
			placed[unit] = append(placed[unit], weekday)
		}
		slot++
	}
	for index := range p.Workouts {
		sort.Ints(placed[index])
		setUnitWeekdays(&p.Workouts[index], placed[index])
	}
	return nil
}

func setUnitWeekdays(unit *ProgramWorkout, weekdays []int) {
	unit.Weekdays = WeekdayList(weekdays)
	first := weekdays[0]
	unit.PreferredWeekday = &first
}

// ValidateUnitWeekdays checks the weekdays a client supplied and sorts them.
// A unit without the field, or with an empty list, is unplaced: an unplaced
// unit reads back as [], so a client can send a program exactly as it received
// it. A program is all placed or all unplaced. A placed unit's
// preferred_weekday becomes its first weekday.
func ValidateUnitWeekdays(units []ProgramWorkout) error {
	placed := 0
	for index := range units {
		unit := &units[index]
		if len(unit.Weekdays) == 0 {
			unit.Weekdays = nil
			continue
		}
		if len(unit.Weekdays) > 7 {
			return validationError("workouts", "weekdays must hold one to seven ISO weekdays")
		}
		weekdays := append([]int(nil), unit.Weekdays...)
		sort.Ints(weekdays)
		for position, day := range weekdays {
			if day < 1 || day > 7 {
				return validationError("workouts", "weekdays must use ISO weekdays 1 through 7")
			}
			if position > 0 && weekdays[position-1] == day {
				return validationError("workouts", "weekdays must not repeat a day")
			}
		}
		setUnitWeekdays(unit, weekdays)
		placed++
	}
	if placed != 0 && placed != len(units) {
		return validationError("workouts", "weekdays must be set on every workout or on none")
	}
	return nil
}

// LayoutSessions returns one planned session for each unit on each of its
// weekdays between from and to, inclusive, ordered by date then sequence.
// Every session carries its slot date. An empty range yields no sessions.
func LayoutSessions(p *Program, from, to string) ([]ScheduledWorkout, error) {
	start, err := ParseDate(from)
	if err != nil {
		return nil, validationError("from", "from must be YYYY-MM-DD")
	}
	end, err := ParseDate(to)
	if err != nil {
		return nil, validationError("to", "to must be YYYY-MM-DD")
	}
	result := []ScheduledWorkout{}
	if len(p.Workouts) == 0 {
		return result, nil
	}
	if !p.Placed() {
		return nil, validationError("workouts", "program has no weekdays yet")
	}
	units := append([]ProgramWorkout(nil), p.Workouts...)
	sort.SliceStable(units, func(i, j int) bool { return units[i].SequencePosition < units[j].SequencePosition })
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		weekday := isoWeekday(date)
		for index := range units {
			if slices.Contains([]int(units[index].Weekdays), weekday) {
				result = append(result, sessionFromUnit(p.ID, &units[index], date.Format(time.DateOnly)))
			}
		}
	}
	return result, nil
}

// UnitSessions returns the sessions of one placed unit between from and to.
func UnitSessions(programID uuid.UUID, unit *ProgramWorkout, from, to string) ([]ScheduledWorkout, error) {
	return LayoutSessions(&Program{ID: programID, Workouts: []ProgramWorkout{*unit}}, from, to)
}

func sessionFromUnit(programID uuid.UUID, unit *ProgramWorkout, date string) ScheduledWorkout {
	unitID, sequence, slot := unit.ID, unit.SequencePosition, date
	session := ScheduledWorkout{
		Kind: NormalizeUnitKind(unit.Kind), ProgramID: &programID, ProgramWorkoutID: &unitID,
		Date: date, SlotDate: &slot, Name: unit.Name, SequencePosition: &sequence,
		Status: WorkoutStatusPlanned, RequiredSets: []ScheduledSet{}, ExtraSets: []PerformedSet{},
	}
	if unit.IsSport() && unit.Sport != nil {
		// A sport session snapshots its target and has no sets; a linked
		// sport activity completes it.
		session.Sport = &ScheduledSport{SportTarget: unit.Sport.Copy()}
	} else {
		session.RequiredSets = SnapshotScheduledSets(unit.Exercises)
	}
	return session
}

// UnitDiff says how a saved program's units differ from the stored ones.
// Removed holds stored unit IDs; the other fields hold indexes into the saved
// units.
type UnitDiff struct {
	// Removed units are no longer in the program.
	Removed []uuid.UUID
	// Added units are new: they carry no ID, or one the program does not own.
	Added []int
	// Replaced units kept their identity but changed weekdays.
	Replaced []int
	// Resnapshot units kept identity and weekdays but changed what a session
	// snapshots: name, kind, sport target, or exercises.
	Resnapshot []int
}

// DiffUnits compares stored units with the units of a save request.
func DiffUnits(before, after []ProgramWorkout) UnitDiff {
	stored := make(map[uuid.UUID]*ProgramWorkout, len(before))
	for index := range before {
		stored[before[index].ID] = &before[index]
	}
	kept := make(map[uuid.UUID]bool, len(after))
	diff := UnitDiff{Removed: []uuid.UUID{}, Added: []int{}, Replaced: []int{}, Resnapshot: []int{}}
	for index := range after {
		unit := &after[index]
		prior, ok := stored[unit.ID]
		if !ok || unit.ID == uuid.Nil || kept[unit.ID] {
			diff.Added = append(diff.Added, index)
			continue
		}
		kept[unit.ID] = true
		switch {
		case !slices.Equal([]int(prior.Weekdays), []int(unit.Weekdays)):
			diff.Replaced = append(diff.Replaced, index)
		case !sameUnitContent(prior, unit):
			diff.Resnapshot = append(diff.Resnapshot, index)
		}
	}
	for index := range before {
		if !kept[before[index].ID] {
			diff.Removed = append(diff.Removed, before[index].ID)
		}
	}
	return diff
}

// sameUnitContent reports whether two units would snapshot the same session.
// Exercise row IDs are ignored because exercises are re-inserted on each save.
func sameUnitContent(a, b *ProgramWorkout) bool {
	if a.Name != b.Name || NormalizeUnitKind(a.Kind) != NormalizeUnitKind(b.Kind) || a.SequencePosition != b.SequencePosition {
		return false
	}
	if (a.Sport == nil) != (b.Sport == nil) {
		return false
	}
	if a.Sport != nil && (a.Sport.SportID != b.Sport.SportID || a.Sport.SportName != b.Sport.SportName ||
		a.Sport.TargetDurationMinutes != b.Sport.TargetDurationMinutes ||
		!equalPtr(a.Sport.TargetDistanceMeters, b.Sport.TargetDistanceMeters) || !equalPtr(a.Sport.Notes, b.Sport.Notes)) {
		return false
	}
	if len(a.Exercises) != len(b.Exercises) {
		return false
	}
	for index := range a.Exercises {
		x, y := &a.Exercises[index], &b.Exercises[index]
		if x.Name != y.Name || x.Category != y.Category || x.Modality != y.Modality ||
			x.ExerciseOrder != y.ExerciseOrder || x.TargetSets != y.TargetSets ||
			!equalPtr(x.CatalogID, y.CatalogID) || !equalPtr(x.TargetReps, y.TargetReps) ||
			!equalPtr(x.TargetWeight, y.TargetWeight) || !equalPtr(x.TargetDurationSeconds, y.TargetDurationSeconds) ||
			!equalPtr(x.RestSeconds, y.RestSeconds) || !equalPtr(x.Notes, y.Notes) {
			return false
		}
	}
	return true
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// OffAvailabilityUnit is a placed unit with weekdays the athlete does not
// list as available. Weekdays holds only those weekdays.
type OffAvailabilityUnit struct {
	ProgramWorkoutID *uuid.UUID `json:"program_workout_id"`
	Name             string     `json:"name"`
	Weekdays         []int      `json:"weekdays"`
}

// OffAvailability lists the units of a program that sit on weekdays outside
// availableDays. It is never nil.
func OffAvailability(p *Program, availableDays []int) []OffAvailabilityUnit {
	off := []OffAvailabilityUnit{}
	for index := range p.Workouts {
		unit := &p.Workouts[index]
		outside := []int{}
		for _, day := range unit.Weekdays {
			if !slices.Contains(availableDays, day) {
				outside = append(outside, day)
			}
		}
		if len(outside) == 0 {
			continue
		}
		entry := OffAvailabilityUnit{Name: unit.Name, Weekdays: outside}
		if unit.ID != uuid.Nil {
			id := unit.ID
			entry.ProgramWorkoutID = &id
		}
		off = append(off, entry)
	}
	return off
}
