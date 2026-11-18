package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type ScheduleService interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.ScheduledWorkout, error)
	Materialize(ctx context.Context, userID uuid.UUID, req model.MaterializeScheduleRequest) ([]model.ScheduledWorkout, error)
	Regenerate(ctx context.Context, userID uuid.UUID, req model.RegenerateScheduleRequest) (*model.RegenerateScheduleResponse, error)
	PatchWorkout(ctx context.Context, userID, workoutID uuid.UUID, req model.PatchScheduledWorkoutRequest) (*model.ScheduledWorkout, error)
	PatchSetTarget(ctx context.Context, userID, workoutID, setID uuid.UUID, req model.PatchScheduledSetTargetRequest) (*model.ScheduledWorkout, error)
	SwapExercise(ctx context.Context, userID, workoutID uuid.UUID, exerciseOrder int, req model.SwapScheduledExerciseRequest) (*model.ScheduledWorkout, error)
	SkipExercise(ctx context.Context, userID, workoutID uuid.UUID, exerciseOrder int, req model.RevisionRequest) (*model.ScheduledWorkout, error)
	RecoverToday(ctx context.Context, userID uuid.UUID, req model.RecoverScheduledWorkoutRequest) (*model.ScheduledWorkout, error)
	PutRequiredSet(ctx context.Context, userID, workoutID, setID uuid.UUID, req model.SetMutationRequest) (*model.ScheduledWorkout, error)
	AddExtraSet(ctx context.Context, userID, workoutID uuid.UUID, req model.ExtraSetRequest) (*model.ScheduledWorkout, error)
	Complete(ctx context.Context, userID, workoutID uuid.UUID, req model.RevisionRequest) (*model.ScheduledWorkout, error)
	Skip(ctx context.Context, userID, workoutID uuid.UUID, req model.RevisionRequest) (*model.ScheduledWorkout, error)
	Unskip(ctx context.Context, userID, workoutID uuid.UUID, req model.RevisionRequest) (*model.ScheduledWorkout, error)
	Delete(ctx context.Context, userID, workoutID uuid.UUID) error
	Move(ctx context.Context, userID, workoutID uuid.UUID, req model.MoveScheduledWorkoutRequest) (*model.ScheduledWorkout, error)
}

type WorkoutSessionService interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.WorkoutSession, error)
	Get(ctx context.Context, userID, sessionID uuid.UUID) (*model.WorkoutSession, error)
	Create(ctx context.Context, userID uuid.UUID, req model.CreateWorkoutSessionRequest) (*model.WorkoutSession, error)
	Patch(ctx context.Context, userID, sessionID uuid.UUID, req model.PatchWorkoutSessionRequest) (*model.WorkoutSession, error)
}

type ParticipationService interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.DayParticipation, error)
}

type scheduleService struct {
	schedules     dao.ScheduleDAO
	programs      dao.ProgramDAO
	profiles      dao.TrainingProfileDAO
	sessions      dao.WorkoutSessionDAO
	sets          dao.PerformedSetDAO
	participation dao.ParticipationDAO
	mutations     dao.TrainingMutationDAO
	idempotency   dao.IdempotencyDAO
	validator     *validator.Validate
	now           func() time.Time
}

type workoutSessionService struct {
	sessions      dao.WorkoutSessionDAO
	schedules     dao.ScheduleDAO
	idempotency   dao.IdempotencyDAO
	participation dao.ParticipationDAO
	profiles      dao.TrainingProfileDAO
	mutations     dao.TrainingMutationDAO
	validator     *validator.Validate
	now           func() time.Time
}

type participationService struct {
	schedule      ScheduleService
	participation dao.ParticipationDAO
}

func NewScheduleService(schedules dao.ScheduleDAO, programs dao.ProgramDAO, profiles dao.TrainingProfileDAO, sessions dao.WorkoutSessionDAO, sets dao.PerformedSetDAO, participation dao.ParticipationDAO, mutations dao.TrainingMutationDAO, idempotency dao.IdempotencyDAO, v *validator.Validate) ScheduleService {
	return &scheduleService{schedules: schedules, programs: programs, profiles: profiles, sessions: sessions, sets: sets, participation: participation, mutations: mutations, idempotency: idempotency, validator: v, now: time.Now}
}

func NewWorkoutSessionService(sessions dao.WorkoutSessionDAO, schedules dao.ScheduleDAO, participation dao.ParticipationDAO, profiles dao.TrainingProfileDAO, mutations dao.TrainingMutationDAO, idempotency dao.IdempotencyDAO, v *validator.Validate) WorkoutSessionService {
	return &workoutSessionService{sessions: sessions, schedules: schedules, participation: participation, profiles: profiles, mutations: mutations, idempotency: idempotency, validator: v, now: time.Now}
}

func NewParticipationService(schedule ScheduleService, participation dao.ParticipationDAO) ParticipationService {
	return &participationService{schedule: schedule, participation: participation}
}

