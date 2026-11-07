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
// cardio, DurationSeconds zero for strength.
type Exercise struct {
	Entry           model.CatalogExercise
	Sets            int
	Reps            int
	DurationSeconds int
	RestSeconds     int
}

// Workout is one training day.
type Workout struct {
	Name      string
	Kind      string
	Weekday   int
	Exercises []Exercise
}

// Draft is the built program before conversion to the API shape.
type Draft struct {
	Name     string
	Workouts []Workout
}

// Explanation records the choices the rationale is written from.
type Explanation struct {
	Split         string
	Days          []int
	MainLifts     []string
	SecondaryNote string
	FocusNote     string
	EquipmentNote string
	BeginnerNote  string
	Honoured      []string
	Dropped       []string
}

// Build makes a deterministic draft: equal Input (including Variant) gives an
// equal Draft.
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
	kinds := splitFor(in.PrimaryGoal, count)
	beginner := in.Experience == "beginner"
	budget := budgetFor(in.SessionMinutes)
	slots := budget.Slots
	if beginner {
		slots = min(slots, budget.BeginnerCap)
	}
	goalTargets := targets[in.PrimaryGoal][in.Experience]
	if goalTargets == (Targets{}) {
		goalTargets = targets[model.GoalGeneralHealth]["intermediate"]
	}

	b := &builder{in: in, beginner: beginner, used: map[string]bool{}}
	b.explain.Split = splitName(in.PrimaryGoal, kinds)
	b.explain.Days = days
	if beginner {
		b.explain.BeginnerNote = "As a beginner you get fewer exercises, mostly compound lifts, and nothing explosive."
	}
	if slices.Equal(in.Equipment, []string{model.EquipmentBodyweight}) {
		b.explain.EquipmentNote = "Built around bodyweight work."
	}

	draft := Draft{Name: programName(in.PrimaryGoal, count)}
	intervalDays, finisherDays := 0, 0
	for i, kind := range kinds {
		b.used = map[string]bool{}
		w := Workout{Name: workoutName(kind, i, kinds), Kind: kind, Weekday: days[i]}
		order := b.orderSlots(kind, slots)
		if kind == KindCardio {
			b.fillCardioDay(&w, order, slots, intervalDays < maxIntervalDays)
			if intervalDays < maxIntervalDays {
				intervalDays++
			}
		} else {
			circuit := kind == KindCircuit
			if in.PrimaryGoal == model.GoalPower && !beginner {
				b.fill(&w, i, slotExplosive, explosiveTargets[in.Experience], true)
			}
			if slices.Contains(in.SecondaryGoals, model.GoalPower) && !beginner && in.PrimaryGoal != model.GoalPower {
				b.fill(&w, i, slotExplosive, explosiveTargets[in.Experience], true)
			}
			for _, slot := range order {
				if len(w.Exercises) >= slots {
					break
				}
				b.fill(&w, i, slot, goalTargets, false)
			}
			if circuit && len(w.Exercises) < maxExercises {
				b.addCardio(&w, i, circuitFinisher)
			}
			// Secondary strength: the first compound gets the heavier targets.
			if slices.Contains(in.SecondaryGoals, model.GoalStrength) && in.PrimaryGoal != model.GoalStrength {
				for j := range w.Exercises {
					if w.Exercises[j].Entry.IsCompound() {
						w.Exercises[j].Sets, w.Exercises[j].Reps, w.Exercises[j].RestSeconds = secondaryStrength.MainSets, secondaryStrength.MainReps, secondaryStrength.MainRest
						break
					}
				}
			}
			if slices.Contains(in.SecondaryGoals, model.GoalHypertrophy) && in.PrimaryGoal != model.GoalHypertrophy {
				b.addIsolation(&w, i, Targets{12, 3, 60, 12, 3, 60})
			}
			finisher, intervals := b.finisherFor(kind, intervalDays < 1)
			if finisher != nil && finisherDays < maxFinisherDays {
				b.addFinisher(&w, i, *finisher)
				finisherDays++
				if intervals {
					intervalDays++
				}
			}
		}
		b.topUp(&w, i, goalTargets)
		if len(w.Exercises) < minExercises {
			return Draft{}, Explanation{}, ErrTooConstrained
		}
		draft.Workouts = append(draft.Workouts, w)
	}
	b.explain.MainLifts = mainLifts(draft)
	b.explain.SecondaryNote = secondaryNote(in)
	b.explain.FocusNote = focusNote(in.Focus)
	return draft, b.explain, nil
}

var extraFocusSlots = map[string][]Slot{
	AreaUpper: {slotIsoPush, slotIsoPull, slotRearDelt, slotTriceps, slotBiceps, slotHPush, slotHPull, slotVPull},
	AreaLower: {slotLunge, slotHamIso, slotQuadIso, slotCalf, slotSquat, slotHinge, slotLunge},
	AreaCore:  {slotCore, slotCarry, slotCore},
}

type builder struct {
	in       Input
	beginner bool
	used     map[string]bool
	explain  Explanation
}

