package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const (
	poundInKilograms     = 0.45359237
	maxImportedWeight    = 99999.99
	maxImportNotesRunes  = 4000
	importCustomCategory = "other"
)

// WorkoutImportService previews and commits Strong or Hevy CSV imports and
// removes committed imports.
type WorkoutImportService interface {
	Preview(ctx context.Context, userID uuid.UUID, data []byte, opts model.ImportOptions) (*model.ImportPreview, error)
	Commit(ctx context.Context, userID uuid.UUID, data []byte, opts model.ImportOptions, operationKey string) (*model.WorkoutImport, model.ImportCommitOutcome, error)
	List(ctx context.Context, userID uuid.UUID) ([]model.WorkoutImport, error)
	Delete(ctx context.Context, userID, importID uuid.UUID) error
}

type workoutImportService struct {
	repo dao.WorkoutImportDAO
	now  func() time.Time
}

// NewWorkoutImportService creates a new WorkoutImportService.
func NewWorkoutImportService(repo dao.WorkoutImportDAO) WorkoutImportService {
	return &workoutImportService{repo: repo, now: time.Now}
}

func (s *workoutImportService) Preview(ctx context.Context, userID uuid.UUID, data []byte, opts model.ImportOptions) (*model.ImportPreview, error) {
	plan, err := s.plan(ctx, userID, data, opts)
	if err != nil {
		return nil, err
	}
	return plan.preview, nil
}

func (s *workoutImportService) Commit(ctx context.Context, userID uuid.UUID, data []byte, opts model.ImportOptions, operationKey string) (*model.WorkoutImport, model.ImportCommitOutcome, error) {
	if opts.Unmatched != model.ImportUnmatchedKeep && opts.Unmatched != model.ImportUnmatchedSkip {
		return nil, model.ImportCreated, &model.ValidationError{Message: "unmatched must be keep or skip", Field: "unmatched"}
	}
	if operationKey == "" || len(operationKey) > 128 {
		return nil, model.ImportCreated, &model.ValidationError{Message: "operation_key must be 1 to 128 characters", Field: "operation_key"}
	}
	sum := sha256.Sum256(data)
	// The unit is part of the request only when the file needs it.
	hash := hex.EncodeToString(sum[:]) + ":" + opts.SourceWeightUnit + ":" + opts.Unmatched
	// A retry of a committed import replays before planning, because its days
	// now exist and planning would find nothing new.
	if replay, err := s.repo.Replay(ctx, userID, operationKey, hash); err != nil || replay != nil {
		if err != nil {
			return nil, model.ImportCreated, fmt.Errorf("checking workout import replay: %w", err)
		}
		return replay, model.ImportReplayed, nil
	}
	plan, err := s.plan(ctx, userID, data, opts)
	if err != nil {
		return nil, model.ImportCreated, err
	}
	if plan.sourceUnit == "" {
		return nil, model.ImportCreated, &model.ImportFileError{Code: model.ImportCodeUnitRequired,
			Message: "the file does not state its weight unit", Details: map[string]any{"field": "source_weight_unit"}}
	}
	days := plan.keep
	totals := plan.preview.Keep
	if opts.Unmatched == model.ImportUnmatchedSkip {
		days, totals = plan.skip, plan.preview.Skip
	}
	if len(days) == 0 {
		return nil, model.ImportCreated, &model.ValidationError{Message: "the file has no new workouts to import", Field: "file"}
	}
	imp := &model.WorkoutImport{
		ID: uuid.New(), Source: plan.preview.Source, SourceWeightUnit: plan.sourceUnit,
		WeightUnit: plan.preview.WeightUnit, Unmatched: opts.Unmatched,
		FirstDate: totals.FirstDate, LastDate: totals.LastDate,
		WorkoutCount: totals.WorkoutCount, DayCount: totals.DayCount, SetCount: totals.SetCount,
	}
	stored, outcome, err := s.repo.Commit(ctx, userID, imp, days, operationKey, hash)
	if err != nil {
		return nil, outcome, fmt.Errorf("committing workout import: %w", err)
	}
	return stored, outcome, nil
}

func (s *workoutImportService) List(ctx context.Context, userID uuid.UUID) ([]model.WorkoutImport, error) {
	imports, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing workout imports: %w", err)
	}
	return imports, nil
}

func (s *workoutImportService) Delete(ctx context.Context, userID, importID uuid.UUID) error {
	if err := s.repo.Delete(ctx, userID, importID); err != nil {
		return fmt.Errorf("deleting workout import: %w", err)
	}
	return nil
}

// importPlan is a parsed file resolved against the athlete's data: the
// preview plus the days each unmatched choice would write.
type importPlan struct {
	preview    *model.ImportPreview
	sourceUnit string
	keep       []model.ImportDay
	skip       []model.ImportDay
}

type importWorkout struct {
	key   string
	name  string
	notes string
	start time.Time
	end   *time.Time
	rows  []importRow
}

