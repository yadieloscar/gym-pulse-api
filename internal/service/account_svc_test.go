package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type mockAccountDAO struct {
	deleteFn func(ctx context.Context, userID uuid.UUID) error
	calls    int
	trace    *[]string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func (m *mockAccountDAO) DeleteUserData(ctx context.Context, userID uuid.UUID) error {
	m.calls++
	if m.trace != nil {
		*m.trace = append(*m.trace, "data")
	}
	return m.deleteFn(ctx, userID)
}

type mockAuthDeleter struct {
	err      error
	deleteFn func()
	calls    int
	last     uuid.UUID
	trace    *[]string
}

func (m *mockAuthDeleter) DeleteAuthUser(ctx context.Context, userID uuid.UUID) error {
	m.calls++
	m.last = userID
	if m.trace != nil {
		*m.trace = append(*m.trace, "auth")
	}
	if m.deleteFn != nil {
		m.deleteFn()
	}
	return m.err
}

type mockAvatarDeleter struct {
	err       error
	calls     int
	lastPaths []string
	trace     *[]string
}

func (m *mockAvatarDeleter) Delete(ctx context.Context, objectPaths ...string) error {
	m.calls++
	m.lastPaths = append([]string(nil), objectPaths...)
	if m.trace != nil {
		*m.trace = append(*m.trace, "avatar")
	}
	return m.err
}

func TestAccountService_Delete(t *testing.T) {
	userID := uuid.New()

	t.Run("deletes data then avatar then auth user", func(t *testing.T) {
		var trace []string
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }, trace: &trace}
		avatar := &mockAvatarDeleter{trace: &trace}
		auth := &mockAuthDeleter{trace: &trace}
		svc := NewAccountService(repo, avatar, auth)

		if err := svc.Delete(context.Background(), userID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if avatar.calls != 1 || !equalStrings(avatar.lastPaths, avatarObjectPaths(userID)) {
			t.Errorf("expected one avatar batch delete for all bounded paths; calls=%d paths=%v", avatar.calls, avatar.lastPaths)
		}
		if repo.calls != 1 || auth.calls != 1 || auth.last != userID {
			t.Errorf("expected one data delete and one auth delete for %s; got repo=%d auth=%d last=%s", userID, repo.calls, auth.calls, auth.last)
		}
		if strings.Join(trace, ",") != "data,avatar,auth" {
			t.Errorf("deletion order = %v, want data, avatar, auth", trace)
		}
	})

	t.Run("avatar deletion failure preserves auth so the user can retry", func(t *testing.T) {
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }}
		avatar := &mockAvatarDeleter{err: errors.New("storage down")}
		auth := &mockAuthDeleter{}
		svc := NewAccountService(repo, avatar, auth)

		err := svc.Delete(context.Background(), userID)
		if !errors.Is(err, ErrAccountDeletionIncomplete) || !errors.Is(err, avatar.err) {
			t.Fatalf("expected wrapped retryable avatar error, got %v", err)
		}
		if repo.calls != 1 || auth.calls != 0 {
			t.Errorf("auth deletion must not run after failed storage cleanup; repo=%d auth=%d", repo.calls, auth.calls)
		}
	})

	t.Run("data deletion failure aborts before touching auth and can be retried", func(t *testing.T) {
		dbErr := errors.New("db down")
		attempt := 0
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error {
			attempt++
			if attempt == 1 {
				return dbErr
			}
			return nil
		}}
		avatar := &mockAvatarDeleter{}
		auth := &mockAuthDeleter{}
		svc := NewAccountService(repo, avatar, auth)

		err := svc.Delete(context.Background(), userID)
		if !errors.Is(err, ErrAccountDeletionIncomplete) || !errors.Is(err, dbErr) {
			t.Fatalf("expected wrapped retryable data error, got %v", err)
		}
		if auth.calls != 0 {
			t.Errorf("auth deleter must not be called when data deletion fails")
		}

		if err := svc.Delete(context.Background(), userID); err != nil {
			t.Fatalf("retry should complete, got %v", err)
		}
		if avatar.calls != 1 || repo.calls != 2 || auth.calls != 1 {
			t.Errorf("retry calls avatar=%d data=%d auth=%d, want 1,2,1", avatar.calls, repo.calls, auth.calls)
		}
	})

	t.Run("auth failure is surfaced and the whole sequence can be retried", func(t *testing.T) {
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }}
		avatar := &mockAvatarDeleter{}
		authErr := errors.New("supabase 500")
		auth := &mockAuthDeleter{err: authErr}
		svc := NewAccountService(repo, avatar, auth)

		err := svc.Delete(context.Background(), userID)
		if !errors.Is(err, ErrAccountDeletionIncomplete) || !errors.Is(err, authErr) {
			t.Fatalf("expected surfaced retryable auth error, got %v", err)
		}

		auth.err = nil
		if err := svc.Delete(context.Background(), userID); err != nil {
			t.Fatalf("retry should complete, got %v", err)
		}
		if avatar.calls != 2 || repo.calls != 2 || auth.calls != 2 {
			t.Errorf("retry calls avatar=%d data=%d auth=%d, want 2,2,2", avatar.calls, repo.calls, auth.calls)
		}
	})

	t.Run("ambiguous auth failure happens only after all application-held data is gone", func(t *testing.T) {
		var trace []string
		repo := &mockAccountDAO{
			deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil },
			trace:    &trace,
		}
		avatar := &mockAvatarDeleter{trace: &trace}
		auth := &mockAuthDeleter{
			err:   errors.New("connection reset after provider accepted delete"),
			trace: &trace,
		}
		svc := NewAccountService(repo, avatar, auth)

		err := svc.Delete(context.Background(), userID)
		if !errors.Is(err, ErrAccountDeletionIncomplete) || !errors.Is(err, auth.err) {
			t.Fatalf("expected ambiguous auth error to be surfaced, got %v", err)
		}
		if strings.Join(trace, ",") != "data,avatar,auth" {
			t.Fatalf("ambiguous auth call occurred before personal-data cleanup: %v", trace)
		}
	})

	t.Run("unconfigured external integrations preserve local development behavior", func(t *testing.T) {
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }}
		svc := NewAccountService(repo, nil, nil)
		if err := svc.Delete(context.Background(), userID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.calls != 1 {
			t.Fatalf("application data deletion calls = %d, want 1", repo.calls)
		}
	})
}

