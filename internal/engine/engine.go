package engine

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ErrTooConstrained says some workout ends with fewer than three eligible
// exercises; the service answers 422 on avoid.
var ErrTooConstrained = errors.New("the constraints leave too little to build a workout from")

// Input is everything the engine knows. Nothing else about the athlete exists
// here (spec 016 FR-011).
type Input struct {
	PrimaryGoal    string
	SecondaryGoals []string
	Experience     string
	UsualActivity  string
	Equipment      []string
	AvailableDays  []int
	SessionMinutes int
	WorkoutCap     int
	Avoid          []string
	Focus          string
	Variant        int
	Catalog        []model.CatalogExercise
}

// Exercise is one chosen catalog entry with its targets. Reps is zero for
// cardio, DurationSeconds zero for strength. Note names the rep range and
// effort (strength) or the intensity (cardio) in athlete words.
type Exercise struct {
	Entry           model.CatalogExercise
	Sets            int
	Reps            int
	DurationSeconds int
	RestSeconds     int
	Note            string
}

// Workout is one training day.
type Workout struct {
	Name      string
	Kind      string
	Weekday   int
	Heavy     bool
	Exercises []Exercise
}

// Draft is the built program before conversion to the API shape.
type Draft struct {
	Name     string
	Workouts []Workout
}

// Explanation records the choices the rationale is written from.
type Explanation struct {
	Split           string
	Days            []int
	MainLifts       []string
	SecondaryNote   string
	FocusNote       string
	EquipmentNote   string
	BeginnerNote    string
	UndulationNote  string
	ProgressionLine string
	Honoured        []string
	Dropped         []string
}

// Build makes a deterministic draft: equal Input (including Variant) gives an
// equal Draft. It runs the workout rules in order for each day, then the
// weekly rules over the whole draft (spec 016 FR-020).
func Build(in Input) (Draft, Explanation, error) {
	days := slices.Clone(in.AvailableDays)
	slices.Sort(days)
	days = slices.Compact(days)
	count := min(len(days), maxWorkouts)
	if in.WorkoutCap > 0 {
		count = min(count, in.WorkoutCap)
	}
	if count < 1 {
		return Draft{}, Explanation{}, ErrTooConstrained
	}
	b := newBuilder(in, count)
	draft := Draft{Name: programName(in.PrimaryGoal, count)}
	for i, kind := range b.kinds {
		w := Workout{Name: workoutName(kind, i, b.kinds), Kind: kind, Weekday: days[i], Heavy: b.heavyDay(i)}
		b.used = map[string]bool{}
		for _, rule := range workoutRules {
			rule.apply(b, &w, i)
		}
		if len(w.Exercises) < minExercises {
			return Draft{}, Explanation{}, ErrTooConstrained
		}
		draft.Workouts = append(draft.Workouts, w)
	}
	for _, rule := range weeklyRules {
		rule.apply(b, &draft)
	}
	b.explain.Days = days
	b.explain.MainLifts = mainLifts(draft)
	return draft, b.explain, nil
}

// A workoutRule changes one day; a weeklyRule looks at the whole draft. Each
// is named so a test can cover it alone and the order reads top to bottom.
type workoutRule struct {
	name  string
	apply func(b *builder, w *Workout, i int)
}

type weeklyRule struct {
	name  string
	apply func(b *builder, d *Draft)
}

var workoutRules = []workoutRule{
	{"cardio day", ruleCardioDay},
	{"explosive opener", ruleExplosiveOpener},
	{"focus opener", ruleFocusOpener},
	{"fill slots", ruleFillSlots},
	{"circuit finisher", ruleCircuitFinisher},
	{"secondary strength", ruleSecondaryStrength},
	{"secondary hypertrophy", ruleSecondaryHypertrophy},
	{"cardio finisher", ruleCardioFinisher},
	{"top up", ruleTopUp},
}

var weeklyRules = []weeklyRule{
	{"weekly set bands", ruleWeeklySetBands},
}

