package service

import (
	"context"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func strPtr(s string) *string { return &s }

func TestLogService_Update_Replacement(t *testing.T) {
	uid := uuid.New()
	owned := uuid.New()

	tplRepo := &MockTemplateDAO{
		GetByIDFunc: func(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error) {
			if templateID == owned {
				return &model.WorkoutTemplate{ID: templateID, TypeID: "legs", SubtypeID: "strength"}, nil
			}
			return nil, &model.NotFoundError{Message: "template not found"}
		},
	}

	newSvc := func(captured **model.LogReplacement) LogService {
		repo := &MockLogDAO{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, date string, o []model.ExerciseOverride, n *string, rep *model.LogReplacement) error {
				*captured = rep
				return nil
			},
			GetByDateFunc: func(ctx context.Context, u uuid.UUID, date string) (*model.DayLog, error) {
				return &model.DayLog{Date: date}, nil
			},
		}
		return NewLogService(repo, tplRepo, validator.New())
	}

	t.Run("template replacement derives type/subtype from template", func(t *testing.T) {
		var rep *model.LogReplacement
		// type_id in the request deliberately disagrees — template is authoritative
		_, err := newSvc(&rep).Update(context.Background(), uid, "2026-06-10",
			model.UpdateDayLogRequest{TemplateID: &owned, TypeID: strPtr("push")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rep == nil || rep.TypeID != "legs" || rep.SubtypeID != "strength" || rep.TemplateID == nil {
			t.Fatalf("replacement = %+v", rep)
		}
	})

	t.Run("type-only replacement validated and forwarded", func(t *testing.T) {
		var rep *model.LogReplacement
		_, err := newSvc(&rep).Update(context.Background(), uid, "2026-06-10",
			model.UpdateDayLogRequest{TypeID: strPtr("pull"), SubtypeID: strPtr("hypertrophy")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rep == nil || rep.TypeID != "pull" || rep.TemplateID != nil {
			t.Fatalf("replacement = %+v", rep)
		}
	})

	t.Run("plain notes update sends no replacement", func(t *testing.T) {
		var rep *model.LogReplacement
		_, err := newSvc(&rep).Update(context.Background(), uid, "2026-06-10",
			model.UpdateDayLogRequest{SessionNotes: strPtr("solid session")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rep != nil {
			t.Fatalf("expected nil replacement, got %+v", rep)
		}
	})

	t.Run("foreign template rejected", func(t *testing.T) {
		var rep *model.LogReplacement
		foreign := uuid.New()
		_, err := newSvc(&rep).Update(context.Background(), uid, "2026-06-10",
			model.UpdateDayLogRequest{TemplateID: &foreign})
		var nf *model.NotFoundError
		if !errors.As(err, &nf) {
			t.Fatalf("want NotFoundError, got %v", err)
		}
	})

	t.Run("type without subtype rejected", func(t *testing.T) {
		var rep *model.LogReplacement
		_, err := newSvc(&rep).Update(context.Background(), uid, "2026-06-10",
			model.UpdateDayLogRequest{TypeID: strPtr("pull")})
		var v *model.ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("want ValidationError, got %v", err)
		}
	})

	t.Run("invalid type id rejected", func(t *testing.T) {
		var rep *model.LogReplacement
		_, err := newSvc(&rep).Update(context.Background(), uid, "2026-06-10",
			model.UpdateDayLogRequest{TypeID: strPtr("yoga-ish"), SubtypeID: strPtr("general")})
		var v *model.ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("want ValidationError, got %v", err)
		}
	})

	t.Run("replacing to rest with overrides rejected (mirrors Create)", func(t *testing.T) {
		var rep *model.LogReplacement
		_, err := newSvc(&rep).Update(context.Background(), uid, "2026-06-10",
			model.UpdateDayLogRequest{
				TypeID:    strPtr("rest"),
				SubtypeID: strPtr("general"),
				Overrides: []model.CreateOverrideRequest{{ExerciseID: uuid.New(), Skipped: true}},
			})
		var v *model.ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("want ValidationError, got %v", err)
		}
		if rep != nil {
			t.Fatalf("guard must reject before reaching the DAO, got replacement %+v", rep)
		}
	})

	t.Run("replacing to rest without overrides is allowed", func(t *testing.T) {
		var rep *model.LogReplacement
		_, err := newSvc(&rep).Update(context.Background(), uid, "2026-06-10",
			model.UpdateDayLogRequest{TypeID: strPtr("rest"), SubtypeID: strPtr("general")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rep == nil || rep.TypeID != "rest" {
			t.Fatalf("replacement = %+v", rep)
		}
	})
}
