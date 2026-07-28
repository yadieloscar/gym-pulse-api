package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSupabaseAvatarStorageUpload(t *testing.T) {
	storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
	storage.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPut || r.URL.Path != "/storage/v1/object/avatars/user/avatar-0" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-upsert") != "true" || r.Header.Get("apikey") != "key" || r.Header.Get("Content-Type") != "image/png" {
			t.Errorf("missing storage headers")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	storage.now = func() time.Time { return time.Unix(0, 7) }
	url, err := storage.Upload(context.Background(), "user/avatar-0", "image/png", []byte("png"))
	if err != nil || !strings.HasSuffix(url, "/storage/v1/object/public/avatars/user/avatar-0?v=7") {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestSupabaseAvatarStorageDelete(t *testing.T) {
	t.Run("deletes exact deduplicated objects through one storage API call", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage("https://project.example/", "key", "avatars")
		storage.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodDelete || r.URL.Path != "/storage/v1/object/avatars" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("apikey") != "key" || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("missing storage deletion headers")
			}
			var body struct {
				Prefixes []string `json:"prefixes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			want := []string{"user/avatar", "user/avatar-0", "user/avatar-1"}
			if !equalStrings(body.Prefixes, want) {
				t.Fatalf("prefixes = %v, want %v", body.Prefixes, want)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("[]"))}, nil
		})

		if err := storage.Delete(
			context.Background(),
			"/user/avatar",
			"user/avatar-0",
			"user/avatar-1",
			"user/avatar-0",
			"",
		); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty path set is a no-op", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage(":", "key", "avatars")
		if err := storage.Delete(context.Background(), "", " / "); err != nil {
			t.Fatalf("empty delete should not issue a request: %v", err)
		}
	})

	t.Run("missing object is already deleted", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
		storage.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
		})

		if err := storage.Delete(context.Background(), "user/avatar"); err != nil {
			t.Fatalf("404 should be treated as success, got %v", err)
		}
	})

	t.Run("storage failures are retryable errors", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
		storage.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
		})

		err := storage.Delete(context.Background(), "user/avatar")
		if !errors.Is(err, ErrAvatarDeleteFailed) {
			t.Fatalf("expected ErrAvatarDeleteFailed, got %v", err)
		}
	})
}

func TestAvatarObjectPaths(t *testing.T) {
	userID := uuid.MustParse("dcddc478-f6ec-452e-9a21-3fe5f96036c6")
	all := avatarObjectPaths(userID)
	wantAll := []string{
		userID.String() + "/avatar",
		userID.String() + "/avatar-0",
		userID.String() + "/avatar-1",
	}
	if !equalStrings(all, wantAll) {
		t.Fatalf("all paths = %v, want %v", all, wantAll)
	}

	slotZeroURL := "https://project.example/storage/v1/object/public/avatars/" + userID.String() + "/avatar-0?v=1"
	if got := nextAvatarObjectPath(userID, &slotZeroURL); got != userID.String()+"/avatar-1" {
		t.Fatalf("next path from slot zero = %q", got)
	}
	legacyURL := "https://project.example/storage/v1/object/public/avatars/" + userID.String() + "/avatar?v=1"
	if got := nextAvatarObjectPath(userID, &legacyURL); got != userID.String()+"/avatar-0" {
		t.Fatalf("next path from legacy = %q", got)
	}
}

func TestSupabaseAvatarStorageFailureBoundaries(t *testing.T) {
	t.Run("uses the production bucket default", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage("https://project.example/", "key", "")
		if storage.bucket != "avatars" || storage.baseURL != "https://project.example" {
			t.Fatalf("unexpected storage defaults: bucket=%q base=%q", storage.bucket, storage.baseURL)
		}
	})

	t.Run("upload rejects an invalid endpoint", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage(":", "key", "avatars")
		if _, err := storage.Upload(context.Background(), "user/avatar", "image/png", []byte("png")); !errors.Is(err, ErrAvatarUploadFailed) {
			t.Fatalf("expected upload failure, got %v", err)
		}
	})

	t.Run("upload wraps transport errors", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
		storage.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network unavailable")
		})
		if _, err := storage.Upload(context.Background(), "user/avatar", "image/png", []byte("png")); !errors.Is(err, ErrAvatarUploadFailed) {
			t.Fatalf("expected upload failure, got %v", err)
		}
	})

	t.Run("upload rejects non-success status", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
		storage.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("upstream failed")),
			}, nil
		})
		if _, err := storage.Upload(context.Background(), "user/avatar", "image/png", []byte("png")); !errors.Is(err, ErrAvatarUploadFailed) {
			t.Fatalf("expected upload failure, got %v", err)
		}
	})

	t.Run("delete rejects an invalid endpoint", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage(":", "key", "avatars")
		if err := storage.Delete(context.Background(), "user/avatar"); !errors.Is(err, ErrAvatarDeleteFailed) {
			t.Fatalf("expected delete failure, got %v", err)
		}
	})

	t.Run("delete wraps transport errors", func(t *testing.T) {
		storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
		storage.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network unavailable")
		})
		if err := storage.Delete(context.Background(), "user/avatar"); !errors.Is(err, ErrAvatarDeleteFailed) {
			t.Fatalf("expected delete failure, got %v", err)
		}
	})
}
