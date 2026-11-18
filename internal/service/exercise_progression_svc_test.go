package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestLogService_ExerciseProgression(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	squat, bench, unused := uuid.New(), uuid.New(), uuid.New()
	squatWeeks := []model.ProgressionWeek{
		{WeekStart: "2026-08-24", MaxWeight: 200, MaxWeightReps: 5, Date: "2026-08-26"},
		{WeekStart: "2026-08-31", MaxWeight: 205, MaxWeightReps: 5, Date: "2026-09-02"},
	}
	benchWeeks := []model.ProgressionWeek{{WeekStart: "2026-08-31", MaxWeight: 135, MaxWeightReps: 8, Date: "2026-09-01"}}

	t.Run("returns exercises in requested order and omits those without sets", func(t *testing.T) {
		repo := &MockLogDAO{WeeklyHeaviestSetsFunc: func(_ context.Context, uid uuid.UUID, ids []uuid.UUID, _ time.Time) ([]model.ExerciseProgression, error) {
			if uid != userID {
				t.Errorf("user = %s, want %s", uid, userID)
			}
			if want := []uuid.UUID{bench, unused, squat, bench}; !reflect.DeepEqual(ids, want) {
				t.Errorf("ids = %v, want %v", ids, want)
			}
			// The DAO groups by exercise id, not by request order.
			return []model.ExerciseProgression{
				{ExerciseID: squat, Weeks: squatWeeks},
				{ExerciseID: bench, Weeks: benchWeeks},
			}, nil
		}}
		svc := newLogSvcWith(repo, &MockTemplateDAO{})

		got, err := svc.ExerciseProgression(ctx, userID, strings.Join([]string{bench.String(), unused.String(), squat.String(), bench.String()}, ", "), "")
		if err != nil {
			t.Fatalf("ExerciseProgression: %v", err)
		}
		want := []model.ExerciseProgression{
			{ExerciseID: bench, Weeks: benchWeeks},
			{ExerciseID: squat, Weeks: squatWeeks},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ExerciseProgression = %+v, want %+v", got, want)
		}
	})

	t.Run("window starts on the Monday weeks-1 weeks before this week", func(t *testing.T) {
		tests := []struct {
			weeksParam string
			weeks      int
		}{
			{weeksParam: "", weeks: 12},
			{weeksParam: "1", weeks: 1},
			{weeksParam: "52", weeks: 52},
		}
		for _, tc := range tests {
			var since time.Time
			repo := &MockLogDAO{WeeklyHeaviestSetsFunc: func(_ context.Context, _ uuid.UUID, _ []uuid.UUID, s time.Time) ([]model.ExerciseProgression, error) {
				since = s
				return []model.ExerciseProgression{}, nil
			}}
			svc := newLogSvcWith(repo, &MockTemplateDAO{})

			before := model.MondayOfWeek(time.Now())
			got, err := svc.ExerciseProgression(ctx, userID, squat.String(), tc.weeksParam)
			after := model.MondayOfWeek(time.Now())
			if err != nil {
				t.Fatalf("weeks=%q: %v", tc.weeksParam, err)
			}
			if got == nil || len(got) != 0 {
				t.Errorf("weeks=%q: got %v, want an empty non-nil list", tc.weeksParam, got)
			}
			const day = "2006-01-02"
			wantEarly := before.AddDate(0, 0, -7*(tc.weeks-1)).Format(day)
			wantLate := after.AddDate(0, 0, -7*(tc.weeks-1)).Format(day)
			if s := since.Format(day); s != wantEarly && s != wantLate {
				t.Errorf("weeks=%q: since = %s, want %s", tc.weeksParam, s, wantEarly)
			}
			if since.Weekday() != time.Monday {
				t.Errorf("weeks=%q: since is a %s, want Monday", tc.weeksParam, since.Weekday())
			}
		}
	})

	t.Run("empty ids return an empty list without a query", func(t *testing.T) {
		repo := &MockLogDAO{WeeklyHeaviestSetsFunc: func(context.Context, uuid.UUID, []uuid.UUID, time.Time) ([]model.ExerciseProgression, error) {
			t.Error("WeeklyHeaviestSets called for empty ids")
			return nil, nil
		}}
		svc := newLogSvcWith(repo, &MockTemplateDAO{})
		for _, ids := range []string{"", " , "} {
			got, err := svc.ExerciseProgression(ctx, userID, ids, "")
			if err != nil {
				t.Fatalf("ids=%q: %v", ids, err)
			}
			if got == nil || len(got) != 0 {
				t.Errorf("ids=%q: got %v, want an empty non-nil list", ids, got)
			}
		}
	})

	t.Run("rejects invalid input with the offending field", func(t *testing.T) {
		tooMany := make([]string, MaxExerciseIDsPerRequest+1)
		for i := range tooMany {
			tooMany[i] = uuid.NewString()
		}
		tests := []struct {
			name, ids, weeks, field string
		}{
			{name: "malformed id", ids: "nope", weeks: "", field: "ids"},
			{name: "too many ids", ids: strings.Join(tooMany, ","), weeks: "", field: "ids"},
			{name: "weeks zero", ids: squat.String(), weeks: "0", field: "weeks"},
			{name: "weeks negative", ids: squat.String(), weeks: "-1", field: "weeks"},
			{name: "weeks above max", ids: squat.String(), weeks: "53", field: "weeks"},
			{name: "weeks not a number", ids: squat.String(), weeks: "twelve", field: "weeks"},
			{name: "weeks fractional", ids: squat.String(), weeks: "1.5", field: "weeks"},
			{name: "bad weeks with empty ids", ids: "", weeks: "0", field: "weeks"},
		}
		for _, tc := range tests {
			repo := &MockLogDAO{WeeklyHeaviestSetsFunc: func(context.Context, uuid.UUID, []uuid.UUID, time.Time) ([]model.ExerciseProgression, error) {
				t.Errorf("%s: WeeklyHeaviestSets called for invalid input", tc.name)
				return nil, nil
			}}
			_, err := newLogSvcWith(repo, &MockTemplateDAO{}).ExerciseProgression(ctx, userID, tc.ids, tc.weeks)
			var verr *model.ValidationError
			if !errors.As(err, &verr) {
				t.Errorf("%s: err = %v, want ValidationError", tc.name, err)
				continue
			}
			if verr.Field != tc.field {
				t.Errorf("%s: field = %q, want %q", tc.name, verr.Field, tc.field)
			}
		}
	})

	t.Run("passes a repository failure through", func(t *testing.T) {
		boom := errors.New("database unavailable")
		repo := &MockLogDAO{WeeklyHeaviestSetsFunc: func(context.Context, uuid.UUID, []uuid.UUID, time.Time) ([]model.ExerciseProgression, error) {
			return nil, boom
		}}
		if _, err := newLogSvcWith(repo, &MockTemplateDAO{}).ExerciseProgression(ctx, userID, squat.String(), "4"); !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}
