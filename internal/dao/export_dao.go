package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ExportDAO reads everything one athlete owns for a data export.
type ExportDAO interface {
	Export(ctx context.Context, userID uuid.UUID, exportedAt time.Time) (*model.DataExport, error)
}

type exportDAO struct {
	pool *pgxpool.Pool
}

// NewExportDAO creates an ExportDAO backed by the given connection pool.
func NewExportDAO(pool *pgxpool.Pool) ExportDAO {
	return &exportDAO{pool: pool}
}

// Export reads every section inside one read-only repeatable-read transaction,
// so a workout finished mid-export appears whole or not at all. Every query is
// scoped by the authenticated user's id; child rows are reached only through
// an owned parent.
func (r *exportDAO) Export(ctx context.Context, userID uuid.UUID, exportedAt time.Time) (*model.DataExport, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("beginning export transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	out := model.NewDataExport(exportedAt)
	readers := []func(context.Context, pgx.Tx, uuid.UUID, *model.DataExport) error{
		readExportProfile,
		readExportSettings,
		readExportTrainingProfile,
		readExportSessions,
		readExportDayLogs,
		readExportTemplates,
		readExportSportActivities,
		readExportBodyWeights,
		readExportNutritionDays,
		readExportNutritionTargets,
		readExportFoodEntries,
		readExportOwnFoods,
	}
	for _, read := range readers {
		if err := read(ctx, tx, userID, out); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("closing export transaction: %w", err)
	}
	return out, nil
}

func readExportProfile(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	var p model.ExportProfile
	err := tx.QueryRow(ctx, `SELECT display_name, created_at FROM user_profiles WHERE id=$1`, userID).
		Scan(&p.DisplayName, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading export profile: %w", err)
	}
	out.Profile = &p
	return nil
}

func readExportSettings(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	var s model.ExportSettings
	err := tx.QueryRow(ctx, `SELECT weight_unit, weekly_goal FROM user_settings WHERE user_id=$1`, userID).
		Scan(&s.WeightUnit, &s.WeeklyGoal)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading export settings: %w", err)
	}
	out.Settings = &s
	return nil
}

func readExportTrainingProfile(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	var p model.ExportTrainingProfile
	err := tx.QueryRow(ctx, `
		SELECT primary_goal, available_days, usual_activity, experience, equipment,
		       session_duration_minutes, timezone
		FROM training_profiles WHERE user_id=$1`, userID).
		Scan(&p.PrimaryGoal, &p.AvailableDays, &p.UsualActivity, &p.Experience, &p.Equipment,
			&p.SessionDurationMinutes, &p.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading export training profile: %w", err)
	}
	if p.AvailableDays == nil {
		p.AvailableDays = []int16{}
	}
	if p.Equipment == nil {
		p.Equipment = []string{}
	}
	out.TrainingProfile = &p
	return nil
}

