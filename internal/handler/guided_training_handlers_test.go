package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type guidedProfileService struct{}

func (guidedProfileService) Get(context.Context, uuid.UUID) (*model.TrainingProfile, error) {
	return &model.TrainingProfile{}, nil
}
func (guidedProfileService) Update(context.Context, uuid.UUID, model.UpdateTrainingProfileRequest) (*model.TrainingProfile, error) {
	return &model.TrainingProfile{}, nil
}

type guidedProgramService struct{}

func (guidedProgramService) ListStarters(context.Context, model.StarterProgramFilter) ([]model.StarterProgram, error) {
	return []model.StarterProgram{}, nil
}
func (guidedProgramService) List(context.Context, uuid.UUID) ([]model.Program, error) {
	return []model.Program{}, nil
}
func (guidedProgramService) Get(context.Context, uuid.UUID, uuid.UUID) (*model.Program, error) {
	return &model.Program{}, nil
}
func (guidedProgramService) Create(context.Context, uuid.UUID, model.CreateProgramRequest) (*model.Program, error) {
	return &model.Program{}, nil
}
func (guidedProgramService) CloneStarter(context.Context, uuid.UUID, model.CloneStarterProgramRequest) (*model.Program, error) {
	return &model.Program{}, nil
}
func (guidedProgramService) Update(context.Context, uuid.UUID, uuid.UUID, model.UpdateProgramRequest) (*model.Program, error) {
	return &model.Program{}, nil
}

type guidedScheduleService struct{}

func (guidedScheduleService) List(context.Context, uuid.UUID, string, string) ([]model.ScheduledWorkout, error) {
	return []model.ScheduledWorkout{}, nil
}
func (guidedScheduleService) Materialize(context.Context, uuid.UUID, model.MaterializeScheduleRequest) ([]model.ScheduledWorkout, error) {
	return []model.ScheduledWorkout{}, nil
}
func (guidedScheduleService) Regenerate(context.Context, uuid.UUID, model.RegenerateScheduleRequest) (*model.RegenerateScheduleResponse, error) {
	return &model.RegenerateScheduleResponse{}, nil
}
func (guidedScheduleService) PatchWorkout(context.Context, uuid.UUID, uuid.UUID, model.PatchScheduledWorkoutRequest) (*model.ScheduledWorkout, error) {
	return &model.ScheduledWorkout{}, nil
}
func (guidedScheduleService) PatchSetTarget(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.PatchScheduledSetTargetRequest) (*model.ScheduledWorkout, error) {
	return &model.ScheduledWorkout{}, nil
}
func (guidedScheduleService) RecoverToday(context.Context, uuid.UUID, model.RecoverScheduledWorkoutRequest) (*model.ScheduledWorkout, error) {
	return &model.ScheduledWorkout{}, nil
}
func (guidedScheduleService) PutRequiredSet(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.SetMutationRequest) (*model.ScheduledWorkout, error) {
	return &model.ScheduledWorkout{}, nil
}
func (guidedScheduleService) AddExtraSet(context.Context, uuid.UUID, uuid.UUID, model.ExtraSetRequest) (*model.ScheduledWorkout, error) {
	return &model.ScheduledWorkout{}, nil
}
func (guidedScheduleService) Complete(context.Context, uuid.UUID, uuid.UUID, model.RevisionRequest) (*model.ScheduledWorkout, error) {
	return &model.ScheduledWorkout{}, nil
}

type guidedSessionService struct{}

func (guidedSessionService) List(context.Context, uuid.UUID, string, string) ([]model.WorkoutSession, error) {
	return []model.WorkoutSession{}, nil
}
func (guidedSessionService) Get(context.Context, uuid.UUID, uuid.UUID) (*model.WorkoutSession, error) {
	return &model.WorkoutSession{}, nil
}
func (guidedSessionService) Create(context.Context, uuid.UUID, model.CreateWorkoutSessionRequest) (*model.WorkoutSession, error) {
	return &model.WorkoutSession{}, nil
}
func (guidedSessionService) Patch(context.Context, uuid.UUID, uuid.UUID, model.PatchWorkoutSessionRequest) (*model.WorkoutSession, error) {
	return &model.WorkoutSession{}, nil
}