func (s *scheduleService) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.ScheduledWorkout, error) {
	if err := model.ValidateDateRange(from, to); err != nil {
		return nil, err
	}
	// The profile gives the athlete's today for both the fill and the
	// finalize pass. It is read once per request: every read here is a round
	// trip to the database.
	profile, profileErr := s.profiles.Get(ctx, userID)
	if profileErr != nil && !isNotFound(profileErr) {
		return nil, profileErr
	}
	if profileErr == nil {
		if err := s.ensureFilledFor(ctx, userID, profile, to); err != nil {
			return nil, err
		}
	}
	workouts, err := s.schedules.List(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	finalized := false
	today := ""
	for i := range workouts {
		w := &workouts[i]
		if w.FinalizedAt != nil {
			continue
		}
		if profileErr != nil {
			// An unfinalized session needs the saved timezone to be judged.
			return nil, profileErr
		}
		if today == "" {
			location, err := time.LoadLocation(profile.Timezone)
			if err != nil {
				return nil, &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
			}
			today = s.now().In(location).Format("2006-01-02")
		}
		if w.Date >= today {
			continue
		}
		if err := s.finalizePast(ctx, userID, w); err != nil {
			return nil, err
		}
		finalized = true
	}
	if !finalized {
		return workouts, nil
	}
	// A finalize changes status, revision, and participation; read them back.
	return s.schedules.List(ctx, userID, from, to)
}

func (s *scheduleService) Materialize(ctx context.Context, userID uuid.UUID, req model.MaterializeScheduleRequest) ([]model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid materialize request", Field: "body"}
	}
	if err := model.ValidateDateRange(req.From, req.To); err != nil {
		return nil, err
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	if _, atomic := s.schedules.(dao.IdempotentScheduleDAO); !atomic {
		if replay, err := s.idempotency.Get(ctx, userID, "schedule/materialize", req.OperationKey); err == nil {
			if replay.RequestHash != hash {
				return nil, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
			}
			return s.schedules.List(ctx, userID, req.From, req.To)
		} else if !isNotFound(err) {
			return nil, err
		}
	}
	program, err := s.programs.Get(ctx, userID, req.ProgramID)
	if err != nil {
		return nil, err
	}
	if program.Revision != req.ExpectedRevision {
		return nil, &model.ConflictError{Message: "program revision conflict", Expected: req.ExpectedRevision, Actual: program.Revision, Authoritative: program}
	}
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	workouts, placement, err := layoutSessions(program, profile.AvailableDays, req.From, req.To)
	if err != nil {
		return nil, err
	}
	if repo, ok := s.schedules.(dao.IdempotentScheduleDAO); ok {
		result, _, err := repo.MaterializeIdempotent(ctx, userID, req.ProgramID, req.ExpectedRevision, req.To, placement, workouts, model.IdempotencyRecord{Scope: "schedule/materialize", OperationKey: req.OperationKey, RequestHash: hash, ResponseStatus: 201, ResourceType: "schedule"})
		return result, err
	}
	if err := s.schedules.Create(ctx, userID, workouts); err != nil {
		return nil, err
	}
	if err := recordResource(ctx, s.idempotency, userID, "schedule/materialize", req.OperationKey, hash, "schedule", uuid.Nil, program.Revision); err != nil {
		return nil, err
	}
	return workouts, nil
}

func (s *scheduleService) Regenerate(ctx context.Context, userID uuid.UUID, req model.RegenerateScheduleRequest) (*model.RegenerateScheduleResponse, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid regenerate request", Field: "body"}
	}
	if err := model.ValidateDateRange(req.From, req.To); err != nil {
		return nil, err
	}
	program, err := s.programs.Get(ctx, userID, req.ProgramID)
	if err != nil {
		return nil, err
	}
	if program.Revision != req.ExpectedRevision {
		return nil, &model.ConflictError{Message: "program revision conflict", Expected: req.ExpectedRevision, Actual: program.Revision, Authoritative: program}
	}
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	workouts, placement, err := layoutSessions(program, profile.AvailableDays, req.From, req.To)
	if err != nil {
		return nil, err
	}
	token, _ := hashPayload(struct {
		UserID    uuid.UUID
		ProgramID uuid.UUID
		Revision  int64
		From      string
		To        string
	}{userID, req.ProgramID, program.Revision, req.From, req.To})
	response := &model.RegenerateScheduleResponse{PreviewToken: token, ReplacedFrom: &req.From, ReplacedTo: &req.To, ScheduledWorkouts: workouts}
	if !req.Apply {
		return response, nil
	}
	if req.PreviewToken == "" || req.PreviewToken != token {
		return nil, &model.ConflictError{Message: "regeneration preview is stale"}
	}
	if repo, ok := s.schedules.(dao.IdempotentScheduleDAO); ok {
		hash, err := hashPayload(req)
		if err != nil {
			return nil, err
		}
		result, _, err := repo.RegenerateIdempotent(ctx, userID, req.ProgramID, req.ExpectedRevision, req.From, req.To, placement, workouts, response, model.IdempotencyRecord{Scope: "schedule/regenerate", OperationKey: req.OperationKey, RequestHash: hash, ResponseStatus: 200, ResourceType: "schedule"})
		return result, err
	}
	sessions, err := s.sessions.List(ctx, userID, req.From, req.To)
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if sessions[i].Status == "active" {
			return nil, &model.ConflictError{Message: "active session prevents regeneration", Authoritative: &sessions[i]}
		}
	}
	if _, err := s.schedules.DeleteUnstartedRange(ctx, userID, req.From, req.To); err != nil {
		return nil, err
	}
	if err := s.schedules.Create(ctx, userID, workouts); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *scheduleService) PatchWorkout(ctx context.Context, userID, workoutID uuid.UUID, req model.PatchScheduledWorkoutRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid scheduled workout patch", Field: "body"}
	}
	hash, err := hashPayload(struct {
		WorkoutID uuid.UUID
		Request   model.PatchScheduledWorkoutRequest
	}{workoutID, req})
	if err != nil {
		return nil, err
	}
	if s.mutations != nil {
		if replay, found, err := replayResponse[model.ScheduledWorkout](ctx, s.idempotency, userID, "scheduled-workouts/patch", req.OperationKey, hash); err != nil || found {
			return replay, err
		}
	}
	w, err := s.schedules.Get(ctx, userID, workoutID)
	if err != nil {
		return nil, err
	}
	if w.FinalizedAt != nil || w.Date < s.now().UTC().Format("2006-01-02") {
		return nil, &model.ConflictError{Message: "historical scheduled workouts cannot be replaced", Authoritative: w}
	}
	if w.IsSport() && req.RequiredSets != nil && len(*req.RequiredSets) > 0 {
		return nil, errSportSessionHasNoSets
	}
	if req.Name != nil {
		w.Name = *req.Name
	}
	if req.RequiredSets != nil && !w.IsSport() {
		w.RequiredSets = *req.RequiredSets
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	if s.mutations != nil {
		result, _, err := s.mutations.ReplaceScheduledWorkout(ctx, userID, w, req.ExpectedRevision, model.IdempotencyRecord{
			Scope: "scheduled-workouts/patch", OperationKey: req.OperationKey, RequestHash: hash,
			ResponseStatus: 200, ResourceType: "scheduled_workout",
		})
		return result, err
	}
	if err := s.schedules.ReplaceSnapshot(ctx, userID, w, req.ExpectedRevision); err != nil {
		return nil, err
	}
	return s.schedules.Get(ctx, userID, workoutID)
}

