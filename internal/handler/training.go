package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type TrainingProfileHandler struct {
	svc service.TrainingProfileService
}
type ProgramHandler struct{ svc service.ProgramService }
type ScheduleHandler struct{ svc service.ScheduleService }
type WorkoutSessionHandler struct{ svc service.WorkoutSessionService }
type ParticipationHandler struct{ svc service.ParticipationService }

func NewTrainingProfileHandler(svc service.TrainingProfileService) *TrainingProfileHandler {
	return &TrainingProfileHandler{svc: svc}
}

func NewProgramHandler(svc service.ProgramService) *ProgramHandler { return &ProgramHandler{svc: svc} }
func NewScheduleHandler(svc service.ScheduleService) *ScheduleHandler {
	return &ScheduleHandler{svc: svc}
}
func NewWorkoutSessionHandler(svc service.WorkoutSessionService) *WorkoutSessionHandler {
	return &WorkoutSessionHandler{svc: svc}
}
func NewParticipationHandler(svc service.ParticipationService) *ParticipationHandler {
	return &ParticipationHandler{svc: svc}
}

func (h *TrainingProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	resource, err := h.svc.Get(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, resource, err, http.StatusOK)
}

func (h *TrainingProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req model.UpdateTrainingProfileRequest
	if !decodeMutation(w, r, &req, "") {
		return
	}
	resource, err := h.svc.Update(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resource, err, http.StatusOK)
}

func (h *ProgramHandler) ListStarters(w http.ResponseWriter, r *http.Request) {
	filter := model.StarterProgramFilter{PrimaryGoal: r.URL.Query().Get("primary_goal"), Experience: r.URL.Query().Get("experience")}
	if raw := r.URL.Query().Get("available_days"); raw != "" {
		filter.AvailableDays, _ = strconv.Atoi(raw)
	}
	if raw := r.URL.Query().Get("session_duration_minutes"); raw != "" {
		filter.SessionDurationMinutes, _ = strconv.Atoi(raw)
	}
	if raw := r.URL.Query().Get("equipment"); raw != "" {
		filter.Equipment = strings.Split(raw, ",")
	}
	programs, err := h.svc.ListStarters(r.Context(), filter)
	respond(w, map[string]any{"starter_programs": programs}, err, http.StatusOK)
}

func (h *ProgramHandler) List(w http.ResponseWriter, r *http.Request) {
	programs, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, map[string]any{"programs": programs}, err, http.StatusOK)
}

func (h *ProgramHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	resource, err := h.svc.Get(r.Context(), middleware.MustGetUserID(r.Context()), id)
	respond(w, resource, err, http.StatusOK)
}

func (h *ProgramHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateProgramRequest
	if !decodeMutation(w, r, &req, "") {
		return
	}
	resource, err := h.svc.Create(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resource, err, http.StatusCreated)
}

func (h *ProgramHandler) CloneStarter(w http.ResponseWriter, r *http.Request) {
	var req model.CloneStarterProgramRequest
	if !decodeMutation(w, r, &req, req.OperationKey) {
		return
	}
	if !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.CloneStarter(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resource, err, http.StatusCreated)
}

func (h *ProgramHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req model.UpdateProgramRequest
	if !decodeMutation(w, r, &req, "") {
		return
	}
	resource, err := h.svc.Update(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, resource, err, http.StatusOK)
}

func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	resources, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	respond(w, map[string]any{"scheduled_workouts": resources}, err, http.StatusOK)
}

func (h *ScheduleHandler) Materialize(w http.ResponseWriter, r *http.Request) {
	var req model.MaterializeScheduleRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resources, err := h.svc.Materialize(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, map[string]any{"scheduled_workouts": resources}, err, http.StatusCreated)
}

func (h *ScheduleHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	var req model.RegenerateScheduleRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.Regenerate(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resource, err, http.StatusOK)
}

func (h *ScheduleHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req model.PatchScheduledWorkoutRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.PatchWorkout(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, resource, err, http.StatusOK)
}

func (h *ScheduleHandler) PutSet(w http.ResponseWriter, r *http.Request) {
	workoutID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	setID, ok := pathUUID(w, r, "set_id")
	if !ok {
		return
	}
	var req model.SetMutationRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.PutRequiredSet(r.Context(), middleware.MustGetUserID(r.Context()), workoutID, setID, req)
	respond(w, resource, err, http.StatusOK)
}

func (h *ScheduleHandler) AddExtra(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req model.ExtraSetRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.AddExtraSet(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, resource, err, http.StatusCreated)
}

func (h *ScheduleHandler) Complete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req model.RevisionRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.Complete(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, resource, err, http.StatusOK)
}

func (h *WorkoutSessionHandler) List(w http.ResponseWriter, r *http.Request) {
	resources, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	respond(w, map[string]any{"workout_sessions": resources}, err, http.StatusOK)
}

func (h *WorkoutSessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	resource, err := h.svc.Get(r.Context(), middleware.MustGetUserID(r.Context()), id)
	respond(w, resource, err, http.StatusOK)
}

func (h *WorkoutSessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateWorkoutSessionRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.Create(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resource, err, http.StatusCreated)
}

func (h *WorkoutSessionHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req model.PatchWorkoutSessionRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.Patch(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, resource, err, http.StatusOK)
}

func (h *ParticipationHandler) List(w http.ResponseWriter, r *http.Request) {
	resources, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	respond(w, map[string]any{"participation": resources}, err, http.StatusOK)
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name, "BAD_REQUEST", nil)
		return uuid.Nil, false
	}
	return id, true
}

func decodeMutation(w http.ResponseWriter, r *http.Request, target any, _ string) bool {
	if err := decodeJSON(r, target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return false
	}
	return true
}

func matchIdempotencyKey(w http.ResponseWriter, r *http.Request, operationKey string) bool {
	key := r.Header.Get("Idempotency-Key")
	if key == "" || key != operationKey {
		writeError(w, http.StatusUnprocessableEntity, "Idempotency-Key must match operation_key", "VALIDATION_ERROR", map[string]string{"field": "Idempotency-Key"})
		return false
	}
	return true
}

func respond(w http.ResponseWriter, resource any, err error, status int) {
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, status, resource)
}