type guidedParticipationService struct{}

func (guidedParticipationService) List(context.Context, uuid.UUID, string, string) ([]model.DayParticipation, error) {
	return []model.DayParticipation{}, nil
}

type guidedTransitionService struct{}

func (guidedTransitionService) Preview(context.Context, uuid.UUID, model.PreviewPlanTransitionRequest) (*model.PlanTransitionPreview, error) {
	return &model.PlanTransitionPreview{}, nil
}
func (guidedTransitionService) Apply(context.Context, uuid.UUID, model.ApplyPlanTransitionRequest) (*model.PlanTransitionPreview, error) {
	return &model.PlanTransitionPreview{}, nil
}

func guidedHandlerRequest(t *testing.T, method, target string, body any, userID uuid.UUID, operationKey string, params map[string]string) *http.Request {
	t.Helper()
	req := newReq(t, method, target, body, userID)
	if operationKey != "" {
		req.Header.Set("Idempotency-Key", operationKey)
	}
	if len(params) == 0 {
		return req
	}
	routeContext := chi.NewRouteContext()
	for key, value := range params {
		routeContext.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
}

func assertGuidedStatus(t *testing.T, want int, invoke func(http.ResponseWriter)) {
	t.Helper()
	recorder := httptest.NewRecorder()
	invoke(recorder)
	if recorder.Code != want {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, want, recorder.Body.String())
	}
}