// errSportSessionHasNoSets rejects set, target, extra-set, and completion
// routes on a sport session; a linked sport activity completes it instead.
var errSportSessionHasNoSets = &model.ValidationError{Message: "sport sessions have no sets; record a sport activity against the session instead", Field: "kind"}

// rejectSportSession answers 422 on the kind field when the owned scheduled
// unit is a sport session. Ownership failures keep their shared 404.
func (s *scheduleService) rejectSportSession(ctx context.Context, userID, workoutID uuid.UUID) error {
	w, err := s.schedules.Get(ctx, userID, workoutID)
	if err != nil {
		return err
	}
	if w.IsSport() {
		return errSportSessionHasNoSets
	}
	return nil
}

func (s *scheduleService) PatchSetTarget(ctx context.Context, userID, workoutID, setID uuid.UUID, req model.PatchScheduledSetTargetRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid scheduled set target", Field: "body"}
	}
	if err := s.rejectSportSession(ctx, userID, workoutID); err != nil {
		return nil, err
	}
	if s.mutations != nil {
		hash, err := hashPayload(struct {
			WorkoutID uuid.UUID
			SetID     uuid.UUID
			Request   model.PatchScheduledSetTargetRequest
		}{workoutID, setID, req})
		if err != nil {
			return nil, err
		}
		result, _, err := s.mutations.UpdateScheduledSetTarget(ctx, userID, workoutID, setID, req, model.IdempotencyRecord{
			Scope: "scheduled-workouts/set-target", OperationKey: req.OperationKey, RequestHash: hash,
			ResponseStatus: 200, ResourceType: "scheduled_workout",
		})
		return result, err
	}
	if err := s.schedules.UpdateSetTarget(ctx, userID, workoutID, setID, req); err != nil {
		return nil, err
	}
	return s.schedules.Get(ctx, userID, workoutID)
}

// SwapExercise replaces one exercise of a dated workout for that date only.
func (s *scheduleService) SwapExercise(ctx context.Context, userID, workoutID uuid.UUID, exerciseOrder int, req model.SwapScheduledExerciseRequest) (*model.ScheduledWorkout, error) {
	req.ExerciseName = strings.TrimSpace(req.ExerciseName)
	req.ExerciseCategory = strings.TrimSpace(req.ExerciseCategory)
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "exercise_name and exercise_category are required", Field: "body"}
	}
	if err := s.exerciseChangeAllowed(ctx, userID, workoutID, exerciseOrder); err != nil {
		return nil, err
	}
	hash, err := hashPayload(struct {
		WorkoutID     uuid.UUID
		ExerciseOrder int
		Request       model.SwapScheduledExerciseRequest
	}{workoutID, exerciseOrder, req})
	if err != nil {
		return nil, err
	}
	result, _, err := s.mutations.SwapScheduledExercise(ctx, userID, workoutID, exerciseOrder, req, model.IdempotencyRecord{
		Scope: "scheduled-workouts/exercise-swap", OperationKey: req.OperationKey, RequestHash: hash,
		ResponseStatus: 200, ResourceType: "scheduled_workout",
	})
	return result, err
}

