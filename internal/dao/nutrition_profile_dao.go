package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const nutritionProfileScope = "nutrition-profile/put"

const nutritionProfileColumns = `unit_system, weight, height, age_years, sex, strength_days, cardio_days,
	activity_level, goal, calorie_adjustment_kcal, protein_g_per_lb, fat_g_per_lb, carb_cycling,
	preferred_proteins, preferred_carbs, exclusions, meal_choices,
	bmr_kcal, tdee_kcal, target_kcal, protein_g, carbs_g, fat_g, revision, created_at, updated_at`

// NutritionProfileDAO owns the nutrition_profiles table: one saved macro
// calculator plan per athlete.
type NutritionProfileDAO interface {
	// Get returns the athlete's saved profile, or a NotFoundError.
	Get(ctx context.Context, userID uuid.UUID) (*model.NutritionProfile, error)
	// Save stores profile as the athlete's plan atomically with the
	// idempotency ledger. expectedRevision is the stored revision, 0 when no
	// profile exists; a mismatch is a ConflictError carrying the stored row.
	Save(ctx context.Context, userID uuid.UUID, profile *model.NutritionProfile, expectedRevision int64, operationKey, requestHash string) (*model.NutritionProfile, model.NutritionProfileOutcome, error)
}

type nutritionProfileDAO struct{ pool *pgxpool.Pool }

func NewNutritionProfileDAO(pool *pgxpool.Pool) NutritionProfileDAO {
	return &nutritionProfileDAO{pool: pool}
}

// nutritionProfileLedger is the idempotency ledger document for a save. It
// carries the outcome so a verbatim retry replays the original status.
type nutritionProfileLedger struct {
	Outcome model.NutritionProfileOutcome `json:"outcome"`
	Profile *model.NutritionProfile       `json:"profile"`
}

