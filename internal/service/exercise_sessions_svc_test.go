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

func TestLogService_ExerciseSessions(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	exerciseID, catalogID := uuid.New(), uuid.New()

	t.Run("passes the parsed lift and page to the DAO", func(t *testing.T) {
		before := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		tests := []struct {
			name   string
			params ExerciseSessionsParams
			want   model.ExerciseSessionsFilter
		}{
			{
				name:   "exercise id only, default limit",
				params: ExerciseSessionsParams{ExerciseID: exerciseID.String()},
				want:   model.ExerciseSessionsFilter{ExerciseID: &exerciseID, Limit: 20},
			},
			{
				name:   "name is trimmed and lower-cased",
				params: ExerciseSessionsParams{Name: "  Back Squat "},
				want:   model.ExerciseSessionsFilter{Name: "back squat", Limit: 20},
			},
			{
				name:   "every value",
				params: ExerciseSessionsParams{ExerciseID: " " + exerciseID.String(), CatalogID: catalogID.String(), Name: "Bench", Before: "2026-10-01", Limit: "50"},
				want:   model.ExerciseSessionsFilter{ExerciseID: &exerciseID, CatalogID: &catalogID, Name: "bench", Before: &before, Limit: 50},
			},
			{
				name:   "smallest page",
				params: ExerciseSessionsParams{CatalogID: catalogID.String(), Limit: "1"},
				want:   model.ExerciseSessionsFilter{CatalogID: &catalogID, Limit: 1},
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				page := &model.ExerciseSessionsPage{Sessions: []model.ExerciseSession{{Date: "2026-09-30"}}}
				repo := &MockLogDAO{ExerciseSessionsFunc: func(_ context.Context, uid uuid.UUID, filter model.ExerciseSessionsFilter) (*model.ExerciseSessionsPage, error) {
					if uid != userID {
						t.Errorf("user = %s, want %s", uid, userID)
					}
					if !reflect.DeepEqual(filter, tc.want) {
						t.Errorf("filter = %+v, want %+v", filter, tc.want)
					}
					return page, nil
				}}
				got, err := newLogSvcWith(repo, &MockTemplateDAO{}).ExerciseSessions(ctx, userID, tc.params)
				if err != nil {
					t.Fatalf("ExerciseSessions: %v", err)
				}
				if got != page {
					t.Errorf("ExerciseSessions = %+v, want the DAO page", got)
				}
			})
		}
	})

	t.Run("invalid input is refused before the DAO", func(t *testing.T) {
		tests := []struct {
			name   string
			params ExerciseSessionsParams
			field  string
		}{
			{name: "no lift named", params: ExerciseSessionsParams{}, field: "exercise_id"},
			{name: "blank name only", params: ExerciseSessionsParams{Name: "   "}, field: "exercise_id"},
			{name: "malformed exercise id", params: ExerciseSessionsParams{ExerciseID: "nope"}, field: "exercise_id"},
			{name: "malformed catalog id", params: ExerciseSessionsParams{CatalogID: "nope"}, field: "catalog_id"},
			{name: "name too long", params: ExerciseSessionsParams{Name: strings.Repeat("a", 201)}, field: "name"},
			{name: "malformed before", params: ExerciseSessionsParams{Name: "squat", Before: "10/01/2026"}, field: "before"},
			{name: "limit zero", params: ExerciseSessionsParams{Name: "squat", Limit: "0"}, field: "limit"},
			{name: "limit above the cap", params: ExerciseSessionsParams{Name: "squat", Limit: "51"}, field: "limit"},
			{name: "limit not a number", params: ExerciseSessionsParams{Name: "squat", Limit: "many"}, field: "limit"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				repo := &MockLogDAO{ExerciseSessionsFunc: func(context.Context, uuid.UUID, model.ExerciseSessionsFilter) (*model.ExerciseSessionsPage, error) {
					t.Error("DAO called for invalid input")
					return nil, nil
				}}
				_, err := newLogSvcWith(repo, &MockTemplateDAO{}).ExerciseSessions(ctx, userID, tc.params)
				var validation *model.ValidationError
				if !errors.As(err, &validation) || validation.Field != tc.field {
					t.Errorf("err = %v, want a validation error for field %q", err, tc.field)
				}
			})
		}
	})

	t.Run("a name of exactly 200 characters is accepted", func(t *testing.T) {
		if _, err := newLogSvcWith(&MockLogDAO{}, &MockTemplateDAO{}).ExerciseSessions(ctx, userID, ExerciseSessionsParams{Name: strings.Repeat("é", 200)}); err != nil {
			t.Errorf("ExerciseSessions: %v", err)
		}
	})

	t.Run("DAO failure is returned", func(t *testing.T) {
		boom := errors.New("database unavailable")
		repo := &MockLogDAO{ExerciseSessionsFunc: func(context.Context, uuid.UUID, model.ExerciseSessionsFilter) (*model.ExerciseSessionsPage, error) {
			return nil, boom
		}}
		if _, err := newLogSvcWith(repo, &MockTemplateDAO{}).ExerciseSessions(ctx, userID, ExerciseSessionsParams{Name: "squat"}); !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}