type builder struct {
	in            Input
	kinds         []string
	beginner      bool
	budget        sessionBudget
	slots         int
	goal          goalTargets
	used          map[string]bool
	intervalDays  int
	finisherDays  int
	explain       Explanation
	fullBodyCount int
}

func newBuilder(in Input, count int) *builder {
	b := &builder{in: in, kinds: splitFor(in.PrimaryGoal, count), beginner: in.Experience == "beginner", used: map[string]bool{}}
	b.budget = budgetFor(in.SessionMinutes)
	b.slots = b.budget.Slots
	if b.beginner {
		b.slots = min(b.slots, b.budget.BeginnerCap)
	}
	b.goal = targets[in.PrimaryGoal][in.Experience]
	if b.goal == (goalTargets{}) {
		b.goal = targets[model.GoalGeneralHealth]["intermediate"]
	}
	for _, kind := range b.kinds {
		if kind == KindFull {
			b.fullBodyCount++
		}
	}
	b.explain.Split = splitName(in.PrimaryGoal, b.kinds)
	if b.beginner {
		b.explain.BeginnerNote = "As a beginner you get fewer exercises, mostly compound lifts, and nothing explosive."
	}
	if slices.Equal(in.Equipment, []string{model.EquipmentBodyweight}) {
		b.explain.EquipmentNote = "Built around bodyweight work."
	}
	if b.undulates() {
		b.explain.UndulationNote = "Heavier and moderate days alternate through the week."
	}
	if in.PrimaryGoal != model.GoalConditioning {
		b.explain.ProgressionLine = progressionLine
	}
	b.explain.SecondaryNote = secondaryNote(in)
	b.explain.FocusNote = focusNote(in.Focus)
	return b
}

// undulates says whether heavy and moderate days alternate (FR-015): strength,
// hypertrophy, and power with two or more days, never for beginners.
func (b *builder) undulates() bool {
	switch b.in.PrimaryGoal {
	case model.GoalStrength, model.GoalHypertrophy, model.GoalPower:
		return len(b.kinds) >= 2 && !b.beginner
	}
	return false
}

func (b *builder) heavyDay(i int) bool { return !b.undulates() || i%2 == 0 }

func (b *builder) targetsFor(w *Workout) Targets {
	t := b.goal.Moderate
	if w.Heavy {
		t = b.goal.Heavy
	}
	if b.budget.Minimal {
		t.MainSets, t.AccSets = minimalSets, minimalSets
	}
	return t
}

// pickIndex is the position in the eligible list: the variant rotates every
// slot; full-body days alternate between two fixed sets (A/B) so Monday and
// Wednesday stay recognisable; other splits offset by day for variety.
func (b *builder) pickIndex(workoutIndex int) int {
	if b.fullBodyCount >= 2 {
		return b.in.Variant*2 + workoutIndex%2
	}
	return b.in.Variant + workoutIndex
}

// --- workout rules -----------------------------------------------------------

func ruleCardioDay(b *builder, w *Workout, i int) {
	if w.Kind != KindCardio {
		return
	}
	intervals := b.intervalDays < maxIntervalDays
	if intervals {
		b.intervalDays++
	}
	first := true
	for _, slot := range b.orderSlots(w.Kind) {
		if len(w.Exercises) >= max(b.slots, minExercises) {
			break
		}
		if slot.Cardio {
			dose := conditioningSecond
			if first {
				dose = conditioningSteady
				if intervals {
					dose = conditioningIntervals
				}
				first = false
			}
			b.addCardio(w, len(w.Exercises), dose)
			continue
		}
		b.fill(w, i, slot, b.targetsFor(w), false)
	}
}

func ruleExplosiveOpener(b *builder, w *Workout, i int) {
	if w.Kind == KindCardio || b.beginner {
		return
	}
	primaryPower := b.in.PrimaryGoal == model.GoalPower
	secondaryPower := slices.Contains(b.in.SecondaryGoals, model.GoalPower) && !primaryPower
	if primaryPower || secondaryPower {
		b.fill(w, i, slotExplosive, explosiveTargets[b.in.Experience], true)
	}
}