func (r *nutritionProfileDAO) Get(ctx context.Context, userID uuid.UUID) (*model.NutritionProfile, error) {
	profile, err := scanNutritionProfile(r.pool.QueryRow(ctx, `
		SELECT `+nutritionProfileColumns+`
		FROM nutrition_profiles
		WHERE user_id=$1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "nutrition plan not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying nutrition profile: %w", err)
	}
	return profile, nil
}

func (r *nutritionProfileDAO) Save(ctx context.Context, userID uuid.UUID, profile *model.NutritionProfile, expectedRevision int64, operationKey, requestHash string) (*model.NutritionProfile, model.NutritionProfileOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.NutritionProfileCreated, fmt.Errorf("beginning nutrition profile save: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.NutritionProfileCreated, err
	}
	var replay nutritionProfileLedger
	if found, err := findIdempotency(ctx, tx, userID, nutritionProfileScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.NutritionProfileCreated, err
	} else if found {
		return replay.Profile, replay.Outcome, nil
	}

	current, err := scanNutritionProfile(tx.QueryRow(ctx, `
		SELECT `+nutritionProfileColumns+`
		FROM nutrition_profiles
		WHERE user_id=$1
		FOR UPDATE`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		current = nil
	} else if err != nil {
		return nil, model.NutritionProfileCreated, fmt.Errorf("querying nutrition profile: %w", err)
	}
	var actual int64
	if current != nil {
		actual = current.Revision
	}
	if expectedRevision != actual {
		var authoritative any
		if current != nil {
			authoritative = current
		}
		return nil, model.NutritionProfileCreated, &model.ConflictError{Message: "resource revision conflict", Expected: expectedRevision, Actual: actual, Authoritative: authoritative}
	}

	choices, err := json.Marshal(profile.Input.MealChoices)
	if err != nil {
		return nil, model.NutritionProfileCreated, fmt.Errorf("encoding meal choices: %w", err)
	}
	in := profile.Input
	args := []any{
		userID, in.UnitSystem, in.Weight, in.Height, in.AgeYears, in.Sex, in.StrengthDays, in.CardioDays,
		in.ActivityLevel, in.Goal, in.CalorieAdjustmentKcal, in.ProteinGPerLb, in.FatGPerLb, in.CarbCycling,
		in.PreferredProteins, in.PreferredCarbs, in.Exclusions, choices,
		profile.BMRKcal, profile.TDEEKcal, profile.TargetKcal, profile.ProteinG, profile.CarbsG, profile.FatG,
	}
	var result *model.NutritionProfile
	outcome := model.NutritionProfileCreated
	status := 201
	if current == nil {
		result, err = scanNutritionProfile(tx.QueryRow(ctx, `
			INSERT INTO nutrition_profiles (
				user_id, unit_system, weight, height, age_years, sex, strength_days, cardio_days,
				activity_level, goal, calorie_adjustment_kcal, protein_g_per_lb, fat_g_per_lb, carb_cycling,
				preferred_proteins, preferred_carbs, exclusions, meal_choices,
				bmr_kcal, tdee_kcal, target_kcal, protein_g, carbs_g, fat_g)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
			RETURNING `+nutritionProfileColumns, args...))
		if err != nil {
			return nil, model.NutritionProfileCreated, fmt.Errorf("inserting nutrition profile: %w", err)
		}
	} else {
		outcome, status = model.NutritionProfileReplaced, 200
		result, err = scanNutritionProfile(tx.QueryRow(ctx, `
			UPDATE nutrition_profiles SET
				unit_system=$2, weight=$3, height=$4, age_years=$5, sex=$6, strength_days=$7, cardio_days=$8,
				activity_level=$9, goal=$10, calorie_adjustment_kcal=$11, protein_g_per_lb=$12, fat_g_per_lb=$13, carb_cycling=$14,
				preferred_proteins=$15, preferred_carbs=$16, exclusions=$17, meal_choices=$18,
				bmr_kcal=$19, tdee_kcal=$20, target_kcal=$21, protein_g=$22, carbs_g=$23, fat_g=$24,
				revision=revision+1, updated_at=now()
			WHERE user_id=$1
			RETURNING `+nutritionProfileColumns, args...))
		if err != nil {
			return nil, model.NutritionProfileCreated, fmt.Errorf("updating nutrition profile: %w", err)
		}
	}

	record := model.IdempotencyRecord{
		Scope:            nutritionProfileScope,
		OperationKey:     operationKey,
		RequestHash:      requestHash,
		ResponseStatus:   status,
		ResourceType:     "nutrition_profile",
		ResourceID:       &userID,
		ResourceRevision: &result.Revision,
	}
	if err := insertIdempotency(ctx, tx, userID, record, nutritionProfileLedger{Outcome: outcome, Profile: result}); err != nil {
		return nil, model.NutritionProfileCreated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.NutritionProfileCreated, fmt.Errorf("committing nutrition profile save: %w", err)
	}
	return result, outcome, nil
}

func scanNutritionProfile(row rowScanner) (*model.NutritionProfile, error) {
	profile := &model.NutritionProfile{}
	in := &profile.Input
	var adjustment int
	var proteinRate, fatRate float64
	var choices []byte
	if err := row.Scan(
		&in.UnitSystem, &in.Weight, &in.Height, &in.AgeYears, &in.Sex, &in.StrengthDays, &in.CardioDays,
		&in.ActivityLevel, &in.Goal, &adjustment, &proteinRate, &fatRate, &in.CarbCycling,
		&in.PreferredProteins, &in.PreferredCarbs, &in.Exclusions, &choices,
		&profile.BMRKcal, &profile.TDEEKcal, &profile.TargetKcal, &profile.ProteinG, &profile.CarbsG, &profile.FatG,
		&profile.Revision, &profile.CreatedAt, &profile.UpdatedAt,
	); err != nil {
		return nil, err
	}
	in.CalorieAdjustmentKcal, in.ProteinGPerLb, in.FatGPerLb = &adjustment, &proteinRate, &fatRate
	if err := json.Unmarshal(choices, &in.MealChoices); err != nil {
		return nil, fmt.Errorf("decoding meal choices: %w", err)
	}
	if in.MealChoices == nil {
		in.MealChoices = []model.MealChoice{}
	}
	return profile, nil
}
