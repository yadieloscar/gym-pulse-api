package service

import (
	"context"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func intPtr(n int) *int { return &n }

func asValidation(err error, target **model.ValidationError) bool {
	return errors.As(err, target)
}

func newLogSvcWith(repo *MockLogDAO, tmpl *MockTemplateDAO) LogService {
	return NewLogService(repo, tmpl, validator.New())
}

func TestLogService_Create_withSetLogs(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	exID := uuid.New()
	pastDate := "2024-01-15"

	t.Run("persists completed set logs", func(t *testing.T) {
		var captured []model.SetLog
		repo := &MockLogDAO{
			CreateFunc: func(ctx context.Context, uid uuid.UUID, l *model.DayLog) error {
				captured = l.SetLogs
				return nil
			},
		}
		svc := newLogSvcWith(repo, &MockTemplateDAO{})

		_, err := svc.Create(ctx, userID, model.CreateDayLogRequest{
			Date: pastDate, TypeID: "push", SubtypeID: "hypertrophy",
			SetLogs: []model.CreateSetLogRequest{
				{ExerciseID: exID, SetIndex: 1, ActualReps: intPtr(8), Completed: true},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(captured) != 1 || captured[0].SetIndex != 1 || !captured[0].Completed {
			t.Fatalf("set log not passed through to repo: %+v", captured)
		}
	})

	t.Run("rejects completed set without reps or duration", func(t *testing.T) {
		svc := newLogSvcWith(&MockLogDAO{}, &MockTemplateDAO{})
		_, err := svc.Create(ctx, userID, model.CreateDayLogRequest{
			Date: pastDate, TypeID: "push", SubtypeID: "hypertrophy",
			SetLogs: []model.CreateSetLogRequest{{ExerciseID: exID, SetIndex: 1, Completed: true}},
		})
		var verr *model.ValidationError
		if !asValidation(err, &verr) {
			t.Fatalf("expected ValidationError, got %v", err)
		}
	})

	t.Run("rejects set_index below 1", func(t *testing.T) {
		svc := newLogSvcWith(&MockLogDAO{}, &MockTemplateDAO{})
		_, err := svc.Create(ctx, userID, model.CreateDayLogRequest{
			Date: pastDate, TypeID: "push", SubtypeID: "hypertrophy",
			SetLogs: []model.CreateSetLogRequest{{ExerciseID: exID, SetIndex: 0, ActualReps: intPtr(8), Completed: true}},
		})
		var verr *model.ValidationError
		if !asValidation(err, &verr) {
			t.Fatalf("expected ValidationError, got %v", err)
		}
	})

	t.Run("rejects set logs on a rest day", func(t *testing.T) {
		svc := newLogSvcWith(&MockLogDAO{}, &MockTemplateDAO{})
		_, err := svc.Create(ctx, userID, model.CreateDayLogRequest{
			Date: pastDate, TypeID: "rest", SubtypeID: "general",
			SetLogs: []model.CreateSetLogRequest{{ExerciseID: exID, SetIndex: 1, ActualReps: intPtr(8), Completed: true}},
		})
		var verr *model.ValidationError
		if !asValidation(err, &verr) {
			t.Fatalf("expected ValidationError, got %v", err)
		}
	})
}

func TestLogService_ExerciseRecords(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	ex := uuid.New()

	t.Run("reduces to heaviest weight and best e1rm", func(t *testing.T) {
		repo := &MockLogDAO{
			RecordSetsFunc: func(ctx context.Context, uid uuid.UUID, ids []uuid.UUID) ([]model.SetPerf, error) {
				return []model.SetPerf{
					{ExerciseID: ex, Weight: 135, Reps: 5, Date: "2026-06-01"},
					{ExerciseID: ex, Weight: 185, Reps: 1, Date: "2026-06-05"}, // heaviest
					{ExerciseID: ex, Weight: 155, Reps: 8, Date: "2026-06-10"}, // best e1rm
				}, nil
			},
		}
		svc := newLogSvcWith(repo, &MockTemplateDAO{})

		recs, err := svc.ExerciseRecords(ctx, userID, ex.String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(recs) != 1 {
			t.Fatalf("expected 1 record, got %d", len(recs))
		}
		r := recs[0]
		if r.MaxWeight == nil || *r.MaxWeight != 185 {
			t.Errorf("max weight: got %v want 185", r.MaxWeight)
		}
		// e1rm: 155*(1+8/30)=196.3 beats 185*(1+1/30)=191.2 and 135*(1+5/30)=157.5
		if r.E1RMWeight == nil || *r.E1RMWeight != 155 || r.E1RMReps == nil || *r.E1RMReps != 8 {
			t.Errorf("best e1rm set: got %v×%v want 155×8", r.E1RMWeight, r.E1RMReps)
		}
	})

	t.Run("omits exercises with no sets", func(t *testing.T) {
		svc := newLogSvcWith(&MockLogDAO{}, &MockTemplateDAO{})
		recs, err := svc.ExerciseRecords(ctx, userID, ex.String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(recs) != 0 {
			t.Fatalf("expected 0 records, got %d", len(recs))
		}
	})

	t.Run("rejects an invalid id", func(t *testing.T) {
		svc := newLogSvcWith(&MockLogDAO{}, &MockTemplateDAO{})
		_, err := svc.ExerciseRecords(ctx, userID, "nope")
		var verr *model.ValidationError
		if !asValidation(err, &verr) {
			t.Fatalf("expected ValidationError, got %v", err)
		}
	})
}

func TestLogService_ExerciseHistory(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	id1, id2 := uuid.New(), uuid.New()

	t.Run("parses ids and forwards to repo", func(t *testing.T) {
		var got []uuid.UUID
		repo := &MockLogDAO{
			ExerciseHistoryFunc: func(ctx context.Context, uid uuid.UUID, ids []uuid.UUID) ([]model.ExerciseHistory, error) {
				got = ids
				return []model.ExerciseHistory{{ExerciseID: id1}}, nil
			},
		}
		svc := newLogSvcWith(repo, &MockTemplateDAO{})

		res, err := svc.ExerciseHistory(ctx, userID, id1.String()+" , "+id2.String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0] != id1 || got[1] != id2 {
			t.Fatalf("ids not parsed/forwarded: %v", got)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 history row, got %d", len(res))
		}
	})

	t.Run("rejects an invalid id", func(t *testing.T) {
		svc := newLogSvcWith(&MockLogDAO{}, &MockTemplateDAO{})
		_, err := svc.ExerciseHistory(ctx, userID, "not-a-uuid")
		var verr *model.ValidationError
		if !asValidation(err, &verr) {
			t.Fatalf("expected ValidationError, got %v", err)
		}
	})

	t.Run("empty param yields no ids", func(t *testing.T) {
		var got []uuid.UUID
		repo := &MockLogDAO{
			ExerciseHistoryFunc: func(ctx context.Context, uid uuid.UUID, ids []uuid.UUID) ([]model.ExerciseHistory, error) {
				got = ids
				return []model.ExerciseHistory{}, nil
			},
		}
		svc := newLogSvcWith(repo, &MockTemplateDAO{})
		if _, err := svc.ExerciseHistory(ctx, userID, "  "); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected 0 ids, got %d", len(got))
		}
	})
}
