package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
)

// ErrSupabaseAdmin marks non-success responses from the Supabase Admin API.
var (
	ErrSupabaseAdmin             = errors.New("supabase admin api error")
	ErrAccountDeletionIncomplete = errors.New("account deletion incomplete")
)

// AuthUserDeleter removes the user from the auth provider. Separate interface
// so the Supabase admin call is mockable and optional.
type AuthUserDeleter interface {
	DeleteAuthUser(ctx context.Context, userID uuid.UUID) error
}

// UserOperationLocker serializes one user's privacy-sensitive operations
// across API replicas.
type UserOperationLocker interface {
	WithUserLock(ctx context.Context, userID uuid.UUID, operation func(context.Context) error) error
}

type AccountService interface {
	// Delete removes the user's avatar, application data, and auth identity.
	// Every operation is idempotent so a surfaced partial failure can be
	// retried safely.
	Delete(ctx context.Context, userID uuid.UUID) error
}

type accountService struct {
	repo   dao.AccountDAO
	avatar AvatarDeleter
	auth   AuthUserDeleter
	locker UserOperationLocker
}

func NewAccountService(repo dao.AccountDAO, avatar AvatarDeleter, auth AuthUserDeleter, locker ...UserOperationLocker) AccountService {
	var userLocker UserOperationLocker
	if len(locker) > 0 {
		userLocker = locker[0]
	}
	return &accountService{repo: repo, avatar: avatar, auth: auth, locker: userLocker}
}

func (s *accountService) Delete(ctx context.Context, userID uuid.UUID) error {
	deleteAccount := func(ctx context.Context) error {
		if err := s.repo.DeleteUserData(ctx, userID); err != nil {
			return fmt.Errorf("%w: deleting application data: %w", ErrAccountDeletionIncomplete, err)
		}
		if s.avatar != nil {
			if err := s.avatar.Delete(ctx, avatarObjectPaths(userID)...); err != nil {
				return fmt.Errorf("%w: deleting avatar: %w", ErrAccountDeletionIncomplete, err)
			}
		}
		// Auth is deliberately last. If the provider deletes the identity but
		// its response is lost, the user may be unable to authenticate a later
		// retry; all application-held personal data must already be gone.
		if s.auth != nil {
			if err := s.auth.DeleteAuthUser(ctx, userID); err != nil {
				return fmt.Errorf("%w: deleting auth user: %w", ErrAccountDeletionIncomplete, err)
			}
		}
		return nil
	}
	if s.locker != nil {
		if err := s.locker.WithUserLock(ctx, userID, deleteAccount); err != nil {
			if errors.Is(err, ErrAccountDeletionIncomplete) {
				return err
			}
			return fmt.Errorf("%w: serializing deletion: %w", ErrAccountDeletionIncomplete, err)
		}
		return nil
	}
	return deleteAccount(ctx)
}

// SupabaseAdmin implements AuthUserDeleter against the Supabase Admin API.
type SupabaseAdmin struct {
	BaseURL        string // e.g. https://xyz.supabase.co
	ServiceRoleKey string
	Client         *http.Client
}

func NewSupabaseAdmin(baseURL, serviceRoleKey string) *SupabaseAdmin {
	return &SupabaseAdmin{
		BaseURL:        strings.TrimRight(baseURL, "/"),
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
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	// 404 means the auth user is already gone — that's the desired end state.
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("%w: status %d", ErrSupabaseAdmin, resp.StatusCode)
	}
	return nil
}