// readExportSessions reads goal-based sessions and their sets. A discarded
// session was thrown away by the athlete and is not part of their history.
func readExportSessions(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT id, date::text, name, status, scheduled_workout_id IS NOT NULL, notes, started_at, completed_at
		FROM workout_sessions
		WHERE user_id=$1 AND status <> 'discarded'
		ORDER BY date, created_at, id`, userID)
	if err != nil {
		return fmt.Errorf("querying export sessions: %w", err)
	}
	index := map[uuid.UUID]int{}
	workouts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportWorkout, error) {
		w := model.ExportWorkout{Source: model.ExportWorkoutSession, Sets: []model.ExportSet{}, Exercises: []model.ExportExerciseSummary{}}
		err := row.Scan(&w.ID, &w.Date, &w.Name, &w.Status, &w.Planned, &w.Notes, &w.StartedAt, &w.CompletedAt)
		return w, err
	})
	if err != nil {
		return fmt.Errorf("scanning export sessions: %w", err)
	}
	first := len(out.Workouts)
	for i, w := range workouts {
		index[w.ID] = first + i
	}
	out.Workouts = append(out.Workouts, workouts...)

	setRows, err := tx.Query(ctx, `
		SELECT s.workout_session_id, s.exercise_name, s.exercise_category, s.exercise_modality, s.set_index,
		       s.target_reps, s.target_weight, s.actual_reps, s.actual_weight, s.duration_seconds,
		       s.completed, s.is_extra
		FROM set_logs s
		JOIN workout_sessions ws ON ws.id = s.workout_session_id
		LEFT JOIN scheduled_sets ss ON ss.id = s.scheduled_set_id
		WHERE ws.user_id=$1 AND ws.status <> 'discarded'
		ORDER BY s.workout_session_id, ss.exercise_order NULLS LAST, s.exercise_name, s.set_index, s.logged_at, s.id`, userID)
	if err != nil {
		return fmt.Errorf("querying export session sets: %w", err)
	}
	return collectExportSets(setRows, index, out, "session")
}

// readExportDayLogs reads legacy day logs, their sets, and their aggregate
// exercise summaries.
func readExportDayLogs(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT dl.id, dl.date::text, COALESCE(t.name, dl.type_id),
		       CASE WHEN dl.type_id = 'rest' THEN 'rest' ELSE 'logged' END,
		       dl.session_notes, dl.logged_at
		FROM day_logs dl
		LEFT JOIN workout_templates t ON t.id = dl.template_id AND t.user_id = dl.user_id
		WHERE dl.user_id=$1
		ORDER BY dl.date, dl.logged_at, dl.id`, userID)
	if err != nil {
		return fmt.Errorf("querying export day logs: %w", err)
	}
	logs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportWorkout, error) {
		w := model.ExportWorkout{Source: model.ExportWorkoutDayLog, Sets: []model.ExportSet{}, Exercises: []model.ExportExerciseSummary{}}
		var loggedAt time.Time
		err := row.Scan(&w.ID, &w.Date, &w.Name, &w.Status, &w.Notes, &loggedAt)
		w.CompletedAt = &loggedAt
		return w, err
	})
	if err != nil {
		return fmt.Errorf("scanning export day logs: %w", err)
	}
	index := map[uuid.UUID]int{}
	first := len(out.Workouts)
	for i, w := range logs {
		index[w.ID] = first + i
	}
	out.Workouts = append(out.Workouts, logs...)

	setRows, err := tx.Query(ctx, `
		SELECT s.day_log_id, s.exercise_name, s.exercise_category, s.exercise_modality, s.set_index,
		       s.target_reps, s.target_weight, s.actual_reps, s.actual_weight, s.duration_seconds,
		       s.completed, s.is_extra
		FROM set_logs s
		JOIN day_logs dl ON dl.id = s.day_log_id
		LEFT JOIN exercises e ON e.id = s.exercise_id
		WHERE dl.user_id=$1
		ORDER BY s.day_log_id, e.sort_order NULLS LAST, s.exercise_name, s.set_index, s.logged_at, s.id`, userID)
	if err != nil {
		return fmt.Errorf("querying export day-log sets: %w", err)
	}
	if err := collectExportSets(setRows, index, out, "day-log"); err != nil {
		return err
	}

	overrideRows, err := tx.Query(ctx, `
		SELECT o.day_log_id, e.name, o.actual_sets, o.actual_reps, o.actual_weight, o.notes, COALESCE(o.skipped, false)
		FROM exercise_overrides o
		JOIN day_logs dl ON dl.id = o.day_log_id
		JOIN exercises e ON e.id = o.exercise_id
		WHERE dl.user_id=$1
		ORDER BY o.day_log_id, e.sort_order, o.id`, userID)
	if err != nil {
		return fmt.Errorf("querying export exercise summaries: %w", err)
	}
	defer overrideRows.Close()
	for overrideRows.Next() {
		var parent uuid.UUID
		var e model.ExportExerciseSummary
		if err := overrideRows.Scan(&parent, &e.ExerciseName, &e.ActualSets, &e.ActualReps, &e.ActualWeight, &e.Notes, &e.Skipped); err != nil {
			return fmt.Errorf("scanning export exercise summary: %w", err)
		}
		if i, ok := index[parent]; ok {
			out.Workouts[i].Exercises = append(out.Workouts[i].Exercises, e)
		}
	}
	if err := overrideRows.Err(); err != nil {
		return fmt.Errorf("iterating export exercise summaries: %w", err)
	}
	return nil
}