// SkipExercise drops the unperformed sets of one exercise from a dated workout.
func (s *scheduleService) SkipExercise(ctx context.Context, userID, workoutID uuid.UUID, exerciseOrder int, req model.RevisionRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "operation_key and expected_revision are required", Field: "body"}
	}
	if err := s.exerciseChangeAllowed(ctx, userID, workoutID, exerciseOrder); err != nil {
		return nil, err
	}
	hash, err := hashPayload(struct {
		WorkoutID     uuid.UUID
		ExerciseOrder int
		Request       model.RevisionRequest
	}{workoutID, exerciseOrder, req})
	if err != nil {
		return nil, err
	}
	result, _, err := s.mutations.SkipScheduledExercise(ctx, userID, workoutID, exerciseOrder, req.ExpectedRevision, model.IdempotencyRecord{
		Scope: "scheduled-workouts/exercise-skip", OperationKey: req.OperationKey, RequestHash: hash,
		ResponseStatus: 200, ResourceType: "scheduled_workout",
	})
	return result, err
}

var errExerciseChangeNeedsStore = errors.New("exercise changes need the training mutation store")

// exerciseChangeAllowed keeps exercise changes to an owned, unfinished gym
// workout dated today or later. A replayed request is answered by the
// mutation itself, so a workout finished since the first attempt still
// returns its recorded response.
func (s *scheduleService) exerciseChangeAllowed(ctx context.Context, userID, workoutID uuid.UUID, exerciseOrder int) error {
	if exerciseOrder < 1 {
		return &model.ValidationError{Message: "exercise_order must be a positive integer", Field: "exercise_order"}
	}
	if s.mutations == nil {
		return errExerciseChangeNeedsStore
	}
	w, err := s.schedules.Get(ctx, userID, workoutID)
	if err != nil {
		return err
	}
	if w.IsSport() {
		return errSportSessionHasNoSets
	}
	return nil
}

func (s *scheduleService) RecoverToday(ctx context.Context, userID uuid.UUID, req model.RecoverScheduledWorkoutRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid recovery request", Field: "body"}
	}
	if _, err := model.ParseDate(req.Date); err != nil {
		return nil, &model.ValidationError{Message: "date must be YYYY-MM-DD", Field: "date"}
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	if repo, ok := s.schedules.(dao.IdempotentScheduleDAO); ok {
		result, _, err := repo.RecoverIdempotent(ctx, userID, req.Date, model.IdempotencyRecord{Scope: "schedule/recover", OperationKey: req.OperationKey, RequestHash: hash, ResponseStatus: 201, ResourceType: "scheduled_workout"})
		return result, err
	}
	if replay, err := s.idempotency.Get(ctx, userID, "schedule/recover", req.OperationKey); err == nil {
		if replay.RequestHash != hash {
			return nil, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
		}
		if replay.ResourceID == nil {
			return nil, &model.ConflictError{Message: "idempotency record has no recovery workout"}
		}
		return s.schedules.Get(ctx, userID, *replay.ResourceID)
	} else if !isNotFound(err) {
		return nil, err
	}
	// Legacy path for DAOs without RecoverIdempotent; production always takes the
	// atomic path above. It filters resolved misses but cannot mark the source.
	missed, err := s.schedules.List(ctx, userID, "0001-01-01", req.Date)
	if err != nil {
		return nil, err
	}
	var source *model.ScheduledWorkout
	for i := range missed {
		if missed[i].Status == model.WorkoutStatusMissed && missed[i].Resolution == nil && (source == nil || sequenceBefore(&missed[i], source)) {
			source = &missed[i]
		}
	}
	if source == nil {
		return nil, &model.NotFoundError{Message: "no missed workout to recover"}
	}
	recovered := *source
	recovered.ID = uuid.Nil
	recovered.Date = req.Date
	recovered.Status = model.WorkoutStatusPlanned
	recovered.FinalizedAt = nil
	recovered.Resolution = nil
	recovered.ResolvedAt = nil
	recovered.Revision = 0
	recovered.CreatedAt = time.Time{}
	recovered.UpdatedAt = time.Time{}
	recovered.ExtraSets = []model.PerformedSet{}
	recovered.RequiredSets = append([]model.ScheduledSet(nil), source.RequiredSets...)
	for i := range recovered.RequiredSets {
		recovered.RequiredSets[i].ID = uuid.Nil
		recovered.RequiredSets[i].Checked = false
		recovered.RequiredSets[i].PerformedSetID = nil
		recovered.RequiredSets[i].ActualReps = nil
		recovered.RequiredSets[i].ActualWeight = nil
		recovered.RequiredSets[i].ActualDurationSeconds = nil
	}
	created := []model.ScheduledWorkout{recovered}
	if err := s.schedules.Create(ctx, userID, created); err != nil {
		return nil, err
	}
	recovered = created[0]
	if err := recordResource(ctx, s.idempotency, userID, "schedule/recover", req.OperationKey, hash, "scheduled_workout", recovered.ID, recovered.Revision); err != nil {
		return nil, err
	}
	return s.schedules.Get(ctx, userID, recovered.ID)
}

func sequenceBefore(a, b *model.ScheduledWorkout) bool {
	if a.SequencePosition != nil && b.SequencePosition != nil && *a.SequencePosition != *b.SequencePosition {
		return *a.SequencePosition < *b.SequencePosition
	}
	return a.Date < b.Date
}

