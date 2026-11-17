//go:build integration

package dao

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func nutritionProfileInput() *model.NutritionProfile {
	adjustment, protein, fat := -400, 0.95, 0.33
	level := "very"
	return &model.NutritionProfile{
		Input: model.CalculatorInput{
			UnitSystem: "imperial", Weight: 180.5, Height: 70, AgeYears: 30, Sex: "male",
			StrengthDays: 4, CardioDays: 1, ActivityLevel: &level, Goal: "fat_loss",
			CalorieAdjustmentKcal: &adjustment, ProteinGPerLb: &protein, FatGPerLb: &fat, CarbCycling: true,
			PreferredProteins: []string{"chicken_breast", "eggs"}, PreferredCarbs: []string{},
			Exclusions:  []string{"dairy"},
			MealChoices: []model.MealChoice{{Slot: 2, Carb: "flour_tortillas"}},
		},
		BMRKcal: 1783, TDEEKcal: 3075, TargetKcal: 2675, ProteinG: 171, CarbsG: 365, FatG: 59,
	}
}

func isNotFoundError(err error) bool {
	var notFound *model.NotFoundError
	return errors.As(err, &notFound)
}

func isIdempotencyConflict(err error) bool {
	var conflict *model.ConflictError
	return errors.As(err, &conflict) && strings.Contains(conflict.Message, "idempotency")
}

