//go:build integration

package dao

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type importFixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	imports WorkoutImportDAO
	logs    LogDAO
	userID  uuid.UUID
}

func newImportFixture(t *testing.T) *importFixture {
	t.Helper()
	url := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("integration tests require GYMPULSE_TEST_DATABASE_URL pointing to a disposable migrated database")
	}
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &importFixture{ctx: ctx, pool: pool, imports: NewWorkoutImportDAO(pool), logs: NewLogDAO(pool)}
	f.userID = f.newUser(t)
	return f
}

func (f *importFixture) newUser(t *testing.T) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO auth.users (id) VALUES ($1)`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.WithoutCancel(f.ctx), `DELETE FROM auth.users WHERE id = $1`, userID)
	})
	return userID
}

func (f *importFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func importDays(exerciseID *uuid.UUID) []model.ImportDay {
	at := time.Date(2025, 3, 7, 12, 0, 0, 0, time.UTC)
	reps, weight, seconds := 5, 100.5, 60
	notes := "Legs and Abs"
	return []model.ImportDay{
		{Date: "2025-03-07", SessionNotes: &notes, LoggedAt: at.Add(time.Hour), Sets: []model.ImportSet{
			{ExerciseID: exerciseID, ExerciseName: "Back Squat", ExerciseCategory: "legs", ExerciseModality: "strength", SetIndex: 1, ActualReps: &reps, ActualWeight: &weight, LoggedAt: at},
			{ExerciseName: "Cable Kickback", ExerciseCategory: "other", ExerciseModality: "strength", SetIndex: 1, ActualReps: &reps, LoggedAt: at.Add(time.Millisecond)},
		}},
		{Date: "2025-03-08", LoggedAt: at.AddDate(0, 0, 1), Sets: []model.ImportSet{
			{ExerciseName: "Plank", ExerciseCategory: "core", ExerciseModality: "strength", SetIndex: 1, DurationSeconds: &seconds, LoggedAt: at.AddDate(0, 0, 1)},
		}},
	}
}

func newImport(days []model.ImportDay) *model.WorkoutImport {
	first, last := days[0].Date, days[len(days)-1].Date
	return &model.WorkoutImport{ID: uuid.New(), Source: model.ImportSourceStrong, SourceWeightUnit: "kg", WeightUnit: "kg",
		Unmatched: "keep", FirstDate: &first, LastDate: &last, WorkoutCount: 2, DayCount: len(days), SetCount: 3}
}

func TestWorkoutImportCommitReplayListDelete(t *testing.T) {
	f := newImportFixture(t)
	days := importDays(nil)
	imp, outcome, err := f.imports.Commit(f.ctx, f.userID, newImport(days), days, "import-op", "hash-a")
	if err != nil || outcome != model.ImportCreated {
		t.Fatal(err, outcome)
	}

	log, err := f.logs.GetByDate(f.ctx, f.userID, "2025-03-07")
	if err != nil {
		t.Fatal(err)
	}
	if log.ImportID == nil || *log.ImportID != imp.ID || log.TypeID != "other" || log.SubtypeID != "general" || *log.SessionNotes != "Legs and Abs" {
		t.Fatalf("log = %+v", log)
	}
	if len(log.SetLogs) != 2 || log.SetLogs[0].ExerciseName != "Back Squat" || log.SetLogs[1].ExerciseName != "Cable Kickback" ||
		log.SetLogs[0].ExerciseID != nil || *log.SetLogs[0].ActualWeight != 100.5 || !log.SetLogs[0].Completed {
		t.Fatalf("sets = %+v", log.SetLogs)
	}
	week, err := f.logs.ListByWeek(f.ctx, f.userID, time.Date(2025, 3, 3, 0, 0, 0, 0, time.UTC))
	if err != nil || len(week) != 2 || week[0].ImportID == nil {
		t.Fatalf("week = %+v, %v", week, err)
	}
	if n := f.count(t, `SELECT count(*) FROM day_participation WHERE user_id=$1`, f.userID); n != 0 {
		t.Fatalf("participation rows = %d", n)
	}

	replay, outcome, err := f.imports.Commit(f.ctx, f.userID, newImport(days), days, "import-op", "hash-a")
	if err != nil || outcome != model.ImportReplayed || replay.ID != imp.ID {
		t.Fatalf("replay = %+v %v %v", replay, outcome, err)
	}
	var conflict *model.ConflictError
	if _, _, err := f.imports.Commit(f.ctx, f.userID, newImport(days), days, "import-op", "hash-b"); !errors.As(err, &conflict) {
		t.Fatalf("different hash = %v", err)
	}
	// A new key over days that now hold logs is refused whole.
	if _, _, err := f.imports.Commit(f.ctx, f.userID, newImport(days), days, "import-op-2", "hash-a"); !errors.As(err, &conflict) {
		t.Fatalf("overlap = %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM day_logs WHERE user_id=$1`, f.userID); n != 2 {
		t.Fatalf("day logs = %d", n)
	}

	dates, err := f.imports.ExistingLogDates(f.ctx, f.userID, "2025-03-01", "2025-03-31")
	if err != nil || !dates["2025-03-07"] || !dates["2025-03-08"] || len(dates) != 2 {
		t.Fatalf("dates = %v %v", dates, err)
	}
	list, err := f.imports.List(f.ctx, f.userID)
	if err != nil || len(list) != 1 || list[0].ID != imp.ID || list[0].DayCount != 2 || *list[0].FirstDate != "2025-03-07" {
		t.Fatalf("list = %+v %v", list, err)
	}

	other := f.newUser(t)
	var notFound *model.NotFoundError
	if err := f.imports.Delete(f.ctx, other, imp.ID); !errors.As(err, &notFound) {
		t.Fatalf("foreign delete = %v", err)
	}
	if err := f.imports.Delete(f.ctx, f.userID, imp.ID); err != nil {
		t.Fatal(err)
	}
	if n := f.count(t, `SELECT count(*) FROM day_logs WHERE user_id=$1`, f.userID); n != 0 {
		t.Fatalf("day logs after delete = %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM set_logs sl JOIN day_logs dl ON dl.id = sl.day_log_id WHERE dl.user_id=$1`, f.userID); n != 0 {
		t.Fatalf("set logs after delete = %d", n)
	}
	if err := f.imports.Delete(f.ctx, f.userID, imp.ID); !errors.As(err, &notFound) {
		t.Fatalf("second delete = %v", err)
	}
}