// ruleFocusOpener: under a hypertrophy goal with a focus, one isolation
// exercise for the focus area opens the session (FR-016; Nunes 2021).
func ruleFocusOpener(b *builder, w *Workout, i int) {
	if w.Kind == KindCardio || b.beginner || b.in.PrimaryGoal != model.GoalHypertrophy || b.in.Focus == "" {
		return
	}
	slot, ok := focusOpener[b.in.Focus]
	if !ok || slices.Contains(b.in.Avoid, slot.Pattern) {
		return
	}
	b.fill(w, i, slot, b.targetsFor(w), false)
}

func ruleFillSlots(b *builder, w *Workout, i int) {
	if w.Kind == KindCardio {
		return
	}
	for _, slot := range b.orderSlots(w.Kind) {
		if len(w.Exercises) >= b.slots {
			break
		}
		b.fill(w, i, slot, b.targetsFor(w), false)
	}
}

func ruleCircuitFinisher(b *builder, w *Workout, i int) {
	if w.Kind == KindCircuit && len(w.Exercises) < maxExercises {
		b.addCardio(w, i, circuitFinisher)
	}
}

// ruleSecondaryStrength: the first compound gets the heavier targets.
func ruleSecondaryStrength(b *builder, w *Workout, _ int) {
	if w.Kind == KindCardio || !slices.Contains(b.in.SecondaryGoals, model.GoalStrength) || b.in.PrimaryGoal == model.GoalStrength {
		return
	}
	for j := range w.Exercises {
		e := &w.Exercises[j]
		if e.Entry.IsCompound() && e.Entry.Pattern != model.PatternCore && e.Entry.Modality == "strength" {
			e.Sets, e.Reps, e.RestSeconds = secondaryStrength.MainSets, secondaryStrength.MainReps, secondaryStrength.MainRest
			e.Note = strengthNote(secondaryStrength.MainReps, secondaryStrength.MainRepsMax, secondaryStrength.RIR)
			return
		}
	}
}

func ruleSecondaryHypertrophy(b *builder, w *Workout, i int) {
	if w.Kind == KindCardio || !slices.Contains(b.in.SecondaryGoals, model.GoalHypertrophy) || b.in.PrimaryGoal == model.GoalHypertrophy {
		return
	}
	if len(w.Exercises) >= maxExercises {
		w.Exercises = w.Exercises[:maxExercises-1]
	}
	for _, slot := range []Slot{slotIsoPush, slotIsoPull, slotQuadIso, slotHamIso} {
		if e, ok := b.pick(slot, i, false); ok {
			t := secondaryHypertrophy
			w.Exercises = append(w.Exercises, Exercise{Entry: e, Sets: t.AccSets, Reps: t.AccReps, RestSeconds: t.AccRest, Note: strengthNote(t.AccReps, t.AccRepsMax, t.RIR)})
			return
		}
	}
}

func ruleCardioFinisher(b *builder, w *Workout, i int) {
	if w.Kind == KindCardio || w.Kind == KindCircuit {
		return
	}
	dose, intervals := b.finisherFor(b.intervalDays < 1)
	if dose == nil || b.finisherDays >= maxFinisherDays {
		return
	}
	b.addFinisher(w, i, *dose)
	b.finisherDays++
	if intervals {
		b.intervalDays++
	}
}

// ruleTopUp fills a thin workout from any pattern, then isolation work (even
// for beginners), then cardio, before the engine gives up on it.
func ruleTopUp(b *builder, w *Workout, i int) {
	if len(w.Exercises) >= minExercises {
		return
	}
	t := b.targetsFor(w)
	for _, slot := range slotOrder[KindFull] {
		if len(w.Exercises) >= minExercises {
			return
		}
		if !slices.Contains(b.in.Avoid, slot.Pattern) {
			b.fill(w, i, slot, t, false)
		}
	}
	relaxed := b.beginner
	b.beginner = false
	for _, slot := range []Slot{slotLunge, slotCarry, slotHamIso, slotQuadIso, slotCalf, slotIsoPush, slotIsoPull} {
		if len(w.Exercises) >= minExercises {
			break
		}
		if !slices.Contains(b.in.Avoid, slot.Pattern) {
			b.fill(w, i, slot, t, false)
		}
	}
	b.beginner = relaxed
	for len(w.Exercises) < minExercises && !slices.Contains(b.in.Avoid, model.PatternLocomotion) {
		if !b.addCardio(w, i, conditioningSecond) {
			break
		}
	}
}