func (s *scheduleService) PutRequiredSet(ctx context.Context, userID, workoutID, setID uuid.UUID, req model.SetMutationRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid set mutation", Field: "body"}
	}
	if err := s.rejectSportSession(ctx, userID, workoutID); err != nil {
		return nil, err
	}
	if s.mutations != nil {
		hash, err := hashPayload(struct {
			WorkoutID uuid.UUID
			SetID     uuid.UUID
			Request   model.SetMutationRequest
		}{workoutID, setID, req})
		if err != nil {
			return nil, err
		}
		result, _, err := s.mutations.PutRequiredSet(ctx, userID, workoutID, setID, req, model.IdempotencyRecord{
			Scope: "scheduled-workouts/required-set", OperationKey: req.OperationKey, RequestHash: hash,
			ResponseStatus: 200, ResourceType: "scheduled_workout",
		})
		return result, err
	}
	w, err := s.checkedWorkout(ctx, userID, workoutID, req.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	session, err := s.ensureSession(ctx, userID, w)
	if err != nil {
		return nil, err
	}
	set := &model.PerformedSet{ScheduledSetID: &setID, IsExtra: false, ActualReps: req.ActualReps, ActualWeight: req.ActualWeight, DurationSeconds: req.DurationSeconds, Completed: req.Completed, OperationKey: req.OperationKey}
	if _, err := s.sets.PutRequired(ctx, userID, session.ID, setID, set, session.Revision); err != nil {
		return nil, err
	}
	return s.refreshLiveStatus(ctx, userID, w)
}

func (s *scheduleService) AddExtraSet(ctx context.Context, userID, workoutID uuid.UUID, req model.ExtraSetRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid extra set", Field: "body"}
	}
	set := &model.PerformedSet{ExerciseID: req.ExerciseID, IsExtra: true, ExerciseName: req.ExerciseName, ExerciseCategory: req.ExerciseCategory, ExerciseModality: req.ExerciseModality, SetIndex: req.SetIndex, ActualReps: req.ActualReps, ActualWeight: req.ActualWeight, DurationSeconds: req.DurationSeconds, Completed: req.Completed, OperationKey: req.OperationKey}
	if err := set.Validate(); err != nil {
		return nil, err
	}
	if err := s.rejectSportSession(ctx, userID, workoutID); err != nil {
		return nil, err
	}
	if s.mutations != nil {
		hash, err := hashPayload(struct {
			WorkoutID uuid.UUID
			Request   model.ExtraSetRequest
		}{workoutID, req})
		if err != nil {
			return nil, err
		}
		result, _, err := s.mutations.AddExtraSet(ctx, userID, workoutID, req, model.IdempotencyRecord{
			Scope: "scheduled-workouts/extra-set", OperationKey: req.OperationKey, RequestHash: hash,
			ResponseStatus: 201, ResourceType: "scheduled_workout",
		})
		return result, err
	}
	w, err := s.checkedWorkout(ctx, userID, workoutID, req.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	session, err := s.ensureSession(ctx, userID, w)
	if err != nil {
		return nil, err
	}
	if _, err := s.sets.AddExtra(ctx, userID, session.ID, set, session.Revision); err != nil {
		return nil, err
	}
	return s.refreshLiveStatus(ctx, userID, w)
}

func (s *scheduleService) Complete(ctx context.Context, userID, workoutID uuid.UUID, req model.RevisionRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid completion request", Field: "body"}
	}
	if err := s.rejectSportSession(ctx, userID, workoutID); err != nil {
		return nil, err
	}
	if s.mutations != nil {
		hash, err := hashPayload(struct {
			WorkoutID uuid.UUID
			Request   model.RevisionRequest
		}{workoutID, req})
		if err != nil {
			return nil, err
		}
		result, _, err := s.mutations.FinalizeScheduledWorkout(ctx, userID, workoutID, req.ExpectedRevision, s.now(), model.IdempotencyRecord{
			Scope: "scheduled-workouts/complete", OperationKey: req.OperationKey, RequestHash: hash,
			ResponseStatus: 200, ResourceType: "scheduled_workout",
		})
		return result, err
	}
	w, err := s.checkedWorkout(ctx, userID, workoutID, req.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	return s.finalizeLegacy(ctx, userID, w)
}

// ErrResolutionUnavailable reports a schedule service built without the
// transactional mutation DAO that skip and unskip require.
var ErrResolutionUnavailable = errors.New("scheduled workout resolution requires transactional mutations")

// Skip marks an owned missed workout as skipped so it is no longer offered as
// next. Its outcome, participation, and history are unchanged.
func (s *scheduleService) Skip(ctx context.Context, userID, workoutID uuid.UUID, req model.RevisionRequest) (*model.ScheduledWorkout, error) {
	return s.resolve(ctx, userID, workoutID, req, "scheduled-workouts/skip", "invalid skip request", true)
}

// Unskip clears a skip so the missed workout is unresolved again.
func (s *scheduleService) Unskip(ctx context.Context, userID, workoutID uuid.UUID, req model.RevisionRequest) (*model.ScheduledWorkout, error) {
	return s.resolve(ctx, userID, workoutID, req, "scheduled-workouts/unskip", "invalid unskip request", false)
}

