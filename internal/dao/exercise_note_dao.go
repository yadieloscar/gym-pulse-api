package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ErrExerciseNoteLimit reports that an athlete who already keeps
// model.MaxExerciseNotes notes tried to save one for another exercise.
var ErrExerciseNoteLimit = errors.New("exercise note limit reached")

// ExerciseNoteDAO owns the exercise_notes table: one short private note per
// athlete and exercise.
type ExerciseNoteDAO interface {
	// List returns all of the athlete's notes ordered by exercise key. The
	// result is never nil.
	List(ctx context.Context, userID uuid.UUID) ([]model.ExerciseNote, error)
	// Save stores note.Note for note.ExerciseKey, replacing any earlier note
	// and name, and returns the stored row. It returns ErrExerciseNoteLimit
	// when the key is new and the athlete already has model.MaxExerciseNotes
	// notes.
	Save(ctx context.Context, userID uuid.UUID, note model.ExerciseNote) (*model.ExerciseNote, error)
	// Delete removes the athlete's note for exerciseKey. A key with no note
	// is not an error.
	Delete(ctx context.Context, userID uuid.UUID, exerciseKey string) error
}

type exerciseNoteDAO struct{ pool *pgxpool.Pool }

// NewExerciseNoteDAO returns an ExerciseNoteDAO.
func NewExerciseNoteDAO(pool *pgxpool.Pool) ExerciseNoteDAO {
	return &exerciseNoteDAO{pool: pool}
}

func (r *exerciseNoteDAO) List(ctx context.Context, userID uuid.UUID) ([]model.ExerciseNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT exercise_key, exercise_name, note, updated_at
		FROM exercise_notes
		WHERE user_id=$1
		ORDER BY exercise_key`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying exercise notes: %w", err)
	}
	defer rows.Close()
	notes := []model.ExerciseNote{}
	for rows.Next() {
		var note model.ExerciseNote
		if err := rows.Scan(&note.ExerciseKey, &note.ExerciseName, &note.Note, &note.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning exercise note: %w", err)
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading exercise notes: %w", err)
	}
	return notes, nil
}

func (r *exerciseNoteDAO) Save(ctx context.Context, userID uuid.UUID, note model.ExerciseNote) (*model.ExerciseNote, error) {
	// The SELECT yields a row only for a key the athlete already has or
	// while they are under the limit, so a new key past the limit inserts
	// nothing and returns no row. Two first saves racing at the limit can
	// both pass the count; the limit bounds the list, it is not a quota.
	saved := &model.ExerciseNote{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO exercise_notes (user_id, exercise_key, exercise_name, note)
		SELECT $1, $2, $3, $4
		WHERE EXISTS (SELECT 1 FROM exercise_notes WHERE user_id=$1 AND exercise_key=$2)
		   OR (SELECT count(*) FROM exercise_notes WHERE user_id=$1) < $5
		ON CONFLICT (user_id, exercise_key) DO UPDATE
		SET exercise_name=EXCLUDED.exercise_name, note=EXCLUDED.note, updated_at=now()
		RETURNING exercise_key, exercise_name, note, updated_at`,
		userID, note.ExerciseKey, note.ExerciseName, note.Note, model.MaxExerciseNotes).
		Scan(&saved.ExerciseKey, &saved.ExerciseName, &saved.Note, &saved.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExerciseNoteLimit
	}
	if err != nil {
		return nil, fmt.Errorf("saving exercise note: %w", err)
	}
	return saved, nil
}

func (r *exerciseNoteDAO) Delete(ctx context.Context, userID uuid.UUID, exerciseKey string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM exercise_notes WHERE user_id=$1 AND exercise_key=$2`, userID, exerciseKey); err != nil {
		return fmt.Errorf("deleting exercise note: %w", err)
	}
	return nil
}
