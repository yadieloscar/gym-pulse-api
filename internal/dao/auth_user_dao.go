package dao

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authUserQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// AuthUserDAO checks the authoritative Supabase Auth identity root.
type AuthUserDAO struct {
	database authUserQuerier
}

func NewAuthUserDAO(pool *pgxpool.Pool) *AuthUserDAO {
	return &AuthUserDAO{database: pool}
}

// Exists reports whether the JWT subject still has an active auth identity.
// It calls a function owned by the schema owner (migration 043) because hosted
// Supabase does not let the runtime role be granted the auth schema.
func (r *AuthUserDAO) Exists(ctx context.Context, userID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.database.QueryRow(
		ctx,
		`SELECT gym_pulse_private.auth_user_exists($1)`,
		userID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking auth user existence: %w", err)
	}
	return exists, nil
}
