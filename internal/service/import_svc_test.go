package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type fakeImportDAO struct {
	context   model.ImportContext
	existing  map[string]bool
	committed []model.ImportDay
	imp       *model.WorkoutImport
	hash      string
	outcome   model.ImportCommitOutcome
	err       error
	deleted   uuid.UUID
	replay    *model.WorkoutImport
	replayErr error
}

func (f *fakeImportDAO) Context(context.Context, uuid.UUID) (*model.ImportContext, error) {
	c := f.context
	return &c, f.err
}

func (f *fakeImportDAO) ExistingLogDates(context.Context, uuid.UUID, string, string) (map[string]bool, error) {
	if f.existing == nil {
		return map[string]bool{}, f.err
	}
	return f.existing, f.err
}

func (f *fakeImportDAO) Commit(_ context.Context, _ uuid.UUID, imp *model.WorkoutImport, days []model.ImportDay, _, hash string) (*model.WorkoutImport, model.ImportCommitOutcome, error) {
	f.imp, f.committed, f.hash = imp, days, hash
	return imp, f.outcome, f.err
}

func (f *fakeImportDAO) Replay(context.Context, uuid.UUID, string, string) (*model.WorkoutImport, error) {
	return f.replay, f.replayErr
}

func (f *fakeImportDAO) List(context.Context, uuid.UUID) ([]model.WorkoutImport, error) {
	return []model.WorkoutImport{}, f.err
}

func (f *fakeImportDAO) Delete(_ context.Context, _, id uuid.UUID) error {
	f.deleted = id
	return f.err
}

var testCatalog = []model.CatalogExercise{
	{Name: "Back Squat", Category: "legs", Modality: "strength"},
	{Name: "Barbell Bench Press", Category: "push", Modality: "strength"},
	{Name: "Plank", Category: "core", Modality: "strength"},
	{Name: "Overhead Press", Category: "push", Modality: "strength"},
	{Name: "Outdoor Run", Category: "cardio", Modality: "cardio"},
	{Name: "Deadlift", Category: "pull", Modality: "strength"},
	{Name: "Stationary Bike", Category: "cardio", Modality: "cardio"},
	{Name: "Lat Pulldown", Category: "pull", Modality: "strength"},
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "imports", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newTestImportService(unit string) (*workoutImportService, *fakeImportDAO) {
	repo := &fakeImportDAO{context: model.ImportContext{WeightUnit: unit, Timezone: "America/New_York",
		SavedExercises: map[uuid.UUID]string{}, Catalog: testCatalog}}
	svc := &workoutImportService{repo: repo, now: func() time.Time { return time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC) }}
	return svc, repo
}