func (s *workoutImportService) plan(ctx context.Context, userID uuid.UUID, data []byte, opts model.ImportOptions) (*importPlan, error) {
	if opts.SourceWeightUnit != "" && opts.SourceWeightUnit != "lb" && opts.SourceWeightUnit != "kg" {
		return nil, &model.ValidationError{Message: "source_weight_unit must be lb or kg", Field: "source_weight_unit"}
	}
	file, err := parseImportFile(data)
	if err != nil {
		return nil, err
	}
	athlete, err := s.repo.Context(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loading import context: %w", err)
	}
	location, err := time.LoadLocation(athlete.Timezone)
	if err != nil {
		location = time.UTC
	}
	today := s.now().In(location).Format(time.DateOnly)

	workouts, byDate := groupImportWorkouts(file.rows)
	if len(byDate) > maxImportDays {
		return nil, &model.ImportFileError{Code: model.ImportCodeTooLarge, Message: "the file covers too many days",
			Details: map[string]any{"max_days": maxImportDays}}
	}
	sourceUnit := file.weightUnit
	if file.rowUnits {
		sourceUnit = "row"
	} else if sourceUnit == "" {
		sourceUnit = opts.SourceWeightUnit
	}

	preview := &model.ImportPreview{
		Source: file.source, WeightUnit: athlete.WeightUnit, FileWorkouts: len(workouts),
		UnmatchedExercises: []model.UnmatchedExercise{},
	}
	// A file whose rows state their unit converts row by row; it reports
	// the first unit it states.
	reportedUnit := sourceUnit
	if file.rowUnits {
		reportedUnit = athlete.WeightUnit
		for _, row := range file.rows {
			if row.weightUnit != "" {
				reportedUnit = row.weightUnit
				break
			}
		}
	}
	if file.weightUnit != "" || file.rowUnits {
		preview.SourceWeightUnit = &reportedUnit
	}
	dates := sortedKeys(byDate)
	if len(dates) > 0 {
		preview.FileFirstDate, preview.FileLastDate = &dates[0], &dates[len(dates)-1]
	}
	existing := map[string]bool{}
	if len(dates) > 0 {
		if existing, err = s.repo.ExistingLogDates(ctx, userID, dates[0], dates[len(dates)-1]); err != nil {
			return nil, fmt.Errorf("reading existing log dates: %w", err)
		}
	}

	matcher := newExerciseMatcher(athlete.Catalog, athlete.SavedExercises)
	unmatchedSets := map[string]int{}
	unmatchedOrder := []string{}
	matched := map[string]bool{}
	cardioOnly := map[string]bool{}
	for _, w := range workouts {
		for _, row := range w.rows {
			if row.warmup || (row.reps == nil && row.seconds == nil) {
				continue
			}
			if matcher.match(row.exercise) != nil {
				matched[row.exercise] = true
				continue
			}
			if _, seen := unmatchedSets[row.exercise]; !seen {
				unmatchedOrder = append(unmatchedOrder, row.exercise)
				cardioOnly[row.exercise] = true
			}
			unmatchedSets[row.exercise]++
			if row.reps != nil {
				cardioOnly[row.exercise] = false
			}
		}
	}
	for _, name := range unmatchedOrder {
		preview.UnmatchedExercises = append(preview.UnmatchedExercises, model.UnmatchedExercise{Name: name, SetCount: unmatchedSets[name]})
	}
	preview.MatchedExercises = len(matched)

	plan := &importPlan{preview: preview, sourceUnit: reportedUnit}
	builder := dayBuilder{matcher: matcher, cardioOnly: cardioOnly, sourceUnit: sourceUnit, athleteUnit: athlete.WeightUnit,
		location: location}
	for _, date := range dates {
		switch {
		case existing[date]:
			preview.DaysAlreadyPresent++
			continue
		case date > today:
			preview.DaysSkippedFuture++
			continue
		}
		for _, skipUnmatched := range []bool{false, true} {
			day, counts, err := builder.build(date, byDate[date], skipUnmatched)
			if err != nil {
				return nil, err
			}
			totals := &preview.Keep
			if skipUnmatched {
				totals = &preview.Skip
			} else {
				preview.WarmupSetsSkipped += counts.warmups
				preview.EmptySetsSkipped += counts.empty
				preview.SetsLosingDistance += counts.distance
			}
			if day == nil {
				continue
			}
			addTotals(totals, date, counts.workouts, len(day.Sets))
			if skipUnmatched {
				plan.skip = append(plan.skip, *day)
			} else {
				plan.keep = append(plan.keep, *day)
			}
		}
	}
	return plan, nil
}

func addTotals(totals *model.ImportTotals, date string, workouts, sets int) {
	totals.DayCount++
	totals.WorkoutCount += workouts
	totals.SetCount += sets
	if totals.FirstDate == nil {
		first := date
		totals.FirstDate = &first
	}
	last := date
	totals.LastDate = &last
}

