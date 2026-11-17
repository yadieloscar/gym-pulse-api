//go:build integration

package dao

import (
	"testing"
	"time"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// A timed exercise and the chosen stretching routines survive a save, an
// update, the day-log template read, and the data export (spec 033).
func TestTemplateKeepsTimedExercisesAndChosenRoutines(t *testing.T) {
	ctx, pool, logs, userID := newLogOnceFixture(t)
	templates := &templateDAO{pool: pool}
	intp := func(v int) *int { return &v }
	strp := func(v string) *string { return &v }

	saved := &model.WorkoutTemplate{
		Name: "Legs", TypeID: "legs", SubtypeID: "general",
		WarmupRoutineID: strp("warmup-lower"), CooldownRoutineID: strp("hips-hamstrings"),
		Exercises: []model.Exercise{
			{Name: "Back squat", Sets: intp(3), Reps: intp(5)},
			{Name: "Couch stretch", Sets: intp(2), DurationSeconds: intp(30)},
		},
	}
	if err := templates.Create(ctx, userID, saved); err != nil {
		t.Fatal(err)
	}

	got, err := templates.GetByID(ctx, userID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WarmupRoutineID == nil || *got.WarmupRoutineID != "warmup-lower" || got.CooldownRoutineID == nil || *got.CooldownRoutineID != "hips-hamstrings" {
		t.Fatalf("routines = %v %v", got.WarmupRoutineID, got.CooldownRoutineID)
	}
	stretch := got.Exercises[1]
	if stretch.DurationSeconds == nil || *stretch.DurationSeconds != 30 || stretch.Reps != nil || *stretch.Sets != 2 {
		t.Fatalf("timed exercise = %+v", stretch)
	}
	if got.Exercises[0].DurationSeconds != nil {
		t.Fatalf("a rep exercise gained a duration: %+v", got.Exercises[0])
	}

	// The day log's embedded template carries the seconds too.
	if _, err := pool.Exec(ctx, `INSERT INTO day_logs (user_id, date, type_id, subtype_id, template_id) VALUES ($1, '2026-11-14', 'legs', 'general', $2)`, userID, saved.ID); err != nil {
		t.Fatal(err)
	}
	log, err := logs.GetByDate(ctx, userID, "2026-11-14")
	if err != nil {
		t.Fatal(err)
	}
	if log.Template == nil || len(log.Template.Exercises) != 2 || log.Template.Exercises[1].DurationSeconds == nil || *log.Template.Exercises[1].DurationSeconds != 30 {
		t.Fatalf("day log template = %+v", log.Template)
	}

	export, err := NewExportDAO(pool).Export(ctx, userID, time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(export.Templates) != 1 || export.Templates[0].WarmupRoutineID == nil || *export.Templates[0].WarmupRoutineID != "warmup-lower" ||
		export.Templates[0].Exercises[1].DurationSeconds == nil || *export.Templates[0].Exercises[1].DurationSeconds != 30 {
		t.Fatalf("export templates = %+v", export.Templates)
	}

	// Clearing a routine stores NULL; the other choice stays.
	got.WarmupRoutineID = nil
	if err := templates.Update(ctx, userID, got); err != nil {
		t.Fatal(err)
	}
	after, err := templates.GetByID(ctx, userID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.WarmupRoutineID != nil || after.CooldownRoutineID == nil {
		t.Fatalf("after clearing the warm-up: %v %v", after.WarmupRoutineID, after.CooldownRoutineID)
	}

	// The database refuses what the service would have refused.
	for name, sql := range map[string]string{
		"a routine id that is not a slug": `UPDATE workout_templates SET warmup_routine_id='Warm Up' WHERE id=$1`,
		"zero seconds":                    `UPDATE exercises SET duration_seconds=0 WHERE template_id=$1`,
		"more than an hour":               `UPDATE exercises SET duration_seconds=3601 WHERE template_id=$1`,
	} {
		if _, err := pool.Exec(ctx, sql, saved.ID); err == nil {
			t.Fatalf("the database accepted %s", name)
		}
	}
}
