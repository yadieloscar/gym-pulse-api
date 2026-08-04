package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type trainingBlockDAOStub struct {
	listStatus string
	listLimit  int
	listOffset int
	created    model.CreateTrainingBlockRequest
	exposure   model.CreateTrainingExposureRequest
	transition model.CreateTrainingTransitionRequest
	hash       string
}

func (s *trainingBlockDAOStub) List(_ context.Context, _ uuid.UUID, status string, limit, offset int) (*model.TrainingBlockList, error) {
	s.listStatus, s.listLimit, s.listOffset = status, limit, offset
	return &model.TrainingBlockList{TrainingBlocks: []model.TrainingBlockSummary{}}, nil
}
func (s *trainingBlockDAOStub) Get(context.Context, uuid.UUID, uuid.UUID) (*model.TrainingBlock, error) {
	return &model.TrainingBlock{}, nil
}
func (s *trainingBlockDAOStub) Create(_ context.Context, _ uuid.UUID, req model.CreateTrainingBlockRequest, hash string) (*model.TrainingBlock, bool, error) {
	s.created, s.hash = req, hash
	return &model.TrainingBlock{}, false, nil
}
func (s *trainingBlockDAOStub) AddExposure(_ context.Context, _, _ uuid.UUID, req model.CreateTrainingExposureRequest, hash string) (*model.TrainingBlock, bool, error) {
	s.exposure, s.hash = req, hash
	return &model.TrainingBlock{}, false, nil
}
func (s *trainingBlockDAOStub) RecordNextMorning(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.RecordNextMorningRequest, string) (*model.TrainingBlock, bool, error) {
	return &model.TrainingBlock{}, false, nil
}
func (s *trainingBlockDAOStub) Transition(_ context.Context, _, _ uuid.UUID, req model.CreateTrainingTransitionRequest, hash string) (*model.TrainingBlock, bool, error) {
	s.transition, s.hash = req, hash
	return &model.TrainingBlock{}, false, nil
}

type trainingBlockProfileStub struct{ timezone string }

func (s trainingBlockProfileStub) Get(context.Context, uuid.UUID) (*model.TrainingProfile, error) {
	return &model.TrainingProfile{Timezone: s.timezone}, nil
}
func (s trainingBlockProfileStub) Put(context.Context, uuid.UUID, *model.TrainingProfile, int64) error {
	return nil
}

func validTrainingBlockRequest() model.CreateTrainingBlockRequest {
	return model.CreateTrainingBlockRequest{
		Name: " Return to spiking ", Purpose: stringPointer(" Athlete-authored block "),
		OperationKey: uuid.NewString(),
		Stages: []model.CreateTrainingStageRequest{
			{Name: " Stage one ", Instructions: stringPointer(" Controlled "), LoadLevel: model.TrainingLoadEasy, RequiredQualifyingExposures: 2},
			{Name: "Stage two", LoadLevel: model.TrainingLoadDemanding, RequiredQualifyingExposures: 1},
		},
	}
}

func TestTrainingBlockServiceCreateNormalizesAndHashes(t *testing.T) {
	repo := &trainingBlockDAOStub{}
	svc := NewTrainingBlockService(repo, trainingBlockProfileStub{timezone: "America/New_York"})
	if _, err := svc.Create(context.Background(), uuid.New(), validTrainingBlockRequest()); err != nil {
		t.Fatal(err)
	}
	if repo.created.Name != "Return to spiking" || repo.created.Stages[0].Name != "Stage one" || repo.created.Purpose == nil || *repo.created.Purpose != "Athlete-authored block" {
		t.Fatalf("request was not normalized: %+v", repo.created)
	}
	if repo.hash == "" {
		t.Fatal("request payload was not hashed")
	}
}