// groupImportWorkouts groups rows into workouts in file order and workouts
// into calendar dates (the date the workout started, as written).
func groupImportWorkouts(rows []importRow) ([]*importWorkout, map[string][]*importWorkout) {
	var workouts []*importWorkout
	index := map[string]*importWorkout{}
	byDate := map[string][]*importWorkout{}
	for _, row := range rows {
		w, ok := index[row.workoutKey]
		if !ok {
			w = &importWorkout{key: row.workoutKey, name: row.workoutName, notes: row.workoutNotes, start: row.start, end: row.end}
			index[row.workoutKey] = w
			workouts = append(workouts, w)
			date := row.start.Format(time.DateOnly)
			byDate[date] = append(byDate[date], w)
		}
		w.rows = append(w.rows, row)
	}
	for _, day := range byDate {
		slices.SortStableFunc(day, func(a, b *importWorkout) int { return a.start.Compare(b.start) })
	}
	return workouts, byDate
}

type dayCounts struct {
	workouts, warmups, empty, distance int
}

type dayBuilder struct {
	matcher     *exerciseMatcher
	cardioOnly  map[string]bool
	sourceUnit  string // "lb", "kg", "row", or "" when unknown
	athleteUnit string
	location    *time.Location
}

// build turns one date's workouts into a day log, or nil when no set
// survives.
func (b dayBuilder) build(date string, workouts []*importWorkout, skipUnmatched bool) (*model.ImportDay, dayCounts, error) {
	var counts dayCounts
	day := &model.ImportDay{Date: date}
	indexes := map[string]int{}
	var notes []string
	var last time.Time
	for _, w := range workouts {
		kept := 0
		for _, row := range w.rows {
			if row.warmup {
				counts.warmups++
				continue
			}
			if row.reps == nil && row.seconds == nil {
				counts.empty++
				continue
			}
			set, ok, err := b.set(row, skipUnmatched)
			if err != nil {
				return nil, counts, err
			}
			if !ok {
				continue
			}
			if row.hasDistance {
				counts.distance++
			}
			key := set.ExerciseName
			if set.ExerciseID != nil {
				key = set.ExerciseID.String()
			}
			indexes[key]++
			set.SetIndex = indexes[key]
			set.LoggedAt = wallClock(w.start, b.location).Add(time.Duration(len(day.Sets)) * time.Millisecond)
			day.Sets = append(day.Sets, set)
			kept++
		}
		if kept == 0 {
			continue
		}
		counts.workouts++
		entry := w.name
		if w.notes != "" {
			entry += "\n" + w.notes
		}
		notes = append(notes, entry)
		end := w.start
		if w.end != nil {
			end = *w.end
		}
		if end.After(last) {
			last = end
		}
	}
	if len(day.Sets) == 0 {
		return nil, counts, nil
	}
	day.LoggedAt = wallClock(last, b.location)
	if joined := truncateRunes(strings.TrimSpace(strings.Join(notes, "\n\n")), maxImportNotesRunes); joined != "" {
		day.SessionNotes = &joined
	}
	return day, counts, nil
}

func (b dayBuilder) set(row importRow, skipUnmatched bool) (model.ImportSet, bool, error) {
	set := model.ImportSet{ActualReps: row.reps, DurationSeconds: row.seconds}
	if item := b.matcher.match(row.exercise); item != nil {
		set.ExerciseName, set.ExerciseCategory, set.ExerciseModality = item.Name, item.Category, item.Modality
	} else {
		if skipUnmatched {
			return set, false, nil
		}
		set.ExerciseName, set.ExerciseCategory, set.ExerciseModality = row.exercise, importCustomCategory, "strength"
		if b.cardioOnly[row.exercise] {
			set.ExerciseModality = "cardio"
		}
	}
	set.ExerciseID = b.matcher.savedExercise(set.ExerciseName, row.exercise)
	if row.weight != nil {
		weight, err := b.convert(*row.weight, row.weightUnit)
		if err != nil {
			return set, false, invalidImportRow(row.line, fmt.Sprintf("the weight is above %.2f", maxImportedWeight))
		}
		set.ActualWeight = &weight
	}
	return set, true, nil
}

// convert expresses a source weight in the athlete's unit, to 0.01.
func (b dayBuilder) convert(weight float64, rowUnit string) (float64, error) {
	unit := b.sourceUnit
	if unit == "row" || unit == "" {
		unit = rowUnit
	}
	if unit == "" {
		// The commit refuses a file without a unit; a preview only counts.
		unit = b.athleteUnit
	}
	switch {
	case unit == "lb" && b.athleteUnit == "kg":
		weight *= poundInKilograms
	case unit == "kg" && b.athleteUnit == "lb":
		weight /= poundInKilograms
	}
	weight = math.Round(weight*100) / 100
	if weight > maxImportedWeight {
		return 0, fmt.Errorf("%w: the weight is above %.2f", errImportValue, maxImportedWeight)
	}
	return weight, nil
}

// wallClock reads a timezone-less export time in the athlete's timezone.
func wallClock(t time.Time, location *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, location)
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