// orderSlots applies focus (two thirds of strength slots from the focus area,
// at least one from elsewhere) and avoid (patterns removed, noted once).
func (b *builder) orderSlots(kind string, slots int) []Slot {
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
	want := (2*slots + 2) / 3 // ceil(2/3)
	if want >= slots && slots > 1 {
		want = slots - 1
	}
	// When avoided patterns thin the focus area, more slots from that area
	// (isolation and repeats, which pick different entries) keep the share.
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
	e := list[(b.in.Variant+workoutIndex)%len(list)]
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
		ex.Sets, ex.DurationSeconds, ex.RestSeconds = 1, 600, 60
	case explosive || (e.IsCompound() && e.Pattern != model.PatternCore):
		ex.Sets, ex.Reps, ex.RestSeconds = t.MainSets, t.MainReps, t.MainRest
	default:
		ex.Sets, ex.Reps, ex.RestSeconds = t.AccSets, t.AccReps, t.AccRest
	}
	w.Exercises = append(w.Exercises, ex)
}

func (b *builder) addCardio(w *Workout, workoutIndex int, dose cardioDose) bool {
	e, ok := b.pick(slotCardio, workoutIndex, true)
	if !ok {
		return false
	}
	w.Exercises = append(w.Exercises, Exercise{Entry: e, Sets: dose.Sets, DurationSeconds: dose.DurationSeconds, RestSeconds: dose.RestSeconds})
	return true
}

func (b *builder) addIsolation(w *Workout, workoutIndex int, t Targets) {
	if len(w.Exercises) >= maxExercises {
		w.Exercises = w.Exercises[:maxExercises-1]
	}
	for _, slot := range []Slot{slotIsoPush, slotIsoPull, slotQuadIso, slotHamIso} {
		if e, ok := b.pick(slot, workoutIndex, false); ok {
			w.Exercises = append(w.Exercises, Exercise{Entry: e, Sets: t.AccSets, Reps: t.AccReps, RestSeconds: t.AccRest})
			return
		}
	}
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
	w.Exercises = append(w.Exercises, Exercise{Entry: e, Sets: dose.Sets, DurationSeconds: dose.DurationSeconds, RestSeconds: dose.RestSeconds})
}

// topUp fills a thin workout from any pattern, then isolation work (even for
// beginners), then cardio, before the engine gives up on it.
func (b *builder) topUp(w *Workout, workoutIndex int, t Targets) {
	if len(w.Exercises) >= minExercises {
		return
	}
	for _, slot := range slotOrder[KindFull] {
		if len(w.Exercises) >= minExercises {
			return
		}
		if !slices.Contains(b.in.Avoid, slot.Pattern) {
			b.fill(w, workoutIndex, slot, t, false)
		}
	}
	relaxed := b.beginner
	b.beginner = false
	for _, slot := range []Slot{slotLunge, slotCarry, slotHamIso, slotQuadIso, slotCalf, slotIsoPush, slotIsoPull} {
		if len(w.Exercises) >= minExercises {
			break
		}
		if !slices.Contains(b.in.Avoid, slot.Pattern) {
			b.fill(w, workoutIndex, slot, t, false)
		}
	}
	b.beginner = relaxed
	for len(w.Exercises) < minExercises && !slices.Contains(b.in.Avoid, model.PatternLocomotion) {
		if !b.addCardio(w, workoutIndex, conditioningSecond) {
			break
		}
	}
}

// finisherFor says which cardio finisher a strength-led workout gets.
func (b *builder) finisherFor(kind string, intervalsAllowed bool) (*cardioDose, bool) {
	if kind == KindCircuit {
		return nil, false
	}
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

// fillCardioDay builds a conditioning cardio day: a long steady or interval
// bout, core, a second shorter bout, then carries and lunges as time allows.
func (b *builder) fillCardioDay(w *Workout, order []Slot, slots int, intervalsAllowed bool) {
	first := true
	for _, slot := range order {
		if len(w.Exercises) >= max(slots, minExercises) {
			break
		}
		if slot.Cardio {
			dose := conditioningSecond
			if first {
				dose = conditioningSteady
				if intervalsAllowed {
					dose = conditioningIntervals
				}
				first = false
			}
			b.addCardio(w, len(w.Exercises), dose)
			continue
		}
		b.fill(w, 0, slot, targets[model.GoalConditioning][b.in.Experience], false)
	}
}

func (b *builder) note(list *[]string, text string) {
	if !slices.Contains(*list, text) {
		*list = append(*list, text)
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
	nth := 0
	for j := 0; j <= i; j++ {
		if kinds[j] == kind {
			nth++
		}
	}
	label := map[string]string{
		KindFull: "Full body", KindUpper: "Upper", KindLower: "Lower", KindPush: "Push",
		KindPull: "Pull", KindLegs: "Legs", KindCardio: "Cardio", KindCircuit: "Circuit",
	}[kind]
	if strings.Count(strings.Join(kinds, ","), kind) <= 1 {
		return label
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
