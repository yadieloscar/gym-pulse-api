package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
)

// ErrSupabaseAdmin marks non-success responses from the Supabase Admin API.
var ErrSupabaseAdmin = errors.New("supabase admin api error")

// AuthUserDeleter removes the user from the auth provider. Separate interface
// so the Supabase admin call is mockable and optional.
type AuthUserDeleter interface {
	DeleteAuthUser(ctx context.Context, userID uuid.UUID) error
}

type AccountService interface {
	// Delete removes all application data for the user and, when an auth
	// deleter is configured, the Supabase auth user itself. Data deletion is
	// authoritative: an auth-provider failure is logged, not surfaced — the
	// user's data is already gone and retrying auth deletion is an ops task.
	Delete(ctx context.Context, userID uuid.UUID) error
}

type accountService struct {
	repo   dao.AccountDAO
	auth   AuthUserDeleter // nil when SUPABASE_URL/SERVICE_ROLE_KEY are unset
	logger *slog.Logger
}

func NewAccountService(repo dao.AccountDAO, auth AuthUserDeleter, logger *slog.Logger) AccountService {
	return &accountService{repo: repo, auth: auth, logger: logger}
}

func (s *accountService) Delete(ctx context.Context, userID uuid.UUID) error {
	if err := s.repo.DeleteUserData(ctx, userID); err != nil {
		return err
	}
	if s.auth == nil {
		s.logger.Warn("account deleted without auth-provider removal (SUPABASE_URL/SUPABASE_SERVICE_ROLE_KEY unset)", "user_id", userID)
		return nil
	}
	if err := s.auth.DeleteAuthUser(ctx, userID); err != nil {
		s.logger.Error("account data deleted but auth user removal failed", "user_id", userID, "error", err)
	}
	return nil
}

// SupabaseAdmin implements AuthUserDeleter against the Supabase Admin API.
type SupabaseAdmin struct {
	BaseURL        string // e.g. https://xyz.supabase.co
	ServiceRoleKey string
	Client         *http.Client
}

func NewSupabaseAdmin(baseURL, serviceRoleKey string) *SupabaseAdmin {
	return &SupabaseAdmin{
		BaseURL:        baseURL,
		ServiceRoleKey: serviceRoleKey,
		Client:         &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *SupabaseAdmin) DeleteAuthUser(ctx context.Context, userID uuid.UUID) error {
	url := fmt.Sprintf("%s/auth/v1/admin/users/%s", a.BaseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("building supabase admin request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.ServiceRoleKey)
	req.Header.Set("apikey", a.ServiceRoleKey)

	resp, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("calling supabase admin api: %w", err)
	}
	defer resp.Body.Close()

	// 404 means the auth user is already gone — that's the desired end state.
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("%w: status %d", ErrSupabaseAdmin, resp.StatusCode)
	}
	return nil
}