// --- weekly rules ------------------------------------------------------------

// ruleWeeklySetBands keeps each muscle group's weekly sets inside the goal's
// band (FR-014). An over-served group first loses an isolation exercise, then
// a compound whose pattern repeats within its day, then a set from its last
// exercise; an under-served group gains an isolation exercise where a day has
// room. A focus area may carry half again its band, since focus means "most
// of the lifting slots".
func ruleWeeklySetBands(b *builder, d *Draft) {
	band, ok := weeklySetBands[b.in.PrimaryGoal]
	if !ok || b.in.PrimaryGoal == model.GoalConditioning {
		return
	}
	for range 64 {
		tally := weeklySets(*d)
		changed := false
		for _, group := range sortedGroups(tally) {
			if tally[group] > b.bandMax(band, group) && b.trimGroup(d, group, b.in.Focus != "" && focusAreaOf(group) == b.in.Focus) {
				changed = true
				break
			}
		}
		if changed {
			continue
		}
		for _, group := range []string{"chest", "back", "shoulders", "quads", "hamstrings", "glutes"} {
			if tally[group] > 0 && tally[group] < band.Min && b.addIsolationFor(d, group) {
				changed = true
				break
			}
		}
		if !changed {
			return
		}
	}
}

// weeklySets tallies direct sets per major muscle group. Core, whole-body
// work (carries), and explosive lifts are not banded: the bands come from
// per-muscle hypertrophy and strength research, not from core or power dosing.
func weeklySets(d Draft) map[string]int {
	out := map[string]int{}
	for _, w := range d.Workouts {
		for _, e := range w.Exercises {
			if e.Entry.Modality == "strength" && e.Entry.MuscleGroup != "core" && e.Entry.MuscleGroup != "full_body" && e.Entry.Pattern != model.PatternExplosive {
				out[e.Entry.MuscleGroup] += e.Sets
			}
		}
	}
	return out
}

