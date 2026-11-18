package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// StretchLibraryService reads and replaces the stretch moves and routines an
// athlete wrote. The library is one document; the service validates all of
// it and the DAO stores it with its revision and idempotency record. It does
// not know the moves and routines the app bundles. See docs/CONTRACTS.md
// "Stretch library".
type StretchLibraryService interface {
	// Get returns the athlete's library, or the empty library at revision 0
	// when they never saved one.
	Get(ctx context.Context, userID uuid.UUID) (*model.StretchLibrary, error)
	// Save replaces the whole library and returns it at its new revision.
	Save(ctx context.Context, userID uuid.UUID, req model.SaveStretchLibraryRequest) (*model.StretchLibrary, error)
}

type stretchLibraryService struct {
	libraries dao.StretchLibraryDAO
}

// NewStretchLibraryService returns a StretchLibraryService.
func NewStretchLibraryService(libraries dao.StretchLibraryDAO) StretchLibraryService {
	return &stretchLibraryService{libraries: libraries}
}

func (s *stretchLibraryService) Get(ctx context.Context, userID uuid.UUID) (*model.StretchLibrary, error) {
	return s.libraries.Get(ctx, userID)
}

func (s *stretchLibraryService) Save(ctx context.Context, userID uuid.UUID, req model.SaveStretchLibraryRequest) (*model.StretchLibrary, error) {
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return nil, err
	}
	if req.ExpectedRevision < 0 {
		return nil, &model.ValidationError{Message: "expected_revision must be 0 or greater", Field: "expected_revision"}
	}
	if err := normalizeStretchMoves(req.Moves); err != nil {
		return nil, err
	}
	if err := normalizeStretchRoutines(req.Routines, req.Moves); err != nil {
		return nil, err
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	return s.libraries.Save(ctx, userID, req.Moves, req.Routines, req.ExpectedRevision, req.OperationKey, hash)
}

// stretchListError names the list at fault and the zero-based position of
// the entry in it.
func stretchListError(field string, index int, message string) error {
	return &model.ValidationError{Message: message, Field: field, Index: &index}
}

// validOwnStretchID reports whether id is a slug of 4 to 64 characters that
// starts with the prefix of the athlete's own entries.
func validOwnStretchID(id string) bool {
	return len(id) >= 4 && len(id) <= maxRoutineIDLength &&
		strings.HasPrefix(id, model.OwnStretchIDPrefix) && sportIDPattern.MatchString(id)
}

// normalizeStretchName trims a move or routine name and reports whether it
// holds 1 to 60 characters.
func normalizeStretchName(name *string) bool {
	*name = strings.TrimSpace(*name)
	length := utf8.RuneCountInString(*name)
	return length >= 1 && length <= model.MaxStretchNameLength
}

func stretchHoldInRange(seconds int) bool {
	return seconds >= model.MinStretchHoldSeconds && seconds <= model.MaxStretchHoldSeconds
}

func stretchRepsInRange(reps int) bool {
	return reps >= model.MinStretchReps && reps <= model.MaxStretchReps
}

// normalizeStretchMoves trims and validates the moves in place.
func normalizeStretchMoves(moves []model.StretchMove) error {
	const field = "moves"
	if moves == nil {
		return &model.ValidationError{Message: "moves is required", Field: field}
	}
	if len(moves) > model.MaxStretchMoves {
		return &model.ValidationError{Message: fmt.Sprintf("moves holds at most %d entries", model.MaxStretchMoves), Field: field}
	}
	seen := make(map[string]bool, len(moves))
	for i := range moves {
		move := &moves[i]
		switch {
		case !validOwnStretchID(move.ID):
			return stretchListError(field, i, "id must be a slug of 4 to 64 characters that starts with my-")
		case seen[move.ID]:
			return stretchListError(field, i, "id is already used by another move")
		case !normalizeStretchName(&move.Name):
			return stretchListError(field, i, "name must be between 1 and 60 characters")
		}
		seen[move.ID] = true
		move.Cue = strings.TrimSpace(move.Cue)
		if utf8.RuneCountInString(move.Cue) > model.MaxStretchCueLength {
			return stretchListError(field, i, "cue must not exceed 200 characters")
		}
		move.Areas = strings.TrimSpace(move.Areas)
		if utf8.RuneCountInString(move.Areas) > model.MaxStretchAreasLength {
			return stretchListError(field, i, "areas must not exceed 60 characters")
		}
		switch move.Mode {
		case model.StretchModeHold:
			if move.HoldSeconds == nil || !stretchHoldInRange(*move.HoldSeconds) {
				return stretchListError(field, i, "a hold move needs hold_seconds between 5 and 600")
			}
			if move.Reps != nil {
				return stretchListError(field, i, "a hold move cannot carry reps")
			}
		case model.StretchModeReps:
			if move.Reps == nil || !stretchRepsInRange(*move.Reps) {
				return stretchListError(field, i, "a reps move needs reps between 1 and 100")
			}
			if move.HoldSeconds != nil {
				return stretchListError(field, i, "a reps move cannot carry hold_seconds")
			}
		default:
			return stretchListError(field, i, "mode must be hold or reps")
		}
	}
	return nil
}

// normalizeStretchRoutines trims and validates the routines in place. An
// item that names one of the athlete's own moves must find it in moves; any
// other move id is one the app bundles and is not checked.
func normalizeStretchRoutines(routines []model.StretchRoutine, moves []model.StretchMove) error {
	const field = "routines"
	if routines == nil {
		return &model.ValidationError{Message: "routines is required", Field: field}
	}
	if len(routines) > model.MaxStretchRoutines {
		return &model.ValidationError{Message: fmt.Sprintf("routines holds at most %d entries", model.MaxStretchRoutines), Field: field}
	}
	own := make(map[string]bool, len(moves))
	for _, move := range moves {
		own[move.ID] = true
	}
	seen := make(map[string]bool, len(routines))
	for i := range routines {
		routine := &routines[i]
		switch {
		case !validOwnStretchID(routine.ID):
			return stretchListError(field, i, "id must be a slug of 4 to 64 characters that starts with my-")
		case seen[routine.ID]:
			return stretchListError(field, i, "id is already used by another routine")
		case !normalizeStretchName(&routine.Name):
			return stretchListError(field, i, "name must be between 1 and 60 characters")
		case len(routine.Items) < 1 || len(routine.Items) > model.MaxStretchRoutineItems:
			return stretchListError(field, i, "items must hold between 1 and 30 moves")
		}
		seen[routine.ID] = true
		for _, item := range routine.Items {
			switch {
			case len(item.MoveID) > maxRoutineIDLength || !sportIDPattern.MatchString(item.MoveID):
				return stretchListError(field, i, "move_id must be a slug of 1 to 64 characters")
			case strings.HasPrefix(item.MoveID, model.OwnStretchIDPrefix) && !own[item.MoveID]:
				return stretchListError(field, i, "move_id names a move that is not in moves")
			case item.HoldSeconds != nil && item.Reps != nil:
				return stretchListError(field, i, "an item sets at most one of hold_seconds and reps")
			case item.HoldSeconds != nil && !stretchHoldInRange(*item.HoldSeconds):
				return stretchListError(field, i, "hold_seconds must be between 5 and 600")
			case item.Reps != nil && !stretchRepsInRange(*item.Reps):
				return stretchListError(field, i, "reps must be between 1 and 100")
			}
		}
	}
	return nil
}