func nutritionProfileCount(t *testing.T, f *mobilityFixture, userID uuid.UUID) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM nutrition_profiles WHERE user_id=$1`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestNutritionProfileCreateReplayReplaceAndConflict(t *testing.T) {
	f := newMobilityFixture(t)
	profiles := NewNutritionProfileDAO(f.pool)

	if _, err := profiles.Get(f.ctx, f.userID); !isNotFoundError(err) {
		t.Fatalf("Get before save = %v, want not found", err)
	}

	created, outcome, err := profiles.Save(f.ctx, f.userID, nutritionProfileInput(), 0, "np-1", "h1")
	if err != nil || outcome != model.NutritionProfileCreated || created.Revision != 1 {
		t.Fatalf("create: %+v outcome=%v err=%v", created, outcome, err)
	}
	in := created.Input
	if in.Weight != 180.5 || in.ActivityLevel == nil || *in.ActivityLevel != "very" || *in.CalorieAdjustmentKcal != -400 ||
		*in.ProteinGPerLb != 0.95 || *in.FatGPerLb != 0.33 || !in.CarbCycling ||
		len(in.PreferredProteins) != 2 || in.PreferredCarbs == nil || len(in.PreferredCarbs) != 0 ||
		len(in.Exclusions) != 1 || len(in.MealChoices) != 1 || in.MealChoices[0] != (model.MealChoice{Slot: 2, Carb: "flour_tortillas"}) {
		t.Fatalf("stored input round trip: %+v", in)
	}
	if created.TargetKcal != 2675 || created.ProteinG != 171 || created.CarbsG != 365 || created.FatG != 59 {
		t.Fatalf("stored targets: %+v", created)
	}

	replay, outcome, err := profiles.Save(f.ctx, f.userID, nutritionProfileInput(), 0, "np-1", "h1")
	if err != nil || outcome != model.NutritionProfileCreated || replay.Revision != 1 || !replay.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("replay: %+v outcome=%v err=%v", replay, outcome, err)
	}

	if _, _, err := profiles.Save(f.ctx, f.userID, nutritionProfileInput(), 0, "np-1", "other-hash"); !isIdempotencyConflict(err) {
		t.Fatalf("changed payload under the same key = %v, want idempotency conflict", err)
	}

	_, _, err = profiles.Save(f.ctx, f.userID, nutritionProfileInput(), 0, "np-stale", "h-stale")
	var conflict *model.ConflictError
	if !errors.As(err, &conflict) || conflict.Expected != 0 || conflict.Actual != 1 {
		t.Fatalf("stale save = %v, want revision conflict 0 vs 1", err)
	}
	if stored, ok := conflict.Authoritative.(*model.NutritionProfile); !ok || stored.Revision != 1 {
		t.Fatalf("conflict must carry the stored row, got %#v", conflict.Authoritative)
	}

	next := nutritionProfileInput()
	next.Input.ActivityLevel = nil
	next.Input.Goal = "maintenance"
	zero := 0
	next.Input.CalorieAdjustmentKcal = &zero
	next.Input.MealChoices = []model.MealChoice{}
	next.TargetKcal = 2763
	replaced, outcome, err := profiles.Save(f.ctx, f.userID, next, 1, "np-2", "h2")
	if err != nil || outcome != model.NutritionProfileReplaced || replaced.Revision != 2 || replaced.Input.ActivityLevel != nil || replaced.TargetKcal != 2763 || len(replaced.Input.MealChoices) != 0 {
		t.Fatalf("replace: %+v outcome=%v err=%v", replaced, outcome, err)
	}
	if !replaced.CreatedAt.Equal(created.CreatedAt) {
		t.Fatal("replace must keep created_at")
	}

	read, err := profiles.Get(f.ctx, f.userID)
	if err != nil || read.Revision != 2 || read.Input.Goal != "maintenance" {
		t.Fatalf("read: %+v err=%v", read, err)
	}
	if nutritionProfileCount(t, f, f.userID) != 1 {
		t.Fatal("an athlete has exactly one profile row")
	}

	other := f.newUser(t)
	if _, err := profiles.Get(f.ctx, other); !isNotFoundError(err) {
		t.Fatalf("another athlete's read = %v, want not found", err)
	}
}

func TestNutritionProfileConcurrentFirstSavesCreateOnce(t *testing.T) {
	f := newMobilityFixture(t)
	profiles := NewNutritionProfileDAO(f.pool)
	const writers = 6
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, errs[i] = profiles.Save(f.ctx, f.userID, nutritionProfileInput(), 0, "np-race-"+uuid.NewString(), "h")
		}()
	}
	wg.Wait()
	created := 0
	for _, err := range errs {
		var conflict *model.ConflictError
		switch {
		case err == nil:
			created++
		case errors.As(err, &conflict):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != 1 || nutritionProfileCount(t, f, f.userID) != 1 {
		t.Fatalf("created=%d rows=%d, want exactly one", created, nutritionProfileCount(t, f, f.userID))
	}
}

func TestNutritionProfileConstraintsRejectInvalidRows(t *testing.T) {
	f := newMobilityFixture(t)
	profiles := NewNutritionProfileDAO(f.pool)
	for name, mutate := range map[string]func(*model.NutritionProfile){
		"unit system": func(p *model.NutritionProfile) { p.Input.UnitSystem = "stone" },
		"under 18":    func(p *model.NutritionProfile) { p.Input.AgeYears = 17 },
		"sex":         func(p *model.NutritionProfile) { p.Input.Sex = "x" },
		"goal":        func(p *model.NutritionProfile) { p.Input.Goal = "bulk" },
		"days":        func(p *model.NutritionProfile) { p.Input.StrengthDays = 8 },
		"rate":        func(p *model.NutritionProfile) { rate := 1.5; p.Input.ProteinGPerLb = &rate },
		"adjustment":  func(p *model.NutritionProfile) { adj := -900; p.Input.CalorieAdjustmentKcal = &adj },
		"target":      func(p *model.NutritionProfile) { p.TargetKcal = 0 },
		"too many choices": func(p *model.NutritionProfile) {
			p.Input.MealChoices = make([]model.MealChoice, 5)
		},
	} {
		t.Run(name, func(t *testing.T) {
			profile := nutritionProfileInput()
			mutate(profile)
			if _, _, err := profiles.Save(f.ctx, f.userID, profile, 0, "np-bad-"+uuid.NewString(), "h"); err == nil {
				t.Fatal("the table must reject this row")
			}
			if nutritionProfileCount(t, f, f.userID) != 0 {
				t.Fatal("a rejected save must store nothing")
			}
		})
	}
}

func TestNutritionProfileIsRemovedWithTheAccountAndExported(t *testing.T) {
	f := newMobilityFixture(t)
	profiles := NewNutritionProfileDAO(f.pool)
	if _, _, err := profiles.Save(f.ctx, f.userID, nutritionProfileInput(), 0, "np-acct", "h"); err != nil {
		t.Fatal(err)
	}
	export, err := NewExportDAO(f.pool).Export(f.ctx, f.userID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := export.Nutrition.CalculatorProfile; got == nil || got.Goal != "fat_loss" || got.TargetKcal != 2675 || got.Weight != 180.5 {
		t.Fatalf("export calculator profile = %+v", got)
	}
	if err := NewAccountDAO(f.pool).DeleteUserData(f.ctx, f.userID); err != nil {
		t.Fatal(err)
	}
	if nutritionProfileCount(t, f, f.userID) != 0 {
		t.Fatal("account deletion must remove the nutrition profile")
	}

	cascaded := f.newUser(t)
	if _, _, err := profiles.Save(f.ctx, cascaded, nutritionProfileInput(), 0, "np-cascade", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM auth.users WHERE id=$1`, cascaded); err != nil {
		t.Fatal(err)
	}
	if nutritionProfileCount(t, f, cascaded) != 0 {
		t.Fatal("deleting the auth identity must cascade to the nutrition profile")
	}
}

func TestNutritionProfileWorksAsRuntimeRole(t *testing.T) {
	f := newMobilityFixture(t)
	var canAssume bool
	if err := f.pool.QueryRow(f.ctx, `SELECT pg_has_role(current_user, 'gym_pulse_api', 'MEMBER')`).Scan(&canAssume); err != nil {
		t.Fatal(err)
	}
	if !canAssume {
		t.Skip("test database user cannot assume gym_pulse_api")
	}
	config := f.pool.Config().Copy()
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET ROLE gym_pulse_api`)
		return err
	}
	runtime, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)

	profiles := NewNutritionProfileDAO(runtime)
	if _, outcome, err := profiles.Save(f.ctx, f.userID, nutritionProfileInput(), 0, "np-runtime-1", "h1"); err != nil || outcome != model.NutritionProfileCreated {
		t.Fatalf("create as runtime role: outcome=%v err=%v", outcome, err)
	}
	if read, err := profiles.Get(f.ctx, f.userID); err != nil || read.Revision != 1 {
		t.Fatalf("read as runtime role: %+v err=%v", read, err)
	}
}
