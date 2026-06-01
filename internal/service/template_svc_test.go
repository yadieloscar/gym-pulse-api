package service

import (
	"context"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func validCreateReq() model.CreateTemplateRequest {
	sets := 3
	reps := 10
	return model.CreateTemplateRequest{
		Name:      "Push Day",
		TypeID:    "push",
		SubtypeID: "hypertrophy",
		Exercises: []model.CreateExerciseRequest{
			{Name: "Bench Press", Sets: &sets, Reps: &reps},
		},
	}
}

func TestTemplateService_List(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success no filters", func(t *testing.T) {
		called := false
		repo := &MockTemplateDAO{
			ListFunc: func(ctx context.Context, id uuid.UUID, tf, sf string) ([]model.TemplateSummary, error) {
				called = true
				if tf != "" || sf != "" {
					t.Errorf("expected empty filters")
				}
				return []model.TemplateSummary{{Name: "T1"}}, nil
			},
		}
		svc := NewTemplateService(repo, validator.New())
		got, err := svc.List(ctx, userID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if !called || len(got) != 1 {
			t.Errorf("unexpected")
		}
	})

	t.Run("success with valid filters", func(t *testing.T) {
		repo := &MockTemplateDAO{
			ListFunc: func(ctx context.Context, id uuid.UUID, tf, sf string) ([]model.TemplateSummary, error) {
				return nil, nil
			},
		}
		svc := NewTemplateService(repo, validator.New())
		if _, err := svc.List(ctx, userID, "push", "hypertrophy"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("invalid type filter", func(t *testing.T) {
		svc := NewTemplateService(&MockTemplateDAO{}, validator.New())
		_, err := svc.List(ctx, userID, "bogus", "")
		var ve *model.ValidationError
		if !errors.As(err, &ve) || ve.Field != "type" {
			t.Errorf("expected validation type, got %v", err)
		}
	})

	t.Run("invalid subtype filter", func(t *testing.T) {
		svc := NewTemplateService(&MockTemplateDAO{}, validator.New())
		_, err := svc.List(ctx, userID, "", "bogus")
		var ve *model.ValidationError
		if !errors.As(err, &ve) || ve.Field != "subtype" {
			t.Errorf("expected validation subtype, got %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repo := &MockTemplateDAO{
			ListFunc: func(ctx context.Context, id uuid.UUID, tf, sf string) ([]model.TemplateSummary, error) {
				return nil, errors.New("db")
			},
		}
		svc := NewTemplateService(repo, validator.New())
		if _, err := svc.List(ctx, userID, "", ""); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestTemplateService_GetByID(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	tid := uuid.New()

	repo := &MockTemplateDAO{
		GetByIDFunc: func(ctx context.Context, uid, tID uuid.UUID) (*model.WorkoutTemplate, error) {
			if tID != tid {
				t.Errorf("template id mismatch")
			}
			return &model.WorkoutTemplate{ID: tid, Name: "T"}, nil
		},
	}
	svc := NewTemplateService(repo, validator.New())
	got, err := svc.GetByID(ctx, userID, tid)
	if err != nil || got.ID != tid {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestTemplateService_Create(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		repo := &MockTemplateDAO{
			CreateFunc: func(ctx context.Context, id uuid.UUID, tmpl *model.WorkoutTemplate) error {
				if tmpl.Name != "Push Day" {
					t.Errorf("name mismatch")
				}
				if len(tmpl.Exercises) != 1 || tmpl.Exercises[0].SortOrder != 0 {
					t.Errorf("exercises malformed")
				}
				return nil
			},
		}
		svc := NewTemplateService(repo, validator.New())
		got, err := svc.Create(ctx, userID, validCreateReq())
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.Name != "Push Day" {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("validator failure - empty name", func(t *testing.T) {
		req := validCreateReq()
		req.Name = ""
		svc := NewTemplateService(&MockTemplateDAO{}, validator.New())
		_, err := svc.Create(ctx, userID, req)
		var ve *model.ValidationError
		if !errors.As(err, &ve) || ve.Field != "body" {
			t.Errorf("expected body validation, got %v", err)
		}
	})

	t.Run("invalid type_id", func(t *testing.T) {
		req := validCreateReq()
		req.TypeID = "nope"
		svc := NewTemplateService(&MockTemplateDAO{}, validator.New())
		_, err := svc.Create(ctx, userID, req)
		var ve *model.ValidationError
		if !errors.As(err, &ve) || ve.Field != "type_id" {
			t.Errorf("expected type_id, got %v", err)
		}
	})

	t.Run("invalid subtype_id", func(t *testing.T) {
		req := validCreateReq()
		req.SubtypeID = "nope"
		svc := NewTemplateService(&MockTemplateDAO{}, validator.New())
		_, err := svc.Create(ctx, userID, req)
		var ve *model.ValidationError
		if !errors.As(err, &ve) || ve.Field != "subtype_id" {
			t.Errorf("expected subtype_id, got %v", err)
		}
	})

	t.Run("invalid exercise (empty name)", func(t *testing.T) {
		req := validCreateReq()
		req.Exercises = []model.CreateExerciseRequest{{Name: ""}}
		svc := NewTemplateService(&MockTemplateDAO{}, validator.New())
		_, err := svc.Create(ctx, userID, req)
		var ve *model.ValidationError
		if !errors.As(err, &ve) || ve.Field != "exercises" {
			t.Errorf("expected exercises, got %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repo := &MockTemplateDAO{
			CreateFunc: func(ctx context.Context, id uuid.UUID, tmpl *model.WorkoutTemplate) error {
				return errors.New("db")
			},
		}
		svc := NewTemplateService(repo, validator.New())
		if _, err := svc.Create(ctx, userID, validCreateReq()); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestTemplateService_Update(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	tid := uuid.New()

	t.Run("success", func(t *testing.T) {
		repo := &MockTemplateDAO{
			GetByIDFunc: func(ctx context.Context, uid, id uuid.UUID) (*model.WorkoutTemplate, error) {
				return &model.WorkoutTemplate{ID: tid, Name: "old"}, nil
			},
			UpdateFunc: func(ctx context.Context, uid uuid.UUID, tmpl *model.WorkoutTemplate) error {
				if tmpl.Name != "Push Day" {
					t.Errorf("expected updated name")
				}
				return nil
			},
		}
		svc := NewTemplateService(repo, validator.New())
		got, err := svc.Update(ctx, userID, tid, validCreateReq())
		if err != nil || got.Name != "Push Day" {
			t.Fatalf("err %v got %+v", err, got)
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		req := validCreateReq()
		req.Name = ""
		svc := NewTemplateService(&MockTemplateDAO{}, validator.New())
		_, err := svc.Update(ctx, userID, tid, req)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("get by id error", func(t *testing.T) {
		repo := &MockTemplateDAO{
			GetByIDFunc: func(ctx context.Context, uid, id uuid.UUID) (*model.WorkoutTemplate, error) {
				return nil, &model.NotFoundError{Message: "not found"}
			},
		}
		svc := NewTemplateService(repo, validator.New())
		_, err := svc.Update(ctx, userID, tid, validCreateReq())
		var nf *model.NotFoundError
		if !errors.As(err, &nf) {
			t.Errorf("expected NotFoundError, got %v", err)
		}
	})

	t.Run("update error", func(t *testing.T) {
		repo := &MockTemplateDAO{
			GetByIDFunc: func(ctx context.Context, uid, id uuid.UUID) (*model.WorkoutTemplate, error) {
				return &model.WorkoutTemplate{ID: tid}, nil
			},
			UpdateFunc: func(ctx context.Context, uid uuid.UUID, tmpl *model.WorkoutTemplate) error {
				return errors.New("db")
			},
		}
		svc := NewTemplateService(repo, validator.New())
		if _, err := svc.Update(ctx, userID, tid, validCreateReq()); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestTemplateService_Delete(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	tid := uuid.New()

	t.Run("success", func(t *testing.T) {
		called := false
		repo := &MockTemplateDAO{
			DeleteFunc: func(ctx context.Context, uid, id uuid.UUID) error {
				called = true
				return nil
			},
		}
		svc := NewTemplateService(repo, validator.New())
		if err := svc.Delete(ctx, userID, tid); err != nil || !called {
			t.Fatalf("err %v called %v", err, called)
		}
	})

	t.Run("error", func(t *testing.T) {
		repo := &MockTemplateDAO{
			DeleteFunc: func(ctx context.Context, uid, id uuid.UUID) error {
				return errors.New("db")
			},
		}
		svc := NewTemplateService(repo, validator.New())
		if err := svc.Delete(ctx, userID, tid); err == nil {
			t.Fatal("expected error")
		}
	})
}
