package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type trainingBlockHandlerService struct {
	listLimit int
	created   model.CreateTrainingBlockRequest
	block     model.TrainingBlock
}

func (s *trainingBlockHandlerService) List(_ context.Context, _ uuid.UUID, _ string, limit, _ int) (*model.TrainingBlockList, error) {
	s.listLimit = limit
	return &model.TrainingBlockList{TrainingBlocks: []model.TrainingBlockSummary{}}, nil
}
func (s *trainingBlockHandlerService) Get(context.Context, uuid.UUID, uuid.UUID) (*model.TrainingBlock, error) {
	return &s.block, nil
}
func (s *trainingBlockHandlerService) Create(_ context.Context, _ uuid.UUID, req model.CreateTrainingBlockRequest) (*model.TrainingBlock, error) {
	s.created = req
	return &s.block, nil
}
func (s *trainingBlockHandlerService) AddExposure(context.Context, uuid.UUID, uuid.UUID, model.CreateTrainingExposureRequest) (*model.TrainingBlock, error) {
	return &s.block, nil
}
func (s *trainingBlockHandlerService) RecordNextMorning(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.RecordNextMorningRequest) (*model.TrainingBlock, error) {
	return &s.block, nil
}
func (s *trainingBlockHandlerService) Transition(context.Context, uuid.UUID, uuid.UUID, model.CreateTrainingTransitionRequest) (*model.TrainingBlock, error) {
	return &s.block, nil
}

func TestTrainingBlockHandlerContracts(t *testing.T) {
	svc := &trainingBlockHandlerService{block: model.TrainingBlock{TrainingBlockSummary: model.TrainingBlockSummary{ID: uuid.New()}}}
	h := NewTrainingBlockHandler(svc)
	userID, blockID, exposureID, operationKey := uuid.New(), uuid.New(), uuid.New(), uuid.NewString()

	list := httptest.NewRecorder()
	h.List(list, guidedHandlerRequest(t, http.MethodGet, "/?status=active&limit=12&offset=0", nil, userID, "", nil))
	if list.Code != http.StatusOK || svc.listLimit != 12 {
		t.Fatalf("list status=%d limit=%d body=%s", list.Code, svc.listLimit, list.Body.String())
	}

	create := httptest.NewRecorder()
	req := model.CreateTrainingBlockRequest{Name: "Block", OperationKey: operationKey}
	h.Create(create, guidedHandlerRequest(t, http.MethodPost, "/", req, userID, operationKey, nil))
	if create.Code != http.StatusCreated || svc.created.OperationKey != operationKey {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}

	next := httptest.NewRecorder()
	h.RecordNextMorning(next, guidedHandlerRequest(t, http.MethodPost, "/", model.RecordNextMorningRequest{OperationKey: operationKey}, userID, operationKey, map[string]string{"block_id": blockID.String(), "exposure_id": exposureID.String()}))
	if next.Code != http.StatusOK {
		t.Fatalf("next-morning status=%d body=%s", next.Code, next.Body.String())
	}
}

func TestTrainingBlockHandlerRejectsBadQueriesAndKeys(t *testing.T) {
	h := NewTrainingBlockHandler(&trainingBlockHandlerService{})
	userID := uuid.New()
	badQuery := httptest.NewRecorder()
	h.List(badQuery, guidedHandlerRequest(t, http.MethodGet, "/?limit=lots", nil, userID, "", nil))
	if badQuery.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad query status=%d body=%s", badQuery.Code, badQuery.Body.String())
	}
	badKey := httptest.NewRecorder()
	h.Create(badKey, guidedHandlerRequest(t, http.MethodPost, "/", model.CreateTrainingBlockRequest{OperationKey: "body"}, userID, "header", nil))
	if badKey.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad key status=%d body=%s", badKey.Code, badKey.Body.String())
	}
}
