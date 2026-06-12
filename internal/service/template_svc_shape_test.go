package service

import (
	"context"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// Exercise shape rule: strength (sets+reps) XOR cardio (duration_minutes);
// intensity only rides with duration. Legacy payloads must keep working.
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