func TestPreviewStrongSecondsLayout(t *testing.T) {
	svc, _ := newTestImportService("kg")
	preview, err := svc.Preview(context.Background(), uuid.New(), fixture(t, "strong_seconds.csv"), model.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Source != model.ImportSourceStrong || preview.SourceWeightUnit == nil || *preview.SourceWeightUnit != "kg" {
		t.Fatalf("source = %s %v", preview.Source, preview.SourceWeightUnit)
	}
	if got := preview.Keep; got.DayCount != 2 || got.WorkoutCount != 2 || got.SetCount != 7 || *got.FirstDate != "2025-03-07" || *got.LastDate != "2025-03-24" {
		t.Fatalf("keep = %+v", got)
	}
	if got := preview.Skip; got.DayCount != 2 || got.SetCount != 6 {
		t.Fatalf("skip = %+v", got)
	}
	if preview.WarmupSetsSkipped != 1 || preview.EmptySetsSkipped != 1 || preview.MatchedExercises != 3 {
		t.Fatalf("counts = %+v", preview)
	}
	if len(preview.UnmatchedExercises) != 1 || preview.UnmatchedExercises[0] != (model.UnmatchedExercise{Name: "Cable Kickback", SetCount: 1}) {
		t.Fatalf("unmatched = %+v", preview.UnmatchedExercises)
	}
}

func TestCommitStrongSecondsWritesDays(t *testing.T) {
	svc, repo := newTestImportService("kg")
	saved := uuid.New()
	repo.context.SavedExercises[saved] = "Barbell Bench Press"
	imp, outcome, err := svc.Commit(context.Background(), uuid.New(), fixture(t, "strong_seconds.csv"),
		model.ImportOptions{Unmatched: model.ImportUnmatchedKeep}, "import-1")
	if err != nil || outcome != model.ImportCreated {
		t.Fatal(err, outcome)
	}
	if imp.DayCount != 2 || imp.SetCount != 7 || imp.SourceWeightUnit != "kg" || imp.Unmatched != "keep" {
		t.Fatalf("import = %+v", imp)
	}
	legs := repo.committed[0]
	if legs.Date != "2025-03-07" || len(legs.Sets) != 3 {
		t.Fatalf("legs day = %+v", legs)
	}
	for i, set := range legs.Sets {
		if set.ExerciseName != "Back Squat" || set.ExerciseCategory != "legs" || set.SetIndex != i+1 || set.ExerciseID != nil {
			t.Fatalf("legs set %d = %+v", i, set)
		}
	}
	if *legs.Sets[2].ActualWeight != 80 || *legs.Sets[2].ActualReps != 8 {
		t.Fatalf("drop set = %+v", legs.Sets[2])
	}
	wantEnd := time.Date(2025, 3, 7, 12, 32, 12, 0, mustLocation(t, "America/New_York")).Add(753 * time.Second)
	if !legs.LoggedAt.Equal(wantEnd) {
		t.Fatalf("logged_at = %v, want %v", legs.LoggedAt, wantEnd)
	}
	chest := repo.committed[1]
	if chest.SessionNotes == nil || *chest.SessionNotes != "Chest and Triceps\nFelt good" {
		t.Fatalf("notes = %v", chest.SessionNotes)
	}
	var bench, custom int
	for _, set := range chest.Sets {
		switch set.ExerciseName {
		case "Barbell Bench Press":
			bench++
			if set.ExerciseID == nil || *set.ExerciseID != saved {
				t.Fatalf("bench not linked to saved exercise: %+v", set)
			}
		case "Cable Kickback":
			custom++
			if set.ExerciseCategory != "other" || set.ExerciseModality != "strength" {
				t.Fatalf("custom set = %+v", set)
			}
		case "Plank":
			if set.DurationSeconds == nil || *set.DurationSeconds != 60 {
				t.Fatalf("plank = %+v", set)
			}
		}
	}
	if bench != 2 || custom != 1 {
		t.Fatalf("bench %d custom %d", bench, custom)
	}
	if *chest.Sets[1].ActualWeight != 80.5 {
		t.Fatalf("decimal comma weight = %v", *chest.Sets[1].ActualWeight)
	}
	if !strings.HasSuffix(repo.hash, "::keep") {
		t.Fatalf("hash = %s", repo.hash)
	}
}

func TestCommitSkipUnmatchedDropsCustomSets(t *testing.T) {
	svc, repo := newTestImportService("kg")
	if _, _, err := svc.Commit(context.Background(), uuid.New(), fixture(t, "strong_seconds.csv"),
		model.ImportOptions{Unmatched: model.ImportUnmatchedSkip}, "import-1"); err != nil {
		t.Fatal(err)
	}
	for _, day := range repo.committed {
		for _, set := range day.Sets {
			if set.ExerciseName == "Cable Kickback" {
				t.Fatal("unmatched set imported with skip")
			}
		}
	}
	if repo.imp.SetCount != 6 {
		t.Fatalf("set count = %d", repo.imp.SetCount)
	}
}

func TestLegacyStrongNeedsUnitAndConverts(t *testing.T) {
	svc, repo := newTestImportService("kg")
	preview, err := svc.Preview(context.Background(), uuid.New(), fixture(t, "strong_legacy.csv"), model.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceWeightUnit != nil || preview.Keep.DayCount != 1 || preview.Keep.WorkoutCount != 2 || preview.SetsLosingDistance != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	_, _, err = svc.Commit(context.Background(), uuid.New(), fixture(t, "strong_legacy.csv"), model.ImportOptions{Unmatched: "keep"}, "k")
	var fileErr *model.ImportFileError
	if !errors.As(err, &fileErr) || fileErr.Code != model.ImportCodeUnitRequired {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := svc.Commit(context.Background(), uuid.New(), fixture(t, "strong_legacy.csv"),
		model.ImportOptions{Unmatched: "keep", SourceWeightUnit: "lb"}, "k"); err != nil {
		t.Fatal(err)
	}
	day := repo.committed[0]
	if len(day.Sets) != 3 || *day.Sets[0].ActualWeight != 43.09 || day.Sets[0].ExerciseName != "Overhead Press" {
		t.Fatalf("sets = %+v", day.Sets)
	}
	run := day.Sets[2]
	if run.ExerciseName != "Outdoor Run" || run.ExerciseModality != "cardio" || run.ActualReps != nil || run.ActualWeight != nil || *run.DurationSeconds != 2100 {
		t.Fatalf("run = %+v", run)
	}
	if *day.SessionNotes != "Push\n\nEvening run" {
		t.Fatalf("notes = %q", *day.SessionNotes)
	}
}

func TestStrongUnitsLayoutReadsRowUnits(t *testing.T) {
	svc, repo := newTestImportService("kg")
	if _, _, err := svc.Commit(context.Background(), uuid.New(), fixture(t, "strong_units.csv"), model.ImportOptions{Unmatched: "keep"}, "k"); err != nil {
		t.Fatal(err)
	}
	if repo.imp.SourceWeightUnit != "lb" {
		t.Fatalf("source unit = %s", repo.imp.SourceWeightUnit)
	}
	sets := repo.committed[0].Sets
	if sets[0].ExerciseName != "Deadlift" || *sets[0].ActualWeight != 102.06 || sets[1].ExerciseName != "Stationary Bike" {
		t.Fatalf("sets = %+v", sets)
	}
	end := time.Date(2022, 1, 10, 7, 45, 0, 0, mustLocation(t, "America/New_York"))
	if !repo.committed[0].LoggedAt.Equal(end) {
		t.Fatalf("logged_at = %v", repo.committed[0].LoggedAt)
	}
}

func TestHevyLayouts(t *testing.T) {
	svc, repo := newTestImportService("kg")
	preview, err := svc.Preview(context.Background(), uuid.New(), fixture(t, "hevy_lbs.csv"), model.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Source != model.ImportSourceHevy || *preview.SourceWeightUnit != "lb" || preview.WarmupSetsSkipped != 1 ||
		preview.Keep.DayCount != 2 || preview.Keep.SetCount != 4 || len(preview.UnmatchedExercises) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	if _, _, err := svc.Commit(context.Background(), uuid.New(), fixture(t, "hevy_lbs.csv"), model.ImportOptions{Unmatched: "keep"}, "k"); err != nil {
		t.Fatal(err)
	}
	if got := *repo.committed[0].Sets[0].ActualWeight; got != 61.23 {
		t.Fatalf("135 lb = %v kg", got)
	}
	if repo.committed[0].Sets[2].ExerciseName != "Lat Pulldown" {
		t.Fatalf("dropset = %+v", repo.committed[0].Sets[2])
	}
	if *repo.committed[0].SessionNotes != "Chest/Back\nbarbells" {
		t.Fatalf("notes = %q", *repo.committed[0].SessionNotes)
	}

	preview, err = svc.Preview(context.Background(), uuid.New(), fixture(t, "hevy_kg_crlf.csv"), model.ImportOptions{})
	if err != nil || *preview.SourceWeightUnit != "kg" || preview.Keep.SetCount != 1 {
		t.Fatalf("crlf preview = %+v, %v", preview, err)
	}
}

func TestPreviewSkipsExistingAndFutureDays(t *testing.T) {
	svc, repo := newTestImportService("kg")
	repo.existing = map[string]bool{"2025-03-07": true}
	svc.now = func() time.Time { return time.Date(2025, 3, 20, 12, 0, 0, 0, time.UTC) }
	preview, err := svc.Preview(context.Background(), uuid.New(), fixture(t, "strong_seconds.csv"), model.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.DaysAlreadyPresent != 1 || preview.DaysSkippedFuture != 1 || preview.Keep.DayCount != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	_, _, err = svc.Commit(context.Background(), uuid.New(), fixture(t, "strong_seconds.csv"), model.ImportOptions{Unmatched: "keep"}, "k")
	var validation *model.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("err = %v", err)
	}
}

func TestImportRejectsBadFiles(t *testing.T) {
	svc, _ := newTestImportService("lb")
	cases := map[string]struct {
		data string
		code string
	}{
		"unknown header": {"Date,Exercise,Weight\n2025-01-01,Squat,100\n", model.ImportCodeUnrecognized},
		"empty":          {"", model.ImportCodeUnrecognized},
		"bad date":       {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE\nyesterday,A,1m,Squat,1,1,1,,,,,\n", model.ImportCodeInvalidRow},
		"bad set order":  {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE\n2025-01-01 10:00:00,A,1m,Squat,X,1,1,,,,,\n", model.ImportCodeInvalidRow},
		"bad weight":     {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE\n2025-01-01 10:00:00,A,1m,Squat,1,heavy,1,,,,,\n", model.ImportCodeInvalidRow},
		"short row":      {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE\n2025-01-01 10:00:00,A\n", model.ImportCodeInvalidRow},
		"huge weight":    {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE\n2025-01-01 10:00:00,A,1m,Squat,1,100000,1,,,,,\n", model.ImportCodeInvalidRow},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Preview(context.Background(), uuid.New(), []byte(tc.data), model.ImportOptions{SourceWeightUnit: "lb"})
			var fileErr *model.ImportFileError
			if !errors.As(err, &fileErr) || fileErr.Code != tc.code {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
		})
	}
	invalid := []byte("Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE\n2025-01-01 10:00:00,A,1m,Squat,X,1,1,,,,,\n")
	_, err := svc.Preview(context.Background(), uuid.New(), invalid, model.ImportOptions{})
	var fileErr *model.ImportFileError
	if !errors.As(err, &fileErr) || fileErr.Details["line"] != 2 {
		t.Fatalf("line detail = %v", err)
	}
}

func TestImportRowAndDayLimits(t *testing.T) {
	svc, _ := newTestImportService("lb")
	header := "Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE\n"
	var b strings.Builder
	b.WriteString(header)
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i <= maxImportDays; i++ {
		b.WriteString(start.AddDate(0, 0, i).Format("2006-01-02") + " 10:00:00,A,1m,Squat,1,1,1,,,,,\n")
	}
	_, err := svc.Preview(context.Background(), uuid.New(), []byte(b.String()), model.ImportOptions{SourceWeightUnit: "lb"})
	var fileErr *model.ImportFileError
	if !errors.As(err, &fileErr) || fileErr.Code != model.ImportCodeTooLarge || fileErr.Details["max_days"] != maxImportDays {
		t.Fatalf("days err = %v", err)
	}
	b.Reset()
	b.WriteString(header)
	row := "2025-01-01 10:00:00,A,1m,Squat,1,1,1,,,,,\n"
	b.WriteString(strings.Repeat(row, maxImportRows+1))
	_, err = svc.Preview(context.Background(), uuid.New(), []byte(b.String()), model.ImportOptions{SourceWeightUnit: "lb"})
	if !errors.As(err, &fileErr) || fileErr.Details["max_rows"] != maxImportRows {
		t.Fatalf("rows err = %v", err)
	}
}

func TestCommitValidatesOptionsAndBOM(t *testing.T) {
	svc, repo := newTestImportService("kg")
	data := append([]byte("\xef\xbb\xbf"), fixture(t, "hevy_kg_crlf.csv")...)
	if _, _, err := svc.Commit(context.Background(), uuid.New(), data, model.ImportOptions{Unmatched: "maybe"}, "k"); err == nil {
		t.Fatal("unmatched accepted")
	}
	if _, _, err := svc.Commit(context.Background(), uuid.New(), data, model.ImportOptions{Unmatched: "keep"}, ""); err == nil {
		t.Fatal("empty operation key accepted")
	}
	if _, err := svc.Preview(context.Background(), uuid.New(), data, model.ImportOptions{SourceWeightUnit: "st"}); err == nil {
		t.Fatal("bad unit accepted")
	}
	repo.replay = &model.WorkoutImport{ID: uuid.New()}
	repo.existing = map[string]bool{"2025-12-05": true}
	if imp, outcome, err := svc.Commit(context.Background(), uuid.New(), data, model.ImportOptions{Unmatched: "keep"}, "k"); err != nil || outcome != model.ImportReplayed || imp != repo.replay {
		t.Fatal(err, outcome)
	}
	repo.replay, repo.replayErr = nil, errors.New("db down")
	if _, _, err := svc.Commit(context.Background(), uuid.New(), data, model.ImportOptions{Unmatched: "keep"}, "k"); err == nil {
		t.Fatal("replay error swallowed")
	}
	repo.replayErr = nil
	repo.err = errors.New("db down")
	if _, _, err := svc.Commit(context.Background(), uuid.New(), data, model.ImportOptions{Unmatched: "keep"}, "k"); err == nil {
		t.Fatal("dao error swallowed")
	}
	if _, err := svc.List(context.Background(), uuid.New()); err == nil {
		t.Fatal("list error swallowed")
	}
	id := uuid.New()
	if err := svc.Delete(context.Background(), uuid.New(), id); err == nil || repo.deleted != id {
		t.Fatal("delete error swallowed")
	}
}

func TestExerciseMatcher(t *testing.T) {
	saved := map[uuid.UUID]string{uuid.New(): "Pull-Up", uuid.New(): "Curl", uuid.New(): "curl"}
	m := newExerciseMatcher([]model.CatalogExercise{{Name: "Pull-Up"}, {Name: "Barbell Bench Press"}, {Name: "Back Squat"}}, saved)
	for source, want := range map[string]string{
		"Pull Up":               "Pull-Up",
		"pull-up":               "Pull-Up",
		"Pull Up (Assisted)":    "Pull-Up",
		"Bench Press (Barbell)": "Barbell Bench Press",
		"Squat (Barbell)":       "Back Squat",
		"Bench Press":           "Barbell Bench Press",
	} {
		if got := m.match(source); got == nil || got.Name != want {
			t.Errorf("match(%q) = %v, want %s", source, got, want)
		}
	}
	if m.match("Zercher Carry") != nil || m.match("(Barbell)") != nil {
		t.Error("unexpected match")
	}
	if m.savedExercise("Pull-Up") == nil {
		t.Error("unique saved exercise not linked")
	}
	if m.savedExercise("Curl") != nil {
		t.Error("ambiguous saved exercise linked")
	}
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}