func (s *scheduleService) resolve(ctx context.Context, userID, workoutID uuid.UUID, req model.RevisionRequest, scope, invalid string, skip bool) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: invalid, Field: "body"}
	}
	if s.mutations == nil {
		return nil, ErrResolutionUnavailable
	}
	hash, err := hashPayload(struct {
		WorkoutID uuid.UUID
		Request   model.RevisionRequest
	}{workoutID, req})
	if err != nil {
		return nil, err
	}
	record := model.IdempotencyRecord{
		Scope: scope, OperationKey: req.OperationKey, RequestHash: hash,
		ResponseStatus: 200, ResourceType: "scheduled_workout",
	}
	if skip {
		result, _, err := s.mutations.SkipScheduledWorkout(ctx, userID, workoutID, req.ExpectedRevision, record)
		return result, err
	}
	result, _, err := s.mutations.UnskipScheduledWorkout(ctx, userID, workoutID, req.ExpectedRevision, record)
	return result, err
}

// ensureFilled keeps the active program scheduled ahead of today before a
// read that reaches today or later. The schedule rolls forward on its own:
// the athlete never has to change something to get next week.
// ensureFilledFor extends the active program's sessions to the horizon when
// the requested range reaches today or later.
func (s *scheduleService) ensureFilledFor(ctx context.Context, userID uuid.UUID, profile *model.TrainingProfile, to string) error {
	repo, ok := s.schedules.(dao.RollingScheduleDAO)
	if !ok {
		return nil
	}
	today := localToday(s.now(), profile.Timezone)
	if to < today {
		return nil
	}
	return repo.EnsureFilled(ctx, userID, today, profile.AvailableDays)
}

// Move changes the date of one planned, unstarted session to a date from
// today through the filled range. Only that session changes; next week the
// program is back on its own weekdays.
func (s *scheduleService) Move(ctx context.Context, userID, workoutID uuid.UUID, req model.MoveScheduledWorkoutRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid move request", Field: "body"}
	}
	repo, ok := s.schedules.(dao.RollingScheduleDAO)
	if !ok {
		return nil, errTrainingMutationsUnavailable
	}
	target, err := model.ParseDate(req.Date)
	if err != nil {
		return nil, &model.ValidationError{Field: "date", Message: "date must be YYYY-MM-DD"}
	}
	timezone := "UTC"
	if profile, err := s.profiles.Get(ctx, userID); err == nil {
		timezone = profile.Timezone
	} else if !isNotFound(err) {
		return nil, err
	}
	today := localToday(s.now(), timezone)
	first, _ := model.ParseDate(today)
	if target.Before(first) || target.After(first.AddDate(0, 0, model.ScheduleHorizonDays)) {
		return nil, &model.ValidationError{Field: "date", Message: "date must be from today through the next eight weeks"}
	}
	hash, err := hashPayload(struct {
		WorkoutID uuid.UUID
		Request   model.MoveScheduledWorkoutRequest
	}{workoutID, req})
	if err != nil {
		return nil, err
	}
	result, _, err := repo.MoveIdempotent(ctx, userID, workoutID, req.Date, today, req.ExpectedRevision, model.IdempotencyRecord{
		Scope: "scheduled-workouts/move", OperationKey: req.OperationKey, RequestHash: hash,
		ResponseStatus: 200, ResourceType: "scheduled_workout",
	})
	return result, err
}

// Delete removes an owned past or finalized scheduled workout and what was
// performed against it. Today's and future unfinished workouts belong to the
// plan, which is edited, skipped, or regenerated instead.
func (s *scheduleService) Delete(ctx context.Context, userID, workoutID uuid.UUID) error {
	w, err := s.schedules.Get(ctx, userID, workoutID)
	if err != nil {
		return err
	}
	if w.FinalizedAt == nil && w.Date >= s.now().UTC().Format("2006-01-02") {
		return &model.ConflictError{Message: "only past or finished scheduled workouts can be deleted", Authoritative: w}
	}
	return s.schedules.Delete(ctx, userID, workoutID)
}

func (s *scheduleService) checkedWorkout(ctx context.Context, userID, workoutID uuid.UUID, revision int64) (*model.ScheduledWorkout, error) {
	w, err := s.schedules.Get(ctx, userID, workoutID)
	if err != nil {
		return nil, err
	}
	if w.Revision != revision {
		return nil, &model.ConflictError{Message: "scheduled workout revision conflict", Expected: revision, Actual: w.Revision, Authoritative: w}
	}
	return w, nil
}

func (s *scheduleService) ensureSession(ctx context.Context, userID uuid.UUID, w *model.ScheduledWorkout) (*model.WorkoutSession, error) {
	sessions, err := s.sessions.List(ctx, userID, w.Date, w.Date)
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if sessions[i].ScheduledWorkoutID != nil && *sessions[i].ScheduledWorkoutID == w.ID && sessions[i].Status != "discarded" {
			return &sessions[i], nil
		}
	}
	id := w.ID
	session := &model.WorkoutSession{ScheduledWorkoutID: &id, Date: w.Date, Name: w.Name, Status: "draft"}
	if err := s.sessions.Create(ctx, userID, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *scheduleService) refreshLiveStatus(ctx context.Context, userID uuid.UUID, prior *model.ScheduledWorkout) (*model.ScheduledWorkout, error) {
	w, err := s.schedules.Get(ctx, userID, prior.ID)
	if err != nil {
		return nil, err
	}
	w.Status = deriveWorkoutStatus(w, w.FinalizedAt != nil)
	if err := s.schedules.UpdateOutcome(ctx, userID, w, prior.Revision); err != nil {
		return nil, err
	}
	return s.schedules.Get(ctx, userID, prior.ID)
}