func TestWorkoutImportLinksSavedExerciseIntoRecords(t *testing.T) {
	f := newImportFixture(t)
	var templateID, exerciseID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO workout_templates (user_id, name, type_id, subtype_id) VALUES ($1, 'Legs', 'legs', 'strength') RETURNING id`, f.userID).Scan(&templateID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO exercises (template_id, name, sets, reps, sort_order) VALUES ($1, 'Back Squat', 3, 5, 0) RETURNING id`, templateID).Scan(&exerciseID); err != nil {
		t.Fatal(err)
	}
	athlete, err := f.imports.Context(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if athlete.SavedExercises[exerciseID] != "Back Squat" || athlete.WeightUnit != "lb" || athlete.Timezone != "UTC" || len(athlete.Catalog) == 0 {
		t.Fatalf("context = %+v", athlete)
	}
	days := importDays(&exerciseID)
	if _, _, err := f.imports.Commit(f.ctx, f.userID, newImport(days), days, "link-op", "hash"); err != nil {
		t.Fatal(err)
	}
	records, err := f.logs.RecordSets(f.ctx, f.userID, []uuid.UUID{exerciseID})
	if err != nil || len(records) != 1 || records[0].Weight != 100.5 || records[0].Date != "2025-03-07" {
		t.Fatalf("records = %+v %v", records, err)
	}
}

func TestImportedDayLogIsNotHealthExportable(t *testing.T) {
	f := newImportFixture(t)
	days := importDays(nil)
	if _, _, err := f.imports.Commit(f.ctx, f.userID, newImport(days), days, "health-op", "hash"); err != nil {
		t.Fatal(err)
	}
	log, err := f.logs.GetByDate(f.ctx, f.userID, "2025-03-07")
	if err != nil {
		t.Fatal(err)
	}
	exports := NewHealthExportDAO(f.pool)
	_, _, err = exports.Record(f.ctx, f.userID, &model.HealthExport{WorkoutKind: "day_log", WorkoutID: log.ID, Source: "apple_health",
		ExternalID: "ext-1", ExportedAt: time.Now()}, "hk-op", "hash")
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("imported log export = %v", err)
	}
}

func TestWorkoutImportCommitsLargeHistoryAcrossChunks(t *testing.T) {
	f := newImportFixture(t)
	start := time.Date(2010, 1, 1, 18, 0, 0, 0, time.UTC)
	reps, weight := 5, 100.0
	days := make([]model.ImportDay, 2500)
	for i := range days {
		at := start.AddDate(0, 0, i)
		sets := make([]model.ImportSet, 10)
		for j := range sets {
			sets[j] = model.ImportSet{ExerciseName: "Back Squat", ExerciseCategory: "legs", ExerciseModality: "strength",
				SetIndex: j + 1, ActualReps: &reps, ActualWeight: &weight, LoggedAt: at.Add(time.Duration(j) * time.Millisecond)}
		}
		days[i] = model.ImportDay{Date: at.Format(time.DateOnly), LoggedAt: at.Add(time.Hour), Sets: sets}
	}
	begin := time.Now()
	if _, _, err := f.imports.Commit(f.ctx, f.userID, newImport(days), days, "large-op", "hash"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(begin); elapsed > 10*time.Second {
		t.Fatalf("25,000 sets took %v", elapsed)
	}
	if n := f.count(t, `SELECT count(*) FROM set_logs sl JOIN day_logs dl ON dl.id = sl.day_log_id WHERE dl.user_id=$1 AND dl.import_id IS NOT NULL`, f.userID); n != 25000 {
		t.Fatalf("set logs = %d", n)
	}
}