func collectExportSets(rows pgx.Rows, index map[uuid.UUID]int, out *model.DataExport, kind string) error {
	defer rows.Close()
	for rows.Next() {
		var parent uuid.UUID
		var s model.ExportSet
		if err := rows.Scan(&parent, &s.ExerciseName, &s.ExerciseCategory, &s.ExerciseModality, &s.SetIndex,
			&s.TargetReps, &s.TargetWeight, &s.ActualReps, &s.ActualWeight, &s.DurationSeconds,
			&s.Completed, &s.Extra); err != nil {
			return fmt.Errorf("scanning export %s set: %w", kind, err)
		}
		if i, ok := index[parent]; ok {
			out.Workouts[i].Sets = append(out.Workouts[i].Sets, s)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating export %s sets: %w", kind, err)
	}
	return nil
}

func readExportTemplates(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT id, name, type_id, subtype_id, created_at, updated_at
		FROM workout_templates
		WHERE user_id=$1
		ORDER BY created_at, id`, userID)
	if err != nil {
		return fmt.Errorf("querying export templates: %w", err)
	}
	templates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportTemplate, error) {
		t := model.ExportTemplate{Exercises: []model.ExportTemplateExercise{}}
		err := row.Scan(&t.ID, &t.Name, &t.TypeID, &t.SubtypeID, &t.CreatedAt, &t.UpdatedAt)
		return t, err
	})
	if err != nil {
		return fmt.Errorf("scanning export templates: %w", err)
	}
	index := map[uuid.UUID]int{}
	for i, t := range templates {
		index[t.ID] = i
	}

	exerciseRows, err := tx.Query(ctx, `
		SELECT e.template_id, e.name, e.sets, e.reps, e.weight, e.rest_seconds, e.duration_minutes, e.intensity, e.notes
		FROM exercises e
		JOIN workout_templates t ON t.id = e.template_id
		WHERE t.user_id=$1
		ORDER BY e.template_id, e.sort_order, e.id`, userID)
	if err != nil {
		return fmt.Errorf("querying export template exercises: %w", err)
	}
	defer exerciseRows.Close()
	for exerciseRows.Next() {
		var parent uuid.UUID
		var e model.ExportTemplateExercise
		if err := exerciseRows.Scan(&parent, &e.Name, &e.Sets, &e.Reps, &e.Weight, &e.RestSeconds, &e.DurationMinutes, &e.Intensity, &e.Notes); err != nil {
			return fmt.Errorf("scanning export template exercise: %w", err)
		}
		if i, ok := index[parent]; ok {
			templates[i].Exercises = append(templates[i].Exercises, e)
		}
	}
	if err := exerciseRows.Err(); err != nil {
		return fmt.Errorf("iterating export template exercises: %w", err)
	}
	out.Templates = append(out.Templates, templates...)
	return nil
}

func readExportSportActivities(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT id, date::text, sport_id, sport_name, duration_minutes, perceived_effort,
		       distance_meters, energy_kcal, avg_heart_rate_bpm, source, notes
		FROM sport_activities
		WHERE user_id=$1
		ORDER BY date, created_at, id`, userID)
	if err != nil {
		return fmt.Errorf("querying export sport activities: %w", err)
	}
	activities, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportSportActivity, error) {
		var a model.ExportSportActivity
		err := row.Scan(&a.ID, &a.Date, &a.SportID, &a.SportName, &a.DurationMinutes, &a.PerceivedEffort,
			&a.DistanceMeters, &a.EnergyKcal, &a.AvgHeartRateBPM, &a.Source, &a.Notes)
		return a, err
	})
	if err != nil {
		return fmt.Errorf("scanning export sport activities: %w", err)
	}
	out.SportActivities = append(out.SportActivities, activities...)
	return nil
}

