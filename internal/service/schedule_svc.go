package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	RecoverToday(ctx context.Context, userID uuid.UUID, req model.RecoverScheduledWorkoutRequest) (*model.ScheduledWorkout, error)
	PutRequiredSet(ctx context.Context, userID, workoutID, setID uuid.UUID, req model.SetMutationRequest) (*model.ScheduledWorkout, error)
	AddExtraSet(ctx context.Context, userID, workoutID uuid.UUID, req model.ExtraSetRequest) (*model.ScheduledWorkout, error)
	Complete(ctx context.Context, userID, workoutID uuid.UUID, req model.RevisionRequest) (*model.ScheduledWorkout, error)
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
	validator     *validator.Validate
	now           func() time.Time
}

type participationService struct {
	schedule      ScheduleService
	participation dao.ParticipationDAO
}

func NewScheduleService(schedules dao.ScheduleDAO, programs dao.ProgramDAO, profiles dao.TrainingProfileDAO, sessions dao.WorkoutSessionDAO, sets dao.PerformedSetDAO, participation dao.ParticipationDAO, idempotency dao.IdempotencyDAO, v *validator.Validate) ScheduleService {
	return &scheduleService{schedules: schedules, programs: programs, profiles: profiles, sessions: sessions, sets: sets, participation: participation, idempotency: idempotency, validator: v, now: time.Now}
}

func NewWorkoutSessionService(sessions dao.WorkoutSessionDAO, schedules dao.ScheduleDAO, participation dao.ParticipationDAO, profiles dao.TrainingProfileDAO, idempotency dao.IdempotencyDAO, v *validator.Validate) WorkoutSessionService {
	return &workoutSessionService{sessions: sessions, schedules: schedules, participation: participation, profiles: profiles, idempotency: idempotency, validator: v, now: time.Now}
}

func NewParticipationService(schedule ScheduleService, participation dao.ParticipationDAO) ParticipationService {
	return &participationService{schedule: schedule, participation: participation}
}

func (s *scheduleService) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.ScheduledWorkout, error) {
	if err := model.ValidateDateRange(from, to); err != nil {
		return nil, err
	}
	workouts, err := s.schedules.List(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	for i := range workouts {
		if err := s.lazyFinalize(ctx, userID, &workouts[i]); err != nil {
			return nil, err
		}
	}
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
	workouts, err := model.MaterializeProgramForWeekdays(program, profile.AvailableDays, req.From, req.To)
	if err != nil {
		return nil, err
	}
	if repo, ok := s.schedules.(dao.IdempotentScheduleDAO); ok {
		result, _, err := repo.MaterializeIdempotent(ctx, userID, req.ProgramID, req.ExpectedRevision, workouts, model.IdempotencyRecord{Scope: "schedule/materialize", OperationKey: req.OperationKey, RequestHash: hash, ResponseStatus: 201, ResourceType: "schedule"})
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
	workouts, err := model.MaterializeProgramForWeekdays(program, profile.AvailableDays, req.From, req.To)
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
		result, _, err := repo.RegenerateIdempotent(ctx, userID, req.ProgramID, req.ExpectedRevision, req.From, req.To, workouts, response, model.IdempotencyRecord{Scope: "schedule/regenerate", OperationKey: req.OperationKey, RequestHash: hash, ResponseStatus: 200, ResourceType: "schedule"})
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
	w, err := s.schedules.Get(ctx, userID, workoutID)
	if err != nil {
		return nil, err
	}
	if w.FinalizedAt != nil || w.Date < s.now().UTC().Format("2006-01-02") {
		return nil, &model.ConflictError{Message: "historical scheduled workouts cannot be replaced", Authoritative: w}
	}
	if req.Name != nil {
		w.Name = *req.Name
	}
	if req.RequiredSets != nil {
		w.RequiredSets = *req.RequiredSets
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	if err := s.schedules.ReplaceSnapshot(ctx, userID, w, req.ExpectedRevision); err != nil {
		return nil, err
	}
	return s.schedules.Get(ctx, userID, workoutID)
}

func (s *scheduleService) PatchSetTarget(ctx context.Context, userID, workoutID, setID uuid.UUID, req model.PatchScheduledSetTargetRequest) (*model.ScheduledWorkout, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid scheduled set target", Field: "body"}
	}
	if err := s.schedules.UpdateSetTarget(ctx, userID, workoutID, setID, req); err != nil {
		return nil, err
	}
	return s.schedules.Get(ctx, userID, workoutID)
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
	missed, err := s.schedules.List(ctx, userID, "0001-01-01", req.Date)
	if err != nil {
		return nil, err
	}
	var source *model.ScheduledWorkout
	for i := range missed {
		if missed[i].Status == model.WorkoutStatusMissed && (source == nil || sequenceBefore(&missed[i], source)) {
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
	w, err := s.checkedWorkout(ctx, userID, workoutID, req.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	session, err := s.ensureSession(ctx, userID, w)
	if err != nil {
		return nil, err
	}
	set := &model.PerformedSet{ExerciseID: req.ExerciseID, IsExtra: true, ExerciseName: req.ExerciseName, ExerciseCategory: req.ExerciseCategory, ExerciseModality: req.ExerciseModality, SetIndex: req.SetIndex, ActualReps: req.ActualReps, ActualWeight: req.ActualWeight, DurationSeconds: req.DurationSeconds, Completed: req.Completed, OperationKey: req.OperationKey}
	if err := set.Validate(); err != nil {
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
	w, err := s.checkedWorkout(ctx, userID, workoutID, req.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	return s.finalize(ctx, userID, w)
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
	w.Status = deriveScheduledStatus(w.RequiredSets, w.FinalizedAt != nil)
	if err := s.schedules.UpdateOutcome(ctx, userID, w, prior.Revision); err != nil {
		return nil, err
	}
	return s.schedules.Get(ctx, userID, prior.ID)
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

func (s *scheduleService) lazyFinalize(ctx context.Context, userID uuid.UUID, w *model.ScheduledWorkout) error {
	if w.FinalizedAt != nil {
		return nil
	}
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return err
	}
	location, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}
	if w.Date >= s.now().In(location).Format("2006-01-02") {
		return nil
	}
	_, err = s.finalize(ctx, userID, w)
	return err
}

func (s *scheduleService) finalize(ctx context.Context, userID uuid.UUID, w *model.ScheduledWorkout) (*model.ScheduledWorkout, error) {
	if w.FinalizedAt != nil {
		return w, nil
	}
	checked := checkedCount(w.RequiredSets)
	w.Status = deriveScheduledStatus(w.RequiredSets, true)
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

func materializeProgram(p *model.Program, from, to string) ([]model.ScheduledWorkout, error) {
	days := []int{}
	for _, workout := range p.Workouts {
		if workout.PreferredWeekday != nil {
			days = append(days, *workout.PreferredWeekday)
		}
	}
	return model.MaterializeProgramForWeekdays(p, days, from, to)
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
