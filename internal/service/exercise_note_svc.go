package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ExerciseNoteService lists and saves an athlete's exercise notes: one short
// private note per exercise, addressed by the exercise's name. A save
// replaces the whole note, so it is idempotent and carries no operation key
// or revision; the last save wins. See docs/CONTRACTS.md "Exercise notes".
type ExerciseNoteService interface {
	// List returns all of the athlete's notes ordered by exercise key.
	List(ctx context.Context, userID uuid.UUID) (*model.ExerciseNotesResponse, error)
	// Save stores req.Note for the exercise named req.ExerciseName and
	// returns the note as it now stands. An empty note clears it; the
	// returned note then has Note "".
	Save(ctx context.Context, userID uuid.UUID, req model.SaveExerciseNoteRequest) (*model.ExerciseNote, error)
}

type exerciseNoteService struct {
	notes dao.ExerciseNoteDAO
	now   func() time.Time
}

// NewExerciseNoteService returns an ExerciseNoteService.
func NewExerciseNoteService(notes dao.ExerciseNoteDAO) ExerciseNoteService {
	return &exerciseNoteService{notes: notes, now: time.Now}
}

func (s *exerciseNoteService) List(ctx context.Context, userID uuid.UUID) (*model.ExerciseNotesResponse, error) {
	notes, err := s.notes.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &model.ExerciseNotesResponse{ExerciseNotes: notes}, nil
}

func (s *exerciseNoteService) Save(ctx context.Context, userID uuid.UUID, req model.SaveExerciseNoteRequest) (*model.ExerciseNote, error) {
	name := model.CleanExerciseName(req.ExerciseName)
	if name == "" {
		return nil, &model.ValidationError{Message: "exercise_name is required", Field: "exercise_name"}
	}
	key := model.ExerciseNoteKey(name)
	// Lowercasing can change a name's length in a few scripts, so both the
	// name and the key are held to the limit the table checks.
	if utf8.RuneCountInString(name) > model.MaxExerciseNameLength || utf8.RuneCountInString(key) > model.MaxExerciseNameLength {
		return nil, &model.ValidationError{Message: "exercise_name must be at most 120 characters", Field: "exercise_name"}
	}
	text := strings.TrimSpace(req.Note)
	if utf8.RuneCountInString(text) > model.MaxExerciseNoteLength {
		return nil, &model.ValidationError{Message: "note must be at most 200 characters", Field: "note"}
	}
	if text == "" {
		if err := s.notes.Delete(ctx, userID, key); err != nil {
			return nil, err
		}
		return &model.ExerciseNote{ExerciseKey: key, ExerciseName: name, Note: "", UpdatedAt: s.now().UTC()}, nil
	}
	saved, err := s.notes.Save(ctx, userID, model.ExerciseNote{ExerciseKey: key, ExerciseName: name, Note: text})
	if errors.Is(err, dao.ErrExerciseNoteLimit) {
		return nil, &model.ValidationError{Message: "you can keep notes for at most 500 exercises", Field: "exercise_name"}
	}
	if err != nil {
		return nil, err
	}
	return saved, nil
}