func TestSupabaseAdmin_DeleteAuthUser(t *testing.T) {
	userID := uuid.New()
	clientFor := func(status int, inspect func(*http.Request)) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if inspect != nil {
				inspect(req)
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})}
	}

	t.Run("issues an authorized DELETE to the admin endpoint", func(t *testing.T) {
		var gotMethod, gotPath, gotAuth string
		a := NewSupabaseAdmin("https://example.supabase.co/", "service-key")
		a.Client = clientFor(http.StatusOK, func(r *http.Request) {
			gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		})
		if err := a.DeleteAuthUser(context.Background(), userID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodDelete || gotPath != "/auth/v1/admin/users/"+userID.String() {
			t.Errorf("unexpected request: %s %s", gotMethod, gotPath)
		}
		if gotAuth != "Bearer service-key" {
			t.Errorf("missing service-role bearer, got %q", gotAuth)
		}
	})

	t.Run("404 is success — the auth user is already gone", func(t *testing.T) {
		a := NewSupabaseAdmin("https://example.supabase.co", "k")
		a.Client = clientFor(http.StatusNotFound, nil)
		if err := a.DeleteAuthUser(context.Background(), userID); err != nil {
			t.Fatalf("404 should be treated as success, got %v", err)
		}
	})

	t.Run("other error statuses surface", func(t *testing.T) {
		a := NewSupabaseAdmin("https://example.supabase.co", "k")
		a.Client = clientFor(http.StatusInternalServerError, nil)
		if err := a.DeleteAuthUser(context.Background(), userID); err == nil {
			t.Fatal("expected error on 500")
		}
	})
}
