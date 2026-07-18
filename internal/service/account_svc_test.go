package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type mockAccountDAO struct {
	deleteFn func(ctx context.Context, userID uuid.UUID) error
	calls    int
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func (m *mockAccountDAO) DeleteUserData(ctx context.Context, userID uuid.UUID) error {
	m.calls++
	return m.deleteFn(ctx, userID)
}

type mockAuthDeleter struct {
	err   error
	calls int
	last  uuid.UUID
}

func (m *mockAuthDeleter) DeleteAuthUser(ctx context.Context, userID uuid.UUID) error {
	m.calls++
	m.last = userID
	return m.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAccountService_Delete(t *testing.T) {
	userID := uuid.New()

	t.Run("deletes data then the auth user", func(t *testing.T) {
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }}
		auth := &mockAuthDeleter{}
		svc := NewAccountService(repo, auth, discardLogger())

		if err := svc.Delete(context.Background(), userID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.calls != 1 || auth.calls != 1 || auth.last != userID {
			t.Errorf("expected one data delete and one auth delete for %s; got repo=%d auth=%d last=%s", userID, repo.calls, auth.calls, auth.last)
		}
	})

	t.Run("data deletion failure aborts before touching auth", func(t *testing.T) {
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error { return errors.New("db down") }}
		auth := &mockAuthDeleter{}
		svc := NewAccountService(repo, auth, discardLogger())

		if err := svc.Delete(context.Background(), userID); err == nil {
			t.Fatal("expected error")
		}
		if auth.calls != 0 {
			t.Errorf("auth deleter must not be called when data deletion fails")
		}
	})

	t.Run("auth failure is logged, not surfaced — data is already gone", func(t *testing.T) {
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }}
		auth := &mockAuthDeleter{err: errors.New("supabase 500")}
		svc := NewAccountService(repo, auth, discardLogger())

		if err := svc.Delete(context.Background(), userID); err != nil {
			t.Fatalf("expected success despite auth failure, got %v", err)
		}
	})

	t.Run("nil auth deleter (unconfigured) deletes data only", func(t *testing.T) {
		repo := &mockAccountDAO{deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }}
		svc := NewAccountService(repo, nil, discardLogger())

		if err := svc.Delete(context.Background(), userID); err != nil {
			t.Fatalf("unexpected error: %v", err)
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
		a := NewSupabaseAdmin("https://example.supabase.co", "service-key")
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