// deriveWorkoutStatus derives the live or finalized status for any unit kind.
// A sport session is completed only by its linked activity and is otherwise
// planned until finalized, then missed.
func deriveWorkoutStatus(w *model.ScheduledWorkout, finalized bool) string {
	if w.IsSport() {
		if w.Sport != nil && w.Sport.SportActivityID != nil {
			return model.WorkoutStatusCompleted
		}
		if finalized {
			return model.WorkoutStatusMissed
		}
		return model.WorkoutStatusPlanned
	}
	return deriveScheduledStatus(w.RequiredSets, finalized)
}

func deriveScheduledStatus(sets []model.ScheduledSet, finalized bool) string {
	checked := checkedCount(sets)
	if finalized && len(sets) > 0 && checked == len(sets) {
		return model.WorkoutStatusCompleted
	}
	if finalized && checked > 0 {
		return model.WorkoutStatusIncomplete
	}
	if finalized {
		return model.WorkoutStatusMissed
	}
	if checked > 0 {
		return model.WorkoutStatusInProgress
	}
	return model.WorkoutStatusPlanned
}

// finalizePast closes a session dated before the athlete's today that was
// never finished: it becomes missed or completed from what was logged.
func (s *scheduleService) finalizePast(ctx context.Context, userID uuid.UUID, w *model.ScheduledWorkout) error {
	if s.mutations != nil {
		operationKey := "lazy-finalize:" + w.ID.String()
		hash, hashErr := hashPayload(struct {
			WorkoutID        uuid.UUID
			ExpectedRevision int64
		}{w.ID, w.Revision})
		if hashErr != nil {
			return hashErr
		}
		_, _, err := s.mutations.FinalizeScheduledWorkout(ctx, userID, w.ID, w.Revision, s.now(), model.IdempotencyRecord{
			Scope: "scheduled-workouts/lazy-finalize", OperationKey: operationKey, RequestHash: hash,
			ResponseStatus: 200, ResourceType: "scheduled_workout",
		})
		return err
	}
	_, err := s.finalizeLegacy(ctx, userID, w)
	return err
}

func (s *scheduleService) finalizeLegacy(ctx context.Context, userID uuid.UUID, w *model.ScheduledWorkout) (*model.ScheduledWorkout, error) {
	if w.FinalizedAt != nil {
		return w, nil
	}
	checked := checkedCount(w.RequiredSets)
	w.Status = deriveWorkoutStatus(w, true)
	now := s.now().UTC()
	w.FinalizedAt = &now
	if err := s.schedules.UpdateOutcome(ctx, userID, w, w.Revision); err != nil {
		return nil, err
	}
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	participated := checked > 0
	for _, set := range w.ExtraSets {
		participated = participated || set.Completed
	}
	if w.IsSport() {
		participated = w.Status == model.WorkoutStatusCompleted
	}
	outcome := &model.DayParticipation{Date: w.Date, ScheduledOpportunity: true, Participated: participated, FinalizedAt: now, Timezone: profile.Timezone, LocalDate: w.Date}
	if err := s.participation.Finalize(ctx, userID, outcome); err != nil {
		if !isConflict(err) {
			return nil, err
		}
	}
	return s.schedules.Get(ctx, userID, w.ID)
}

func checkedCount(sets []model.ScheduledSet) int {
	count := 0
	for _, set := range sets {
		if set.Checked {
			count++
		}
	}
	return count
}

// layoutSessions returns the program's sessions between from and to. A
// program without weekdays is first placed, in memory, across availableDays
// starting at from; placement then holds the placed units for the DAO to
// store in the transaction that creates the sessions. It is nil when the
// program was already placed.
func layoutSessions(p *model.Program, availableDays []int, from, to string) (sessions []model.ScheduledWorkout, placement []model.ProgramWorkout, err error) {
	if len(p.Workouts) > 0 && !p.Placed() {
		if err := model.PlaceProgram(p, availableDays, from); err != nil {
			return nil, nil, err
		}
		placement = p.Workouts
	}
	sessions, err = model.LayoutSessions(p, from, to)
	if err != nil {
		return nil, nil, err
	}
	return sessions, placement, nil
}

func (s *workoutSessionService) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.WorkoutSession, error) {
	if err := model.ValidateDateRange(from, to); err != nil {
		return nil, err
	}
	return s.sessions.List(ctx, userID, from, to)
}

func (s *workoutSessionService) Get(ctx context.Context, userID, sessionID uuid.UUID) (*model.WorkoutSession, error) {
	return s.sessions.Get(ctx, userID, sessionID)
}

