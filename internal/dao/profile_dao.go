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

// ProfileDAO defines operations on the user_profiles table.
type ProfileDAO interface {
	Get(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error)
	Upsert(ctx context.Context, userID uuid.UUID, profile *model.UpdateProfileRequest) error
}

type profileDAO struct {
	pool *pgxpool.Pool
}

// NewProfileDAO creates a new ProfileDAO backed by the given connection pool.
func NewProfileDAO(pool *pgxpool.Pool) ProfileDAO {
	return &profileDAO{pool: pool}
}

func (r *profileDAO) Get(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	p := &model.UserProfile{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, display_name, avatar_url, onboarding_completed, created_at
		FROM user_profiles
		WHERE id = $1`,
		userID,
	).Scan(&p.ID, &p.DisplayName, &p.AvatarURL, &p.OnboardingCompleted, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &model.UserProfile{
				ID: userID,
			}, nil
		}
		return nil, fmt.Errorf("querying user profile: %w", err)
	}
	return p, nil
}

func (r *profileDAO) Upsert(ctx context.Context, userID uuid.UUID, profile *model.UpdateProfileRequest) error {
	// Ensure the user exists in auth.users first (local dev / Supabase compatibility)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO auth.users (id)
		VALUES ($1)
		ON CONFLICT (id) DO NOTHING`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("ensuring user in auth.users: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO user_profiles (id, display_name, avatar_url, onboarding_completed)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (id) DO UPDATE
		SET display_name = COALESCE(EXCLUDED.display_name, user_profiles.display_name),
		    avatar_url = COALESCE(EXCLUDED.avatar_url, user_profiles.avatar_url),
		    onboarding_completed = true`,
		userID, profile.DisplayName, profile.AvatarURL,
	)
	if err != nil {
		return fmt.Errorf("upserting user profile: %w", err)
	}
	return nil
}
