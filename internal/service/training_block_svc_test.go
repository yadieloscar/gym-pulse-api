package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type trainingBlockDAOStub struct {
	listStatus  string
	listLimit   int
	listOffset  int
	created     model.CreateTrainingBlockRequest
	exposure    model.CreateTrainingExposureRequest
	nextMorning model.RecordNextMorningRequest
	transition  model.CreateTrainingTransitionRequest
	hash        string
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

func (s *trainingBlockDAOStub) RecordNextMorning(_ context.Context, _, _, _ uuid.UUID, req model.RecordNextMorningRequest, hash string) (*model.TrainingBlock, bool, error) {
	s.nextMorning, s.hash = req, hash
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

func TestTrainingBlockServiceGetRecordNextMorningAndListValidation(t *testing.T) {
	repo := &trainingBlockDAOStub{}
	svc := NewTrainingBlockService(repo, trainingBlockProfileStub{timezone: "UTC"})
	userID, blockID, exposureID := uuid.New(), uuid.New(), uuid.New()

	if _, err := svc.Get(context.Background(), userID, blockID); err != nil {
		t.Fatal(err)
	}
	operationKey := uuid.NewString()
	if _, err := svc.RecordNextMorning(context.Background(), userID, blockID, exposureID, model.RecordNextMorningRequest{
		Response:         " baseline ",
		ExpectedRevision: 2,
		OperationKey:     " " + operationKey + " ",
	}); err != nil {
		t.Fatal(err)
	}
	if repo.nextMorning.Response != model.NextMorningBaseline || repo.nextMorning.OperationKey != operationKey || repo.hash == "" {
		t.Fatalf("next-morning request was not normalized and hashed: %+v", repo.nextMorning)
	}

	if _, err := svc.RecordNextMorning(context.Background(), userID, blockID, exposureID, model.RecordNextMorningRequest{Response: "improved"}); err == nil {
		t.Fatal("expected invalid response validation")
	}
	if _, err := svc.RecordNextMorning(context.Background(), userID, blockID, exposureID, model.RecordNextMorningRequest{Response: model.NextMorningBaseline, OperationKey: uuid.NewString()}); err == nil {
		t.Fatal("expected revision validation")
	}
	for _, tc := range []struct {
		name   string
		status string
		limit  int
		offset int
	}{
		{name: "limit below range", limit: -1},
		{name: "limit above range", limit: 101},
		{name: "negative offset", limit: 20, offset: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.List(context.Background(), userID, tc.status, tc.limit, tc.offset); err == nil {
				t.Fatal("expected list validation")
			}
		})
	}
}

func TestValidateCreateTrainingBlockFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.CreateTrainingBlockRequest)
		field  string
	}{
		{name: "name", mutate: func(req *model.CreateTrainingBlockRequest) { req.Name = "" }, field: "name"},
		{name: "purpose", mutate: func(req *model.CreateTrainingBlockRequest) { req.Purpose = stringPointer(strings.Repeat("p", 501)) }, field: "purpose"},
		{name: "stage count", mutate: func(req *model.CreateTrainingBlockRequest) { req.Stages = req.Stages[:1] }, field: "stages"},
		{name: "stage name", mutate: func(req *model.CreateTrainingBlockRequest) { req.Stages[0].Name = "" }, field: "stages[0].name"},
		{name: "instructions", mutate: func(req *model.CreateTrainingBlockRequest) {
			req.Stages[0].Instructions = stringPointer(strings.Repeat("i", 1001))
		}, field: "stages[0].instructions"},
		{name: "load", mutate: func(req *model.CreateTrainingBlockRequest) { req.Stages[0].LoadLevel = "clinical" }, field: "stages[0].load_level"},
		{name: "count", mutate: func(req *model.CreateTrainingBlockRequest) { req.Stages[0].TargetCount = trainingIntPointer(0) }, field: "stages[0].target_count"},
		{name: "duration", mutate: func(req *model.CreateTrainingBlockRequest) {
			req.Stages[0].TargetDurationMinutes = trainingIntPointer(1441)
		}, field: "stages[0].target_duration_minutes"},
		{name: "intensity", mutate: func(req *model.CreateTrainingBlockRequest) {
			req.Stages[0].TargetIntensityPercent = trainingIntPointer(101)
		}, field: "stages[0].target_intensity_percent"},
		{name: "qualifying exposures", mutate: func(req *model.CreateTrainingBlockRequest) { req.Stages[0].RequiredQualifyingExposures = 0 }, field: "stages[0].required_qualifying_exposures"},
		{name: "operation key", mutate: func(req *model.CreateTrainingBlockRequest) { req.OperationKey = "not-a-uuid" }, field: "operation_key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validTrainingBlockRequest()
			tc.mutate(&req)
			var validation *model.ValidationError
			if err := validateCreateTrainingBlock(req); !errors.As(err, &validation) || validation.Field != tc.field {
				t.Fatalf("expected validation field %q, got %v", tc.field, err)
			}
		})
	}
}