func readExportBodyWeights(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT date::text, weight, unit FROM body_weights WHERE user_id=$1 ORDER BY date`, userID)
	if err != nil {
		return fmt.Errorf("querying export body weights: %w", err)
	}
	weights, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportBodyWeight, error) {
		var w model.ExportBodyWeight
		err := row.Scan(&w.Date, &w.Weight, &w.Unit)
		return w, err
	})
	if err != nil {
		return fmt.Errorf("scanning export body weights: %w", err)
	}
	out.BodyWeights = append(out.BodyWeights, weights...)
	return nil
}

func readExportNutritionDays(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT date::text, energy_kcal, protein_g, carbs_g, fat_g, source
		FROM nutrition_days WHERE user_id=$1 ORDER BY date`, userID)
	if err != nil {
		return fmt.Errorf("querying export nutrition days: %w", err)
	}
	days, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportNutritionDay, error) {
		var d model.ExportNutritionDay
		err := row.Scan(&d.Date, &d.EnergyKcal, &d.ProteinG, &d.CarbsG, &d.FatG, &d.Source)
		return d, err
	})
	if err != nil {
		return fmt.Errorf("scanning export nutrition days: %w", err)
	}
	out.Nutrition.Days = append(out.Nutrition.Days, days...)
	return nil
}

func readExportNutritionTargets(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT effective_from::text, protein_g, energy_kcal, carbs_g, fat_g
		FROM nutrition_targets WHERE user_id=$1 ORDER BY effective_from`, userID)
	if err != nil {
		return fmt.Errorf("querying export nutrition targets: %w", err)
	}
	targets, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportNutritionTarget, error) {
		var t model.ExportNutritionTarget
		err := row.Scan(&t.EffectiveFrom, &t.ProteinG, &t.EnergyKcal, &t.CarbsG, &t.FatG)
		return t, err
	})
	if err != nil {
		return fmt.Errorf("scanning export nutrition targets: %w", err)
	}
	out.Nutrition.Targets = append(out.Nutrition.Targets, targets...)
	return nil
}

func readExportFoodEntries(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT fe.date::text, fe.kind, COALESCE(uf.name, fc.name), fe.serving_label, fe.serving_g, fe.amount,
		       fe.energy_kcal, fe.protein_g, fe.carbs_g, fe.fat_g, fe.logged_at
		FROM food_entries fe
		LEFT JOIN user_foods uf ON uf.id = fe.user_food_id AND uf.user_id = fe.user_id
		LEFT JOIN food_catalog fc ON fc.id = fe.catalog_food_id
		WHERE fe.user_id=$1
		ORDER BY fe.date, fe.logged_at, fe.id`, userID)
	if err != nil {
		return fmt.Errorf("querying export food entries: %w", err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportFoodEntry, error) {
		var e model.ExportFoodEntry
		err := row.Scan(&e.Date, &e.Kind, &e.FoodName, &e.ServingLabel, &e.ServingG, &e.Amount,
			&e.EnergyKcal, &e.ProteinG, &e.CarbsG, &e.FatG, &e.LoggedAt)
		return e, err
	})
	if err != nil {
		return fmt.Errorf("scanning export food entries: %w", err)
	}
	out.Nutrition.FoodEntries = append(out.Nutrition.FoodEntries, entries...)
	return nil
}

func readExportOwnFoods(ctx context.Context, tx pgx.Tx, userID uuid.UUID, out *model.DataExport) error {
	rows, err := tx.Query(ctx, `
		SELECT name, brand, serving_label, serving_g, energy_kcal, protein_g, carbs_g, fat_g, archived_at IS NOT NULL
		FROM user_foods WHERE user_id=$1 ORDER BY created_at, id`, userID)
	if err != nil {
		return fmt.Errorf("querying export own foods: %w", err)
	}
	foods, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.ExportOwnFood, error) {
		var f model.ExportOwnFood
		err := row.Scan(&f.Name, &f.Brand, &f.ServingLabel, &f.ServingG, &f.EnergyKcal, &f.ProteinG, &f.CarbsG, &f.FatG, &f.Archived)
		return f, err
	})
	if err != nil {
		return fmt.Errorf("scanning export own foods: %w", err)
	}
	out.Nutrition.OwnFoods = append(out.Nutrition.OwnFoods, foods...)
	return nil
}
