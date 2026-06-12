package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestLogService_ListByWeek(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	todayStr := time.Now().Format("2006-01-02")

	t.Run("success", func(t *testing.T) {
		mockSummaries := []model.DayLogSummary{
			{Date: todayStr, TypeID: "push", SubtypeID: "hypertrophy"},
		}
		repo := &MockLogDAO{
			ListByWeekFunc: func(ctx context.Context, uid uuid.UUID, monday time.Time) ([]model.DayLogSummary, error) {
				if uid != userID {
					t.Errorf("expected userID %s, got %s", userID, uid)
				}
				return mockSummaries, nil
			},
		}

		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		res, err := svc.ListByWeek(ctx, userID, todayStr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1 summary, got %d", len(res))
		}
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.ListByWeek(ctx, userID, "invalid-date")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var valErr *model.ValidationError
		if !errors.As(err, &valErr) {
			t.Errorf("expected ValidationError, got %T", err)
		}
	})
}

func TestLogService_GetByDate(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	todayStr := time.Now().Format("2006-01-02")

	t.Run("success", func(t *testing.T) {
		mockLog := &model.DayLog{
			Date:      todayStr,
			TypeID:    "push",
			SubtypeID: "hypertrophy",
		}
		repo := &MockLogDAO{
			GetByDateFunc: func(ctx context.Context, uid uuid.UUID, date string) (*model.DayLog, error) {
				if uid != userID {
					t.Errorf("expected userID %s, got %s", userID, uid)
				}
				if date != todayStr {
					t.Errorf("expected date %s, got %s", todayStr, date)
				}
				return mockLog, nil
			},
		}

		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		res, err := svc.GetByDate(ctx, userID, todayStr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Date != todayStr {
			t.Errorf("expected date %s, got %s", todayStr, res.Date)
		}
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.GetByDate(ctx, userID, "invalid-date")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLogService_Create(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	todayStr := time.Now().Format("2006-01-02")

	validReq := model.CreateDayLogRequest{
		Date:      todayStr,
		TypeID:    "push",
		SubtypeID: "hypertrophy",
	}

	t.Run("success without template", func(t *testing.T) {
		createCalled := false
		repo := &MockLogDAO{
			CreateFunc: func(ctx context.Context, uid uuid.UUID, l *model.DayLog) error {
				if uid != userID {
					t.Errorf("expected userID %s, got %s", userID, uid)
				}
				if l.Date != todayStr {
					t.Errorf("expected date %s, got %s", todayStr, l.Date)
				}
				createCalled = true
				return nil
			},
		}

		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		log, err := svc.Create(ctx, userID, validReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !createCalled {
			t.Error("expected create to be called")
		}
		if log.TypeID != "push" {
			t.Errorf("expected TypeID push, got %s", log.TypeID)
		}
	})

	t.Run("success with template ownership verified", func(t *testing.T) {
		templateID := uuid.New()
		req := model.CreateDayLogRequest{
			Date:       todayStr,
			TypeID:     "push",
			SubtypeID:  "hypertrophy",
			TemplateID: &templateID,
		}

		templateRepoCalled := false
		templateRepo := &MockTemplateDAO{
			GetByIDFunc: func(ctx context.Context, uid, tid uuid.UUID) (*model.WorkoutTemplate, error) {
				if uid != userID {
					t.Errorf("expected userID %s, got %s", userID, uid)
				}
				if tid != templateID {
					t.Errorf("expected templateID %s, got %s", templateID, tid)
				}
				templateRepoCalled = true
				return &model.WorkoutTemplate{
					ID:        templateID,
					UserID:    userID,
					Name:      "Push Template",
					TypeID:    "push",
					SubtypeID: "hypertrophy",
				}, nil
			},
		}

		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, templateRepo, v)

		_, err := svc.Create(ctx, userID, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !templateRepoCalled {
			t.Error("expected template repo GetByID to be called")
		}
	})

	t.Run("template ownership verification fails", func(t *testing.T) {
		templateID := uuid.New()
		req := model.CreateDayLogRequest{
			Date:       todayStr,
			TypeID:     "push",
			SubtypeID:  "hypertrophy",
			TemplateID: &templateID,
		}

		templateRepo := &MockTemplateDAO{
			GetByIDFunc: func(ctx context.Context, uid, tid uuid.UUID) (*model.WorkoutTemplate, error) {
				return nil, errors.New("template not owned or found")
			},
		}

		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, templateRepo, v)

		_, err := svc.Create(ctx, userID, req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("validation failure - validation rules", func(t *testing.T) {
		req := model.CreateDayLogRequest{
			Date:   "",
			TypeID: "push",
		}
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, req)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})

	t.Run("invalid date format", func(t *testing.T) {
		req := model.CreateDayLogRequest{
			Date:      "2026/06/01",
			TypeID:    "push",
			SubtypeID: "hypertrophy",
		}
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("future date rejection", func(t *testing.T) {
		tomorrowStr := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
		req := model.CreateDayLogRequest{
			Date:      tomorrowStr,
			TypeID:    "push",
			SubtypeID: "hypertrophy",
		}
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, req)
		if err == nil {
			t.Fatal("expected future date error, got nil")
		}
	})

	t.Run("invalid TypeID rejection", func(t *testing.T) {
		req := model.CreateDayLogRequest{
			Date:      todayStr,
			TypeID:    "invalid-type",
			SubtypeID: "hypertrophy",
		}
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid SubtypeID rejection", func(t *testing.T) {
		req := model.CreateDayLogRequest{
			Date:      todayStr,
			TypeID:    "push",
			SubtypeID: "invalid-subtype",
		}
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("rest day with template rejection", func(t *testing.T) {
		templateID := uuid.New()
		req := model.CreateDayLogRequest{
			Date:       todayStr,
			TypeID:     "rest",
			SubtypeID:  "general",
			TemplateID: &templateID,
		}
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, req)
		if err == nil {
			t.Fatal("expected rest day template rejection, got nil")
		}
		var valErr *model.ValidationError
		if !errors.As(err, &valErr) || valErr.Field != "template_id" {
			t.Errorf("expected ValidationError on template_id, got %v", err)
		}
	})

	t.Run("rest day with overrides rejection", func(t *testing.T) {
		req := model.CreateDayLogRequest{
			Date:      todayStr,
			TypeID:    "rest",
			SubtypeID: "general",
			Overrides: []model.CreateOverrideRequest{
				{ExerciseID: uuid.New(), Skipped: true},
			},
		}
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, req)
		if err == nil {
			t.Fatal("expected rest day overrides rejection, got nil")
		}
		var valErr *model.ValidationError
		if !errors.As(err, &valErr) || valErr.Field != "overrides" {
			t.Errorf("expected ValidationError on overrides, got %v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		repo := &MockLogDAO{
			CreateFunc: func(ctx context.Context, uid uuid.UUID, l *model.DayLog) error {
				return errors.New("db error")
			},
		}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, validReq)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("success with overrides", func(t *testing.T) {
		exerciseID := uuid.New()
		setsVal := 3
		repsVal := 10
		weightVal := 135.0
		notesVal := "Felt heavy"
		req := model.CreateDayLogRequest{
			Date:      todayStr,
			TypeID:    "push",
			SubtypeID: "hypertrophy",
			Overrides: []model.CreateOverrideRequest{
				{
					ExerciseID:   exerciseID,
					ActualSets:   &setsVal,
					ActualReps:   &repsVal,
					ActualWeight: &weightVal,
					Notes:        &notesVal,
					Skipped:      false,
				},
			},
		}
		createCalled := false
		repo := &MockLogDAO{
			CreateFunc: func(ctx context.Context, uid uuid.UUID, l *model.DayLog) error {
				if len(l.Overrides) != 1 {
					t.Errorf("expected 1 override, got %d", len(l.Overrides))
				} else {
					o := l.Overrides[0]
					if o.ExerciseID != exerciseID ||
						o.ActualSets == nil || *o.ActualSets != setsVal ||
						o.ActualReps == nil || *o.ActualReps != repsVal ||
						o.ActualWeight == nil || *o.ActualWeight != weightVal ||
						o.Notes == nil || *o.Notes != notesVal ||
						o.Skipped != false {
						t.Errorf("override fields do not match")
					}
				}
				createCalled = true
				return nil
			},
		}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Create(ctx, userID, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !createCalled {
			t.Error("expected create to be called")
		}
	})
}

func TestLogService_Update(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	todayStr := time.Now().Format("2006-01-02")
	notes := "Felt fantastic"
	validReq := model.UpdateDayLogRequest{
		SessionNotes: &notes,
	}

	t.Run("success", func(t *testing.T) {
		updateCalled := false
		repo := &MockLogDAO{
			UpdateFunc: func(ctx context.Context, uid uuid.UUID, date string, overrides []model.ExerciseOverride, notesParam *string, replace *model.LogReplacement) error {
				if uid != userID {
					t.Errorf("expected userID %s, got %s", userID, uid)
				}
				if date != todayStr {
					t.Errorf("expected date %s, got %s", todayStr, date)
				}
				if notesParam == nil || *notesParam != notes {
					t.Errorf("expected notes %s, got %v", notes, notesParam)
				}
				updateCalled = true
				return nil
			},
			GetByDateFunc: func(ctx context.Context, uid uuid.UUID, date string) (*model.DayLog, error) {
				return &model.DayLog{
					Date:         date,
					TypeID:       "push",
					SubtypeID:    "hypertrophy",
					SessionNotes: &notes,
				}, nil
			},
		}

		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		log, err := svc.Update(ctx, userID, todayStr, validReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updateCalled {
			t.Error("expected update to be called")
		}
		if log.SessionNotes == nil || *log.SessionNotes != notes {
			t.Errorf("expected updated session notes %s", notes)
		}
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Update(ctx, userID, "invalid-date", validReq)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("db error", func(t *testing.T) {
		repo := &MockLogDAO{
			UpdateFunc: func(ctx context.Context, uid uuid.UUID, date string, overrides []model.ExerciseOverride, notesParam *string, replace *model.LogReplacement) error {
				return errors.New("db error")
			},
		}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		_, err := svc.Update(ctx, userID, todayStr, validReq)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLogService_Delete(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	todayStr := time.Now().Format("2006-01-02")

	t.Run("success", func(t *testing.T) {
		deleteCalled := false
		repo := &MockLogDAO{
			DeleteFunc: func(ctx context.Context, uid uuid.UUID, date string) error {
				if uid != userID {
					t.Errorf("expected userID %s, got %s", userID, uid)
				}
				if date != todayStr {
					t.Errorf("expected date %s, got %s", todayStr, date)
				}
				deleteCalled = true
				return nil
			},
		}

		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		err := svc.Delete(ctx, userID, todayStr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !deleteCalled {
			t.Error("expected delete to be called")
		}
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &MockLogDAO{}
		v := validator.New()
		svc := NewLogService(repo, &MockTemplateDAO{}, v)

		err := svc.Delete(ctx, userID, "invalid-date")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
