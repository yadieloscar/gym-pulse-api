package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestScheduleServiceDeleteAllowsOnlyPastOrFinishedWorkouts(t *testing.T) {
	ctx := context.Background()
	finalized := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		workout   model.ScheduledWorkout
		wantError bool
	}{
		{name: "past unfinalized", workout: model.ScheduledWorkout{Date: "2026-07-19", Status: model.WorkoutStatusPlanned}},
		{name: "past missed", workout: model.ScheduledWorkout{Date: "2026-07-18", Status: model.WorkoutStatusMissed, FinalizedAt: &finalized}},
		{name: "today finished", workout: model.ScheduledWorkout{Date: "2026-07-20", Status: model.WorkoutStatusCompleted, FinalizedAt: &finalized}},
		{name: "today unfinished", workout: model.ScheduledWorkout{Date: "2026-07-20", Status: model.WorkoutStatusPlanned}, wantError: true},
		{name: "future planned", workout: model.ScheduledWorkout{Date: "2026-07-22", Status: model.WorkoutStatusPlanned}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userID, _, schedules, _, _ := newReleaseScheduleService(t)
			tt.workout.ID = uuid.New()
			tt.workout.Name = "Lower"
			tt.workout.Revision = 1
			schedules.workouts[tt.workout.ID] = tt.workout

			err := svc.Delete(ctx, userID, tt.workout.ID)
			_, stillThere := schedules.workouts[tt.workout.ID]
			if tt.wantError {
				var conflict *model.ConflictError
				if !errors.As(err, &conflict) {
					t.Fatalf("Delete() error = %v, want ConflictError", err)
				}
				if !stillThere {
					t.Fatal("rejected delete removed the workout")
				}
				return
			}
			if err != nil {
				t.Fatalf("Delete() error = %v", err)
			}
			if stillThere {
				t.Fatal("Delete() left the workout in place")
			}
		})
	}
}

func TestScheduleServiceDeleteMissingWorkoutIsNotFound(t *testing.T) {
	svc, userID, _, _, _, _ := newReleaseScheduleService(t)
	var notFound *model.NotFoundError
	if err := svc.Delete(context.Background(), userID, uuid.New()); !errors.As(err, &notFound) {
		t.Fatalf("Delete() error = %v, want NotFoundError", err)
	}
}