func TestValidateTrainingExposureFields(t *testing.T) {
	valid := func() model.CreateTrainingExposureRequest {
		return model.CreateTrainingExposureRequest{
			PerformedOn: "2026-08-04", ActivityLabel: "Spiking", LoadLevel: model.TrainingLoadEasy,
			SessionOutcome: model.SessionCompletedAsPlanned, ExpectedRevision: 1, OperationKey: uuid.NewString(),
		}
	}
	localNow := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*model.CreateTrainingExposureRequest)
		field  string
	}{
		{name: "date format", mutate: func(req *model.CreateTrainingExposureRequest) { req.PerformedOn = "08/04/2026" }, field: "performed_on"},
		{name: "future date", mutate: func(req *model.CreateTrainingExposureRequest) { req.PerformedOn = "2026-08-05" }, field: "performed_on"},
		{name: "activity label", mutate: func(req *model.CreateTrainingExposureRequest) { req.ActivityLabel = "" }, field: "activity_label"},
		{name: "load", mutate: func(req *model.CreateTrainingExposureRequest) { req.LoadLevel = "maximal" }, field: "load_level"},
		{name: "outcome", mutate: func(req *model.CreateTrainingExposureRequest) { req.SessionOutcome = "cleared" }, field: "session_outcome"},
		{name: "count", mutate: func(req *model.CreateTrainingExposureRequest) { req.PerformedCount = trainingIntPointer(10001) }, field: "performed_count"},
		{name: "duration", mutate: func(req *model.CreateTrainingExposureRequest) { req.DurationMinutes = trainingIntPointer(0) }, field: "duration_minutes"},
		{name: "intensity", mutate: func(req *model.CreateTrainingExposureRequest) { req.PerformedIntensityPercent = trainingIntPointer(0) }, field: "performed_intensity_percent"},
		{name: "notes", mutate: func(req *model.CreateTrainingExposureRequest) { req.Notes = stringPointer(strings.Repeat("n", 1001)) }, field: "notes"},
		{name: "revision", mutate: func(req *model.CreateTrainingExposureRequest) { req.ExpectedRevision = 0 }, field: "expected_revision"},
		{name: "operation key", mutate: func(req *model.CreateTrainingExposureRequest) { req.OperationKey = "bad" }, field: "operation_key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := valid()
			tc.mutate(&req)
			var validation *model.ValidationError
			if err := validateTrainingExposure(req, localNow); !errors.As(err, &validation) || validation.Field != tc.field {
				t.Fatalf("expected validation field %q, got %v", tc.field, err)
			}
		})
	}
}

func TestTrainingBlockServiceRejectsInvalidTransitions(t *testing.T) {
	svc := NewTrainingBlockService(&trainingBlockDAOStub{}, trainingBlockProfileStub{timezone: "UTC"})
	userID, blockID := uuid.New(), uuid.New()
	if _, err := svc.Transition(context.Background(), userID, blockID, model.CreateTrainingTransitionRequest{Action: "auto-advance"}); err == nil {
		t.Fatal("expected action validation")
	}
	if _, err := svc.Transition(context.Background(), userID, blockID, model.CreateTrainingTransitionRequest{Action: model.TransitionRegress}); err == nil {
		t.Fatal("expected regression stage validation")
	}
	longReason := strings.Repeat("r", 501)
	if _, err := svc.Transition(context.Background(), userID, blockID, model.CreateTrainingTransitionRequest{
		Action: model.TransitionArchive, Reason: &longReason, ExpectedRevision: 1, OperationKey: uuid.NewString(),
	}); err == nil {
		t.Fatal("expected reason length validation")
	}
}

func stringPointer(value string) *string     { return &value }
func uuidPointer(value uuid.UUID) *uuid.UUID { return &value }
func trainingIntPointer(value int) *int      { return &value }
