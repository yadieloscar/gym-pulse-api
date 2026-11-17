package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// Exercise shape rule: exactly one of strength (sets+reps), timed
// (sets+duration_seconds), or cardio (duration_minutes); intensity only rides
// with duration_minutes. Legacy payloads must keep working.
func TestTemplateService_ExerciseShapeValidation(t *testing.T) {
	intp := func(v int) *int { return &v }
	strp := func(v string) *string { return &v }

	base := func(ex model.CreateExerciseRequest) model.CreateTemplateRequest {
		return model.CreateTemplateRequest{
			Name:      "Shape Test",
			TypeID:    "push",
			SubtypeID: "hypertrophy",
			Exercises: []model.CreateExerciseRequest{ex},
		}
	}

	tests := []struct {
		name    string
		ex      model.CreateExerciseRequest
		wantErr bool
	}{
		{
			name: "legacy strength payload accepted",
			ex:   model.CreateExerciseRequest{Name: "Bench Press", Sets: intp(4), Reps: intp(8)},
		},
		{
			name: "cardio with duration accepted",
			ex:   model.CreateExerciseRequest{Name: "Treadmill", DurationMinutes: intp(20)},
		},
		{
			name: "cardio with duration and intensity accepted",
			ex:   model.CreateExerciseRequest{Name: "Rowing", DurationMinutes: intp(15), Intensity: strp("hard")},
		},
		{
			name:    "neither sets/reps nor duration rejected",
			ex:      model.CreateExerciseRequest{Name: "Mystery Move"},
			wantErr: true,
		},
		{
			name:    "both sets/reps and duration rejected",
			ex:      model.CreateExerciseRequest{Name: "Confused", Sets: intp(3), Reps: intp(10), DurationMinutes: intp(20)},
			wantErr: true,
		},
		{
			name:    "intensity without duration rejected",
			ex:      model.CreateExerciseRequest{Name: "Bench Press", Sets: intp(4), Reps: intp(8), Intensity: strp("hard")},
			wantErr: true,
		},
		{
			name: "timed hold with sets and seconds accepted",
			ex:   model.CreateExerciseRequest{Name: "Couch stretch", Sets: intp(2), DurationSeconds: intp(30)},
		},
		{
			name:    "seconds without sets rejected",
			ex:      model.CreateExerciseRequest{Name: "Couch stretch", DurationSeconds: intp(30)},
			wantErr: true,
		},
		{
			name:    "seconds with reps rejected",
			ex:      model.CreateExerciseRequest{Name: "Plank", Sets: intp(3), Reps: intp(1), DurationSeconds: intp(45)},
			wantErr: true,
		},
		{
			name:    "seconds with minutes rejected",
			ex:      model.CreateExerciseRequest{Name: "Bike", Sets: intp(1), DurationSeconds: intp(45), DurationMinutes: intp(20)},
			wantErr: true,
		},
		{
			name:    "seconds above an hour rejected",
			ex:      model.CreateExerciseRequest{Name: "Plank", Sets: intp(1), DurationSeconds: intp(3601)},
			wantErr: true,
		},
		{
			name:    "zero seconds rejected",
			ex:      model.CreateExerciseRequest{Name: "Plank", Sets: intp(1), DurationSeconds: intp(0)},
			wantErr: true,
		},
		{
			name:    "intensity on a timed hold rejected",
			ex:      model.CreateExerciseRequest{Name: "Plank", Sets: intp(1), DurationSeconds: intp(45), Intensity: strp("easy")},
			wantErr: true,
		},
		{
			name:    "invalid intensity value rejected",
			ex:      model.CreateExerciseRequest{Name: "Bike", DurationMinutes: intp(20), Intensity: strp("brutal")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewTemplateService(&MockTemplateDAO{}, validator.New())
			_, err := svc.Create(context.Background(), uuid.New(), base(tt.ex))
			if tt.wantErr {
				var vErr *model.ValidationError
				if !errors.As(err, &vErr) {
					t.Fatalf("want ValidationError, got %T: %v", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// The chosen warm-up and cool-down routines: a slug is stored, an absent
// field keeps the stored choice on update, and null or "" clears it.
func TestTemplateService_RoutineChoice(t *testing.T) {
	ctx := context.Background()
	userID, templateID := uuid.New(), uuid.New()
	strp := func(v string) *string { return &v }
	decode := func(body string) model.CreateTemplateRequest {
		t.Helper()
		var req model.CreateTemplateRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return req
	}
	const exercises = `"exercises":[{"name":"Couch stretch","sets":2,"duration_seconds":30}]`
	const head = `"name":"Legs","type_id":"legs","subtype_id":"general",`

	t.Run("create stores the chosen routines and the timed exercise", func(t *testing.T) {
		var stored *model.WorkoutTemplate
		repo := &MockTemplateDAO{CreateFunc: func(_ context.Context, _ uuid.UUID, tmpl *model.WorkoutTemplate) error {
			stored = tmpl
			return nil
		}}
		req := decode(`{` + head + `"warmup_routine_id":"warmup-lower","cooldown_routine_id":"hips-hamstrings",` + exercises + `}`)
		if _, err := NewTemplateService(repo, validator.New()).Create(ctx, userID, req); err != nil {
			t.Fatal(err)
		}
		if stored.WarmupRoutineID == nil || *stored.WarmupRoutineID != "warmup-lower" || stored.CooldownRoutineID == nil || *stored.CooldownRoutineID != "hips-hamstrings" {
			t.Fatalf("routines = %v %v", stored.WarmupRoutineID, stored.CooldownRoutineID)
		}
		if got := stored.Exercises[0]; got.DurationSeconds == nil || *got.DurationSeconds != 30 || got.Reps != nil {
			t.Fatalf("exercise = %+v", got)
		}
	})

	update := func(t *testing.T, body string) *model.WorkoutTemplate {
		t.Helper()
		var stored *model.WorkoutTemplate
		repo := &MockTemplateDAO{
			GetByIDFunc: func(context.Context, uuid.UUID, uuid.UUID) (*model.WorkoutTemplate, error) {
				return &model.WorkoutTemplate{ID: templateID, WarmupRoutineID: strp("warmup-lower"), CooldownRoutineID: strp("cooldown-lower")}, nil
			},
			UpdateFunc: func(_ context.Context, _ uuid.UUID, tmpl *model.WorkoutTemplate) error {
				stored = tmpl
				return nil
			},
		}
		if _, err := NewTemplateService(repo, validator.New()).Update(ctx, userID, templateID, decode(body)); err != nil {
			t.Fatal(err)
		}
		return stored
	}

	t.Run("update without the fields keeps the stored choice", func(t *testing.T) {
		stored := update(t, `{`+head+exercises+`}`)
		if stored.WarmupRoutineID == nil || *stored.WarmupRoutineID != "warmup-lower" || stored.CooldownRoutineID == nil {
			t.Fatalf("an older client cleared the routines: %v %v", stored.WarmupRoutineID, stored.CooldownRoutineID)
		}
	})

	t.Run("update with null or empty clears, with a slug replaces", func(t *testing.T) {
		stored := update(t, `{`+head+`"warmup_routine_id":null,"cooldown_routine_id":"full-body",`+exercises+`}`)
		if stored.WarmupRoutineID != nil || *stored.CooldownRoutineID != "full-body" {
			t.Fatalf("routines = %v %v", stored.WarmupRoutineID, stored.CooldownRoutineID)
		}
		if stored := update(t, `{`+head+`"cooldown_routine_id":"",`+exercises+`}`); stored.CooldownRoutineID != nil {
			t.Fatalf("an empty string did not clear the cool-down: %v", *stored.CooldownRoutineID)
		}
	})

	t.Run("a routine id that is not a slug is refused with its field", func(t *testing.T) {
		for field, body := range map[string]string{
			"warmup_routine_id":   `{` + head + `"warmup_routine_id":"Warm Up!",` + exercises + `}`,
			"cooldown_routine_id": `{` + head + `"cooldown_routine_id":"` + strings.Repeat("a", 65) + `",` + exercises + `}`,
		} {
			_, err := NewTemplateService(&MockTemplateDAO{}, validator.New()).Create(ctx, userID, decode(body))
			var invalid *model.ValidationError
			if !errors.As(err, &invalid) || invalid.Field != field {
				t.Fatalf("%s: error = %v, want a validation error on that field", field, err)
			}
		}
	})
}
