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
	ReplaceAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) (*model.UserProfile, error)
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
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_profiles (id, display_name, avatar_url, onboarding_completed)
		VALUES ($1, $2, $3, COALESCE($4, false))
		ON CONFLICT (id) DO UPDATE
		SET display_name = COALESCE(EXCLUDED.display_name, user_profiles.display_name),
		    avatar_url = COALESCE(EXCLUDED.avatar_url, user_profiles.avatar_url),
		    onboarding_completed = CASE WHEN $4 = true THEN true ELSE user_profiles.onboarding_completed END`,
		userID, profile.DisplayName, profile.AvatarURL, profile.OnboardingCompleted,
	)
	if err != nil {
		return fmt.Errorf("upserting user profile: %w", err)
	}
	return nil
}

// ReplaceAvatar atomically persists an avatar URL and returns the committed
// profile, avoiding a second read whose failure could obscure a successful
// write.
func (r *profileDAO) ReplaceAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) (*model.UserProfile, error) {
	p := &model.UserProfile{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO user_profiles (id, avatar_url, onboarding_completed)
		VALUES ($1, $2, false)
		ON CONFLICT (id) DO UPDATE
		SET avatar_url = EXCLUDED.avatar_url
		RETURNING id, display_name, avatar_url, onboarding_completed, created_at`,
		userID, avatarURL,
	).Scan(&p.ID, &p.DisplayName, &p.AvatarURL, &p.OnboardingCompleted, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("replacing profile avatar: %w", err)
	}
	return p, nil
}