func TestGuidedTrainingHandlersSuccessPaths(t *testing.T) {
	userID, resourceID, setID := uuid.New(), uuid.New(), uuid.New()
	params := map[string]string{"id": resourceID.String()}
	setParams := map[string]string{"id": resourceID.String(), "set_id": setID.String()}
	const operationKey = "guided-handler-op"

	profile := NewTrainingProfileHandler(guidedProfileService{})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		profile.Get(w, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		profile.Update(w, guidedHandlerRequest(t, http.MethodPut, "/", model.UpdateTrainingProfileRequest{}, userID, "", nil))
	})

	program := NewProgramHandler(guidedProgramService{})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		program.ListStarters(w, guidedHandlerRequest(t, http.MethodGet, "/?primary_goal=strength&experience=beginner&available_days=3&available_weekdays=1,bad,3&usual_activity=light&session_duration_minutes=45&equipment=dumbbell,bench", nil, userID, "", nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		program.List(w, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		program.Get(w, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", params))
	})
	assertGuidedStatus(t, http.StatusCreated, func(w http.ResponseWriter) {
		program.Create(w, guidedHandlerRequest(t, http.MethodPost, "/", model.CreateProgramRequest{}, userID, "", nil))
	})
	assertGuidedStatus(t, http.StatusCreated, func(w http.ResponseWriter) {
		program.CloneStarter(w, guidedHandlerRequest(t, http.MethodPost, "/", model.CloneStarterProgramRequest{OperationKey: operationKey}, userID, operationKey, nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		program.Update(w, guidedHandlerRequest(t, http.MethodPut, "/", model.UpdateProgramRequest{}, userID, "", params))
	})

	schedule := NewScheduleHandler(guidedScheduleService{})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		schedule.List(w, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-07-20&to=2026-07-26", nil, userID, "", nil))
	})
	assertGuidedStatus(t, http.StatusCreated, func(w http.ResponseWriter) {
		schedule.Materialize(w, guidedHandlerRequest(t, http.MethodPost, "/", model.MaterializeScheduleRequest{OperationKey: operationKey}, userID, operationKey, nil))
	})
	assertGuidedStatus(t, http.StatusCreated, func(w http.ResponseWriter) {
		schedule.Recover(w, guidedHandlerRequest(t, http.MethodPost, "/", model.RecoverScheduledWorkoutRequest{OperationKey: operationKey}, userID, operationKey, nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		schedule.Regenerate(w, guidedHandlerRequest(t, http.MethodPost, "/", model.RegenerateScheduleRequest{OperationKey: operationKey}, userID, operationKey, nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		schedule.Patch(w, guidedHandlerRequest(t, http.MethodPatch, "/", model.PatchScheduledWorkoutRequest{OperationKey: operationKey}, userID, operationKey, params))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		schedule.PutSet(w, guidedHandlerRequest(t, http.MethodPut, "/", model.SetMutationRequest{OperationKey: operationKey}, userID, operationKey, setParams))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		schedule.PatchSetTarget(w, guidedHandlerRequest(t, http.MethodPatch, "/", model.PatchScheduledSetTargetRequest{OperationKey: operationKey}, userID, operationKey, setParams))
	})
	assertGuidedStatus(t, http.StatusCreated, func(w http.ResponseWriter) {
		schedule.AddExtra(w, guidedHandlerRequest(t, http.MethodPost, "/", model.ExtraSetRequest{OperationKey: operationKey}, userID, operationKey, params))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		schedule.Complete(w, guidedHandlerRequest(t, http.MethodPost, "/", model.RevisionRequest{OperationKey: operationKey}, userID, operationKey, params))
	})

	sessions := NewWorkoutSessionHandler(guidedSessionService{})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		sessions.List(w, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-07-20&to=2026-07-26", nil, userID, "", nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		sessions.Get(w, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", params))
	})
	assertGuidedStatus(t, http.StatusCreated, func(w http.ResponseWriter) {
		sessions.Create(w, guidedHandlerRequest(t, http.MethodPost, "/", model.CreateWorkoutSessionRequest{OperationKey: operationKey}, userID, operationKey, nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		sessions.Patch(w, guidedHandlerRequest(t, http.MethodPatch, "/", model.PatchWorkoutSessionRequest{OperationKey: operationKey}, userID, operationKey, params))
	})

	participation := NewParticipationHandler(guidedParticipationService{})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		participation.List(w, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-07-20&to=2026-07-26", nil, userID, "", nil))
	})

	transition := NewPlanTransitionHandler(guidedTransitionService{})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		transition.Preview(w, guidedHandlerRequest(t, http.MethodPost, "/", model.PreviewPlanTransitionRequest{}, userID, "", nil))
	})
	assertGuidedStatus(t, http.StatusOK, func(w http.ResponseWriter) {
		transition.Apply(w, guidedHandlerRequest(t, http.MethodPost, "/", model.ApplyPlanTransitionRequest{OperationKey: operationKey}, userID, operationKey, nil))
	})
}

func TestGuidedTrainingHandlerInputFailures(t *testing.T) {
	userID := uuid.New()
	profile := NewTrainingProfileHandler(guidedProfileService{})
	assertGuidedStatus(t, http.StatusBadRequest, func(w http.ResponseWriter) {
		profile.Update(w, guidedHandlerRequest(t, http.MethodPut, "/", "not-json", userID, "", nil))
	})
	program := NewProgramHandler(guidedProgramService{})
	assertGuidedStatus(t, http.StatusBadRequest, func(w http.ResponseWriter) {
		program.Get(w, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", map[string]string{"id": "bad"}))
	})
	schedule := NewScheduleHandler(guidedScheduleService{})
	assertGuidedStatus(t, http.StatusUnprocessableEntity, func(w http.ResponseWriter) {
		schedule.Materialize(w, guidedHandlerRequest(t, http.MethodPost, "/", model.MaterializeScheduleRequest{OperationKey: "body-key"}, userID, "header-key", nil))
	})

	recorder := httptest.NewRecorder()
	respond(recorder, nil, &model.NotFoundError{Message: "missing"}, http.StatusOK)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("respond error status=%d", recorder.Code)
	}

	request := guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", nil)
	if middleware.MustGetUserID(request.Context()) != userID {
		t.Fatal("authenticated user was not propagated")
	}
}
