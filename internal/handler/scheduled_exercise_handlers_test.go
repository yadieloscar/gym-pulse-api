package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestSwapAndSkipExerciseHandlers(t *testing.T) {
	userID, key := uuid.New(), "exercise-change-1"
	schedule := NewScheduleHandler(guidedScheduleService{})
	params := map[string]string{"id": uuid.NewString(), "exercise_order": "2"}
	swap := model.SwapScheduledExerciseRequest{OperationKey: key, ExpectedRevision: 1, ExerciseName: "Push-Up", ExerciseCategory: "push"}
	skip := model.RevisionRequest{OperationKey: key, ExpectedRevision: 1}

	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		schedule.SwapExercise(w, guidedHandlerRequest(t, http.MethodPost, "/", swap, userID, key, params))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		schedule.SkipExercise(w, guidedHandlerRequest(t, http.MethodPost, "/", skip, userID, key, params))
	})

	for _, bad := range []map[string]string{
		{"id": "not-a-uuid", "exercise_order": "2"},
		{"id": uuid.NewString(), "exercise_order": "0"},
		{"id": uuid.NewString(), "exercise_order": "two"},
	} {
		assertGuidedStatus(t, http.StatusBadRequest, func(w http.ResponseWriter) {
			schedule.SwapExercise(w, guidedHandlerRequest(t, http.MethodPost, "/", swap, userID, key, bad))
		})
		assertGuidedStatus(t, http.StatusBadRequest, func(w http.ResponseWriter) {
			schedule.SkipExercise(w, guidedHandlerRequest(t, http.MethodPost, "/", skip, userID, key, bad))
		})
	}

	// The header key must match the body key, as on every training mutation.
	assertGuidedStatus(t, http.StatusUnprocessableEntity, func(w http.ResponseWriter) {
		schedule.SwapExercise(w, guidedHandlerRequest(t, http.MethodPost, "/", swap, userID, "another-key", params))
	})
	assertGuidedStatus(t, http.StatusUnprocessableEntity, func(w http.ResponseWriter) {
		schedule.SkipExercise(w, guidedHandlerRequest(t, http.MethodPost, "/", skip, userID, "another-key", params))
	})
}