func (s *workoutSessionService) Create(ctx context.Context, userID uuid.UUID, req model.CreateWorkoutSessionRequest) (*model.WorkoutSession, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid workout session", Field: "body"}
	}
	if _, err := model.ParseDate(req.Date); err != nil {
		return nil, &model.ValidationError{Message: "date must be YYYY-MM-DD", Field: "date"}
	}
	if s.mutations != nil {
		hash, err := hashPayload(req)
		if err != nil {
			return nil, err
		}
		session := &model.WorkoutSession{ScheduledWorkoutID: req.ScheduledWorkoutID, Date: req.Date, Name: req.Name, Status: "draft", Notes: req.Notes}
		result, _, err := s.mutations.CreateWorkoutSession(ctx, userID, session, model.IdempotencyRecord{
			Scope: "workout-sessions/create", OperationKey: req.OperationKey, RequestHash: hash,
			ResponseStatus: 201, ResourceType: "workout_session",
		})
		return result, err
	}
	if req.ScheduledWorkoutID != nil {
		if _, err := s.schedules.Get(ctx, userID, *req.ScheduledWorkoutID); err != nil {
			return nil, err
		}
	}
	session := &model.WorkoutSession{ScheduledWorkoutID: req.ScheduledWorkoutID, Date: req.Date, Name: req.Name, Status: "draft", Notes: req.Notes}
	if err := s.sessions.Create(ctx, userID, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *workoutSessionService) Patch(ctx context.Context, userID, sessionID uuid.UUID, req model.PatchWorkoutSessionRequest) (*model.WorkoutSession, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid workout session patch", Field: "body"}
	}
	hash, err := hashPayload(struct {
		SessionID uuid.UUID
		Request   model.PatchWorkoutSessionRequest
	}{sessionID, req})
	if err != nil {
		return nil, err
	}
	if s.mutations != nil {
		if replay, found, err := replayResponse[model.WorkoutSession](ctx, s.idempotency, userID, "workout-sessions/update", req.OperationKey, hash); err != nil || found {
			return replay, err
		}
	}
	session, err := s.sessions.Get(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Date < s.now().UTC().Format("2006-01-02") && req.Status != nil && *req.Status == "discarded" {
		return nil, &model.ConflictError{Message: "historical workout sessions cannot be discarded", Authoritative: session}
	}
	if req.Name != nil {
		session.Name = *req.Name
	}
	if req.Notes != nil {
		session.Notes = req.Notes
	}
	if req.Status != nil {
		valid := false
		for _, status := range model.ValidSessionStatuses {
			valid = valid || *req.Status == status
		}
		if !valid {
			return nil, &model.ValidationError{Message: "unknown workout session status", Field: "status"}
		}
		session.Status = *req.Status
		if *req.Status == "completed" {
			now := s.now().UTC()
			session.CompletedAt = &now
		}
	}
	if s.mutations != nil {
		result, _, err := s.mutations.ReplaceWorkoutSession(ctx, userID, session, req.ExpectedRevision, s.now(), model.IdempotencyRecord{
			Scope: "workout-sessions/update", OperationKey: req.OperationKey, RequestHash: hash,
			ResponseStatus: 200, ResourceType: "workout_session",
		})
		return result, err
	}
	if err := s.sessions.Update(ctx, userID, session, req.ExpectedRevision); err != nil {
		return nil, err
	}
	if session.Status == "completed" {
		profile, err := s.profiles.Get(ctx, userID)
		if err != nil {
			return nil, err
		}
		now := s.now().UTC()
		outcome := &model.DayParticipation{Date: session.Date, ScheduledOpportunity: session.ScheduledWorkoutID != nil, Participated: true, FinalizedAt: now, Timezone: profile.Timezone, LocalDate: session.Date}
		if err := s.participation.Preserve(ctx, userID, outcome); err != nil {
			return nil, err
		}
	}
	return s.sessions.Get(ctx, userID, sessionID)
}

func (s *participationService) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.DayParticipation, error) {
	if _, err := s.schedule.List(ctx, userID, from, to); err != nil {
		return nil, err
	}
	return s.participation.List(ctx, userID, from, to)
}

func hashPayload(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encoding idempotent request: %w", err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func replayResponse[T any](ctx context.Context, repo dao.IdempotencyDAO, userID uuid.UUID, scope, operationKey, requestHash string) (*T, bool, error) {
	if repo == nil {
		return nil, false, nil
	}
	record, err := repo.Get(ctx, userID, scope, operationKey)
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if record.RequestHash != requestHash {
		return nil, false, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
	}
	var response T
	if err := json.Unmarshal(record.ResponseBody, &response); err != nil {
		return nil, false, fmt.Errorf("decoding idempotency response: %w", err)
	}
	return &response, true, nil
}

func recordResource(ctx context.Context, repo dao.IdempotencyDAO, userID uuid.UUID, scope, operationKey, requestHash, resourceType string, resourceID uuid.UUID, revision int64) error {
	record := &model.IdempotencyRecord{Scope: scope, OperationKey: operationKey, RequestHash: requestHash, ResponseStatus: 200, ResponseBody: []byte("{}"), ResourceType: resourceType, ResourceRevision: &revision}
	if resourceID != uuid.Nil {
		record.ResourceID = &resourceID
	}
	return repo.Create(ctx, userID, record)
}

func isConflict(err error) bool {
	var conflict *model.ConflictError
	return errors.As(err, &conflict)
}