func TestTrainingBlockServiceRejectsInvalidCreateAndPagination(t *testing.T) {
	repo := &trainingBlockDAOStub{}
	svc := NewTrainingBlockService(repo, trainingBlockProfileStub{timezone: "UTC"})
	req := validTrainingBlockRequest()
	req.Stages = req.Stages[:1]
	if _, err := svc.Create(context.Background(), uuid.New(), req); err == nil {
		t.Fatal("expected stage-count validation")
	}
	if _, err := svc.List(context.Background(), uuid.New(), "", 0, 0); err != nil {
		t.Fatal(err)
	}
	if repo.listStatus != model.TrainingBlockActive || repo.listLimit != 20 || repo.listOffset != 0 {
		t.Fatalf("unexpected list defaults: %s %d %d", repo.listStatus, repo.listLimit, repo.listOffset)
	}
	if _, err := svc.List(context.Background(), uuid.New(), "medical-clearance", 20, 0); err == nil {
		t.Fatal("expected invalid status validation")
	}
}

func TestTrainingBlockServiceExposureUsesProfileTimezoneAndBounds(t *testing.T) {
	repo := &trainingBlockDAOStub{}
	serviceInterface := NewTrainingBlockService(repo, trainingBlockProfileStub{timezone: "America/Los_Angeles"})
	svc, ok := serviceInterface.(*trainingBlockService)
	if !ok {
		t.Fatal("NewTrainingBlockService returned an unexpected implementation")
	}
	svc.now = func() time.Time { return time.Date(2026, 8, 5, 2, 0, 0, 0, time.UTC) }
	req := model.CreateTrainingExposureRequest{
		PerformedOn: "2026-08-04", ActivityLabel: " Hitting practice ",
		LoadLevel: model.TrainingLoadDemanding, SessionOutcome: model.SessionCompletedAsPlanned,
		ExpectedRevision: 1, OperationKey: uuid.NewString(),
	}
	if _, err := svc.AddExposure(context.Background(), uuid.New(), uuid.New(), req); err != nil {
		t.Fatal(err)
	}
	if repo.exposure.ActivityLabel != "Hitting practice" {
		t.Fatalf("label was not normalized: %q", repo.exposure.ActivityLabel)
	}
	req.PerformedOn = "2026-08-05"
	if _, err := svc.AddExposure(context.Background(), uuid.New(), uuid.New(), req); err == nil {
		t.Fatal("expected future local date rejection")
	}
}

func TestTrainingBlockServiceRequiresRegressionReason(t *testing.T) {
	repo := &trainingBlockDAOStub{}
	svc := NewTrainingBlockService(repo, trainingBlockProfileStub{timezone: "UTC"})
	req := model.CreateTrainingTransitionRequest{Action: model.TransitionRegress, ToStageID: uuidPointer(uuid.New()), ExpectedRevision: 2, OperationKey: uuid.NewString()}
	if _, err := svc.Transition(context.Background(), uuid.New(), uuid.New(), req); err == nil {
		t.Fatal("expected regression reason validation")
	}
	req.Reason = stringPointer(" Rebuild confidence ")
	if _, err := svc.Transition(context.Background(), uuid.New(), uuid.New(), req); err != nil {
		t.Fatal(err)
	}
	if repo.transition.Reason == nil || *repo.transition.Reason != "Rebuild confidence" {
		t.Fatalf("reason was not normalized: %+v", repo.transition.Reason)
	}
}

func TestTrainingBlockServiceRejectsInvalidSavedTimezone(t *testing.T) {
	svc := NewTrainingBlockService(&trainingBlockDAOStub{}, trainingBlockProfileStub{timezone: "not/a-zone"})
	_, err := svc.AddExposure(context.Background(), uuid.New(), uuid.New(), model.CreateTrainingExposureRequest{})
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "timezone" {
		t.Fatalf("expected timezone validation, got %v", err)
	}
}

func stringPointer(value string) *string     { return &value }
func uuidPointer(value uuid.UUID) *uuid.UUID { return &value }