func sortedGroups(tally map[string]int) []string {
	groups := make([]string, 0, len(tally))
	for g := range tally {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	return groups
}

// bandMax is the weekly ceiling for a group: the goal's band, raised by half
// for the focus area.
func (b *builder) bandMax(band setBand, group string) int {
	if b.in.Focus != "" && focusAreaOf(group) == b.in.Focus {
		return band.Max + band.Max/2
	}
	return band.Max
}

// trimGroup lowers a group's weekly sets by one step. Outside the focus area
// it removes work first (isolation, then a repeated pattern) and lowers sets
// last; inside the focus area it lowers sets first so the focus share holds.
func (b *builder) trimGroup(d *Draft, group string, preferSets bool) bool {
	if preferSets && b.lowerSets(d, group) {
		return true
	}
	// 1. An isolation exercise for the group, latest day first.
	for i := len(d.Workouts) - 1; i >= 0; i-- {
		w := &d.Workouts[i]
		for j := len(w.Exercises) - 1; j >= 0; j-- {
			e := w.Exercises[j]
			if e.Entry.MuscleGroup == group && e.Entry.Pattern == model.PatternIsolation && len(w.Exercises) > minExercises {
				w.Exercises = append(w.Exercises[:j], w.Exercises[j+1:]...)
				return true
			}
		}
	}
	// 2. A compound whose pattern already appears earlier in the same day.
	for i := len(d.Workouts) - 1; i >= 0; i-- {
		w := &d.Workouts[i]
		for j := len(w.Exercises) - 1; j > 0; j-- {
			e := w.Exercises[j]
			if e.Entry.MuscleGroup != group || e.Entry.Modality != "strength" || len(w.Exercises) <= minExercises {
				continue
			}
			for k := range j {
				if w.Exercises[k].Entry.Pattern == e.Entry.Pattern {
					w.Exercises = append(w.Exercises[:j], w.Exercises[j+1:]...)
					return true
				}
			}
		}
	}
	// 3. One set less on the group's last exercise, never below two.
	if b.lowerSets(d, group) {
		return true
	}
	// 4. A second exercise for the group within one day, latest day first.
	for i := len(d.Workouts) - 1; i >= 0; i-- {
		w := &d.Workouts[i]
		if len(w.Exercises) <= minExercises {
			continue
		}
		for j := len(w.Exercises) - 1; j > 0; j-- {
			if w.Exercises[j].Entry.MuscleGroup != group || w.Exercises[j].Entry.Modality != "strength" {
				continue
			}
			for k := range j {
				if w.Exercises[k].Entry.MuscleGroup == group {
					w.Exercises = append(w.Exercises[:j], w.Exercises[j+1:]...)
					return true
				}
			}
		}
	}
	return false
}

func (b *builder) lowerSets(d *Draft, group string) bool {
	for i := len(d.Workouts) - 1; i >= 0; i-- {
		w := &d.Workouts[i]
		for j := len(w.Exercises) - 1; j >= 0; j-- {
			e := &w.Exercises[j]
			if e.Entry.MuscleGroup == group && e.Entry.Modality == "strength" && e.Sets > 2 {
				e.Sets--
				return true
			}
		}
	}
	return false
}

func (b *builder) addIsolationFor(d *Draft, group string) bool {
	if b.beginner || b.budget.Minimal {
		return false
	}
	for i := range d.Workouts {
		w := &d.Workouts[i]
		if w.Kind == KindCardio || len(w.Exercises) >= b.slots || len(w.Exercises) >= maxExercises {
			continue
		}
		b.used = map[string]bool{}
		for _, e := range w.Exercises {
			b.used[e.Entry.ID.String()] = true
		}
		slot := Slot{Name: group + " isolation", Pattern: model.PatternIsolation, Area: focusAreaOf(group), Match: isolationOf(group)}
		if slices.Contains(b.in.Avoid, slot.Pattern) {
			return false
		}
		if e, ok := b.pick(slot, i, false); ok {
			t := b.targetsFor(w)
			w.Exercises = append(w.Exercises, Exercise{Entry: e, Sets: t.AccSets, Reps: t.AccReps, RestSeconds: t.AccRest, Note: strengthNote(t.AccReps, t.AccRepsMax, t.RIR)})
			return true
		}
	}
	return false
}

// --- helpers -----------------------------------------------------------------

// orderSlots applies focus (two thirds of strength slots from the focus area,
// at least one from elsewhere) and avoid (patterns removed, noted once).
func (b *builder) orderSlots(kind string) []Slot {
	base := slotOrder[kind]
	kept := make([]Slot, 0, len(base))
	for _, slot := range base {
		if slices.Contains(b.in.Avoid, slot.Pattern) {
			b.note(&b.explain.Honoured, patternWords(slot.Pattern))
			continue
		}
		kept = append(kept, slot)
	}
	if b.in.Focus == "" || kind == KindCardio {
		return kept
	}
	var focus, rest []Slot
	for _, slot := range kept {
		if slot.Area == b.in.Focus {
			focus = append(focus, slot)
		} else {
			rest = append(rest, slot)
		}
	}
	want := (2*b.slots + 2) / 3 // ceil(2/3)
	if want >= b.slots && b.slots > 1 {
		want = b.slots - 1
	}
	for _, extra := range extraFocusSlots[b.in.Focus] {
		if len(focus) >= want {
			break
		}
		if !slices.Contains(b.in.Avoid, extra.Pattern) {
			focus = append(focus, extra)
		}
	}
	out := make([]Slot, 0, len(kept)+1)
	out = append(out, focus[:min(want, len(focus))]...)
	out = append(out, rest...)
	if len(focus) > want {
		out = append(out, focus[want:]...)
	}
	return out
}

func (b *builder) candidates(slot Slot, allowCardio bool) []model.CatalogExercise {
	var out []model.CatalogExercise
	for _, e := range b.in.Catalog {
		if !slot.Match(e) || !e.EligibleFor(b.in.Equipment) || b.used[e.ID.String()] {
			continue
		}
		if slices.Contains(b.in.Avoid, e.Pattern) {
			continue
		}
		if e.Modality == "cardio" && !allowCardio && !slot.Cardio {
			continue
		}
		if b.beginner {
			if e.Pattern == model.PatternExplosive {
				continue
			}
			if e.Modality == "strength" && !e.IsCompound() && e.Pattern != model.PatternCore {
				continue
			}
			if e.Impact == model.ImpactHigh && b.in.UsualActivity == "sedentary" {
				continue
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (b *builder) pick(slot Slot, workoutIndex int, allowCardio bool) (model.CatalogExercise, bool) {
	list := b.candidates(slot, allowCardio)
	if len(list) == 0 {
		b.note(&b.explain.Dropped, slot.Name)
		return model.CatalogExercise{}, false
	}
	e := list[b.pickIndex(workoutIndex)%len(list)]
	b.used[e.ID.String()] = true
	return e, true
}

// fill adds one exercise for the slot with the goal targets: main targets for
// compounds and explosive work, accessory targets for isolation and core.
func (b *builder) fill(w *Workout, workoutIndex int, slot Slot, t Targets, explosive bool) {
	e, ok := b.pick(slot, workoutIndex, false)
	if !ok {
		return
	}
	ex := Exercise{Entry: e}
	switch {
	case e.Modality == "cardio":
		ex.Sets, ex.DurationSeconds, ex.RestSeconds, ex.Note = 1, 600, 60, noteModerate
	case explosive || (e.IsCompound() && e.Pattern != model.PatternCore):
		ex.Sets, ex.Reps, ex.RestSeconds = t.MainSets, t.MainReps, t.MainRest
		ex.Note = strengthNote(t.MainReps, t.MainRepsMax, t.RIR)
		if explosive {
			ex.Note = fmt.Sprintf("%d fast reps, stop while every rep is still sharp.", t.MainReps)
		}
	default:
		ex.Sets, ex.Reps, ex.RestSeconds = t.AccSets, t.AccReps, t.AccRest
		ex.Note = strengthNote(t.AccReps, t.AccRepsMax, t.RIR)
	}
	if b.in.PrimaryGoal == model.GoalConditioning && e.Modality == "strength" {
		ex.Note = fmt.Sprintf("%d to %d reps, keep moving between sets.", t.MainReps, t.MainRepsMax)
	}
	w.Exercises = append(w.Exercises, ex)
}

func (b *builder) addCardio(w *Workout, workoutIndex int, dose cardioDose) bool {
	e, ok := b.pick(slotCardio, workoutIndex, true)
	if !ok {
		return false
	}
	w.Exercises = append(w.Exercises, Exercise{Entry: e, Sets: dose.Sets, DurationSeconds: dose.DurationSeconds, RestSeconds: dose.RestSeconds, Note: dose.Note})
	return true
}

// addFinisher appends a cardio dose; in short sessions it replaces the last
// strength slot instead of extending the session.
func (b *builder) addFinisher(w *Workout, workoutIndex int, dose cardioDose) {
	e, ok := b.pick(slotCardio, workoutIndex, true)
	if !ok {
		b.note(&b.explain.Dropped, "cardio finisher")
		return
	}
	if (b.in.SessionMinutes < finisherCostsSlotBelow && len(w.Exercises) >= minExercises) || len(w.Exercises) >= maxExercises {
		w.Exercises = w.Exercises[:len(w.Exercises)-1]
	}
	w.Exercises = append(w.Exercises, Exercise{Entry: e, Sets: dose.Sets, DurationSeconds: dose.DurationSeconds, RestSeconds: dose.RestSeconds, Note: dose.Note})
}

// finisherFor says which cardio finisher a strength-led workout gets.
func (b *builder) finisherFor(intervalsAllowed bool) (*cardioDose, bool) {
	switch b.in.PrimaryGoal {
	case model.GoalGeneralHealth:
		return &healthFinisher, false
	case model.GoalBodyComposition:
		if intervalsAllowed {
			return &bodyCompIntervals, true
		}
		return &healthFinisher, false
	}
	for _, goal := range b.in.SecondaryGoals {
		switch goal {
		case model.GoalConditioning, model.GoalGeneralHealth, model.GoalBodyComposition:
			return &secondaryFinisher, false
		}
	}
	return nil, false
}

func (b *builder) note(list *[]string, text string) {
	if !slices.Contains(*list, text) {
		*list = append(*list, text)
	}
}

// strengthNote names the rep range and the effort cue (FR-013).
func strengthNote(reps, repsMax, rir int) string {
	rng := fmt.Sprintf("%d reps", reps)
	if repsMax > reps {
		rng = fmt.Sprintf("%d to %d reps", reps, repsMax)
	}
	switch rir {
	case 1:
		return rng + ", leave 1 to 2 in reserve."
	case 2:
		return rng + ", leave about 2 in reserve."
	default:
		return rng + ", stop with 2 to 3 in reserve."
	}
}

func splitName(goal string, kinds []string) string {
	switch {
	case goal == model.GoalConditioning:
		return "cardio and circuit days"
	case slices.Contains(kinds, KindPush):
		return "push, pull, and legs days"
	case slices.Contains(kinds, KindUpper):
		return "upper and lower days"
	default:
		return "full-body days"
	}
}

func programName(goal string, count int) string {
	return fmt.Sprintf("%d-day %s plan", count, strings.ReplaceAll(goal, "_", " "))
}

func workoutName(kind string, i int, kinds []string) string {
	label := map[string]string{
		KindFull: "Full body", KindUpper: "Upper", KindLower: "Lower", KindPush: "Push",
		KindPull: "Pull", KindLegs: "Legs", KindCardio: "Cardio", KindCircuit: "Circuit",
	}[kind]
	total, nth := 0, 0
	for j, k := range kinds {
		if k == kind {
			total++
			if j <= i {
				nth++
			}
		}
	}
	if total <= 1 {
		return label
	}
	if kind == KindFull {
		// Full-body days alternate A and B (FR-015).
		return fmt.Sprintf("%s %s", label, string(rune('A'+(nth-1)%2)))
	}
	return fmt.Sprintf("%s %d", label, nth)
}

func mainLifts(d Draft) []string {
	var out []string
	for _, w := range d.Workouts {
		for _, e := range w.Exercises {
			if e.Entry.IsCompound() && e.Entry.Pattern != model.PatternCore && !slices.Contains(out, e.Entry.Name) {
				out = append(out, e.Entry.Name)
				break
			}
		}
		if len(out) == 3 {
			break
		}
	}
	return out
}

func secondaryNote(in Input) string {
	if len(in.SecondaryGoals) == 0 {
		return ""
	}
	words := make([]string, 0, len(in.SecondaryGoals))
	for _, g := range in.SecondaryGoals {
		words = append(words, strings.ReplaceAll(g, "_", " "))
	}
	return capitalize(strings.Join(words, " and ")) + " gets a smaller share so your main goal keeps the structure."
}

func focusNote(focus string) string {
	switch focus {
	case AreaUpper:
		return "Upper body takes most of the lifting slots, as you asked."
	case AreaLower:
		return "Lower body takes most of the lifting slots, as you asked."
	case AreaCore:
		return "Core work leads each session, as you asked."
	default:
		return ""
	}
}

func patternWords(p string) string {
	return map[string]string{
		model.PatternSquat: "deep squatting", model.PatternHinge: "hinges", model.PatternLunge: "lunges",
		model.PatternHorizontalPush: "pressing", model.PatternVerticalPush: "overhead pressing",
		model.PatternHorizontalPull: "rowing", model.PatternVerticalPull: "pull-downs and pull-ups",
		model.PatternCarry: "carries", model.PatternCore: "direct core work", model.PatternExplosive: "explosive lifts",
		model.PatternLocomotion: "running and jumping", model.PatternIsolation: "isolation work",
	}[p]
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
