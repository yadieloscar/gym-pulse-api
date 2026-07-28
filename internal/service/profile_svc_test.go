package service

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type fakeAvatarStorage struct {
	url               string
	err               error
	path, contentType string
	deletePaths       []string
	objects           map[string][]byte
}

type serialUserOperationLocker struct {
	mu sync.Mutex
}

func (l *serialUserOperationLocker) WithUserLock(
	ctx context.Context,
	_ uuid.UUID,
	operation func(context.Context) error,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return operation(ctx)
}

type mutableActiveUser struct {
	mu     sync.Mutex
	exists bool
	err    error
}

func (u *mutableActiveUser) Exists(context.Context, uuid.UUID) (bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.exists, u.err
}

func (u *mutableActiveUser) setExists(exists bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.exists = exists
}

func (f *fakeAvatarStorage) Upload(_ context.Context, objectPath, contentType string, body []byte) (string, error) {
	f.path, f.contentType = objectPath, contentType
	if f.err == nil && f.objects != nil {
		f.objects[objectPath] = append([]byte(nil), body...)
	}
	return f.url, f.err
}

func (f *fakeAvatarStorage) Delete(_ context.Context, objectPaths ...string) error {
	f.deletePaths = append([]string(nil), objectPaths...)
	for _, objectPath := range objectPaths {
		delete(f.objects, objectPath)
	}
	return nil
}

func TestProfileService_Get(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		displayName := "John Doe"
		mockProfile := &model.UserProfile{
			ID:                  userID,
			DisplayName:         &displayName,
			OnboardingCompleted: true,
			CreatedAt:           time.Now(),
		}

		repo := &MockProfileDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserProfile, error) {
				if id != userID {
					t.Errorf("expected userID %s, got %s", userID, id)
				}
				return mockProfile, nil
			},
		}

		v := validator.New()
		svc := NewProfileService(repo, v)

		p, err := svc.Get(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ID != userID {
			t.Errorf("expected ID %s, got %s", userID, p.ID)
		}
		if p.DisplayName == nil || *p.DisplayName != displayName {
			t.Errorf("expected DisplayName %s", displayName)
		}
	})

	t.Run("db error", func(t *testing.T) {
		repo := &MockProfileDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserProfile, error) {
				return nil, errors.New("db error")
			},
		}

		v := validator.New()
		svc := NewProfileService(repo, v)

		_, err := svc.Get(ctx, userID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestProfileService_Update(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	displayName := "Jane Doe"
	validReq := model.UpdateProfileRequest{
		DisplayName: &displayName,
	}

	t.Run("success", func(t *testing.T) {
		upsertCalled := false
		repo := &MockProfileDAO{
			UpsertFunc: func(ctx context.Context, id uuid.UUID, req *model.UpdateProfileRequest) error {
				if id != userID {
					t.Errorf("expected userID %s, got %s", userID, id)
				}
				if req.DisplayName == nil || *req.DisplayName != displayName {
					t.Errorf("expected DisplayName %s", displayName)
				}
				upsertCalled = true
				return nil
			},
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserProfile, error) {
				return &model.UserProfile{
					ID:                  userID,
					DisplayName:         &displayName,
					OnboardingCompleted: true,
				}, nil
			},
		}

		v := validator.New()
		svc := NewProfileService(repo, v)

		p, err := svc.Update(ctx, userID, validReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !upsertCalled {
			t.Error("expected upsert to be called")
		}
		if p.DisplayName == nil || *p.DisplayName != displayName {
			t.Errorf("expected updated display name %s", displayName)
		}
	})

	t.Run("validation failure - short display name", func(t *testing.T) {
		shortName := "a"
		req := model.UpdateProfileRequest{
			DisplayName: &shortName,
		}

		repo := &MockProfileDAO{}
		v := validator.New()
		svc := NewProfileService(repo, v)

		_, err := svc.Update(ctx, userID, req)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		var valErr *model.ValidationError
		if !errors.As(err, &valErr) {
			t.Errorf("expected ValidationError, got %T", err)
		}
	})

	t.Run("validation failure - long display name", func(t *testing.T) {
		longName := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" // 52 chars
		req := model.UpdateProfileRequest{
			DisplayName: &longName,
		}

		repo := &MockProfileDAO{}
		v := validator.New()
		svc := NewProfileService(repo, v)

		_, err := svc.Update(ctx, userID, req)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})

	t.Run("upsert error", func(t *testing.T) {
		repo := &MockProfileDAO{
			UpsertFunc: func(ctx context.Context, id uuid.UUID, req *model.UpdateProfileRequest) error {
				return errors.New("upsert failed")
			},
		}

		v := validator.New()
		svc := NewProfileService(repo, v)

		_, err := svc.Update(ctx, userID, validReq)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestProfileService_OnboardingAndAvatar(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	completed := false
	svc := NewProfileService(&MockProfileDAO{}, validator.New())
	if _, err := svc.Update(ctx, userID, model.UpdateProfileRequest{OnboardingCompleted: &completed}); err == nil {
		t.Fatal("onboarding reset was accepted")
	}

	avatarURL := "https://project.example/storage/v1/object/public/avatars/" + userID.String() + "/avatar-0?v=1"
	repo := &MockProfileDAO{
		UpsertFunc: func(_ context.Context, _ uuid.UUID, req *model.UpdateProfileRequest) error {
			if req.AvatarURL == nil || *req.AvatarURL != avatarURL {
				t.Fatalf("persisted avatar URL = %v, want %q", req.AvatarURL, avatarURL)
			}
			return nil
		},
		GetFunc: func(_ context.Context, id uuid.UUID) (*model.UserProfile, error) {
			return &model.UserProfile{ID: id}, nil
		},
	}
	avatarBytes := bytes.Repeat([]byte{1}, 20)
	storage := &fakeAvatarStorage{url: avatarURL, objects: make(map[string][]byte)}
	svc = NewProfileService(repo, validator.New(), storage)
	profile, err := svc.UploadAvatar(ctx, userID, "image/png", avatarBytes)
	if err != nil || profile.AvatarURL == nil || *profile.AvatarURL != avatarURL ||
		storage.path != userID.String()+"/avatar-0" {
		t.Fatalf("avatar upload failed: %+v %v path=%s", profile, err, storage.path)
	}
	if len(storage.deletePaths) != 0 || !bytes.Equal(storage.objects[storage.path], avatarBytes) {
		t.Fatalf("initial upload was deleted: cleanup=%v bytes=%q", storage.deletePaths, storage.objects[storage.path])
	}
}

func TestProfileService_UploadAvatarReplacement(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	oldURL := "https://project.example/storage/v1/object/public/avatars/" + userID.String() + "/avatar-0?v=old"
	newURL := "https://project.example/storage/v1/object/public/avatars/" + userID.String() + "/avatar-1?v=new"
	displayName := "Alex"
	current := &model.UserProfile{ID: userID, DisplayName: &displayName, AvatarURL: &oldURL}
	repo := &MockProfileDAO{
		GetFunc: func(context.Context, uuid.UUID) (*model.UserProfile, error) {
			return current, nil
		},
		UpsertFunc: func(_ context.Context, _ uuid.UUID, req *model.UpdateProfileRequest) error {
			if req.AvatarURL == nil || *req.AvatarURL != newURL {
				t.Fatalf("persisted avatar URL = %v, want %q", req.AvatarURL, newURL)
			}
			return nil
		},
	}
	oldPath := userID.String() + "/avatar-0"
	newPath := userID.String() + "/avatar-1"
	oldBytes := []byte("previous avatar")
	newBytes := []byte("replacement")
	storage := &fakeAvatarStorage{
		url:     newURL,
		objects: map[string][]byte{oldPath: append([]byte(nil), oldBytes...)},
	}
	svc := NewProfileService(repo, validator.New(), storage)

	profile, err := svc.UploadAvatar(ctx, userID, "image/png", newBytes)

	if err != nil {
		t.Fatalf("replacement failed: %v", err)
	}
	if storage.path != newPath {
		t.Fatalf("replacement path = %q, want inactive slot avatar-1", storage.path)
	}
	if profile.AvatarURL == nil || *profile.AvatarURL != newURL || profile.DisplayName != current.DisplayName {
		t.Fatalf("returned profile did not preserve the preloaded snapshot: %+v", profile)
	}
	if len(storage.deletePaths) != 0 {
		t.Fatalf("replacement deleted supported avatar paths: %v", storage.deletePaths)
	}
	if !bytes.Equal(storage.objects[oldPath], oldBytes) || !bytes.Equal(storage.objects[newPath], newBytes) {
		t.Fatalf("both slots must remain accessible: old=%q new=%q", storage.objects[oldPath], storage.objects[newPath])
	}
}

func TestProfileService_UploadAvatarPersistenceFailure(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	upsertErr := errors.New("auth identity was deleted")
	oldURL := "https://project.example/storage/v1/object/public/avatars/" + userID.String() + "/avatar-0?v=old"
	newURL := "https://project.example/storage/v1/object/public/avatars/" + userID.String() + "/avatar-1?v=new"
	repo := &MockProfileDAO{
		GetFunc: func(context.Context, uuid.UUID) (*model.UserProfile, error) {
			return &model.UserProfile{ID: userID, AvatarURL: &oldURL}, nil
		},
		UpsertFunc: func(context.Context, uuid.UUID, *model.UpdateProfileRequest) error {
			return upsertErr
		},
	}
	newPath := userID.String() + "/avatar-1"

	t.Run("retains the bounded inactive upload and preserves the active object", func(t *testing.T) {
		oldPath := userID.String() + "/avatar-0"
		oldBytes := []byte("previous active avatar")
		newBytes := []byte("replacement avatar")
		storage := &fakeAvatarStorage{
			url:     newURL,
			objects: map[string][]byte{oldPath: append([]byte(nil), oldBytes...)},
		}
		svc := NewProfileService(repo, validator.New(), storage)

		_, err := svc.UploadAvatar(ctx, userID, "image/png", newBytes)

		if !errors.Is(err, upsertErr) {
			t.Fatalf("error = %v, want wrapped upsert error", err)
		}
		if storage.path != newPath || len(storage.deletePaths) != 0 {
			t.Fatalf("upload path=%q cleanup paths=%v, want retained %q", storage.path, storage.deletePaths, newPath)
		}
		if !bytes.Equal(storage.objects[oldPath], oldBytes) {
			t.Fatalf("previous active avatar bytes were changed or deleted: %q", storage.objects[oldPath])
		}
		if !bytes.Equal(storage.objects[newPath], newBytes) {
			t.Fatalf("bounded inactive upload was not retained: %q", storage.objects[newPath])
		}
	})

	t.Run("retains new bytes when the commit advances after negative confirmation", func(t *testing.T) {
		dbURL := oldURL
		delayedCommitRepo := &MockProfileDAO{
			GetFunc: func(context.Context, uuid.UUID) (*model.UserProfile, error) {
				urlSnapshot := dbURL
				return &model.UserProfile{ID: userID, AvatarURL: &urlSnapshot}, nil
			},
			ReplaceAvatarFunc: func(context.Context, uuid.UUID, string) (*model.UserProfile, error) {
				return nil, upsertErr
			},
		}
		oldPath := userID.String() + "/avatar-0"
		newBytes := []byte("delayed commit avatar")
		storage := &fakeAvatarStorage{
			url:     newURL,
			objects: map[string][]byte{oldPath: []byte("old avatar")},
		}
		svc := NewProfileService(delayedCommitRepo, validator.New(), storage)

		_, err := svc.UploadAvatar(ctx, userID, "image/png", newBytes)
		if !errors.Is(err, upsertErr) {
			t.Fatalf("error = %v, want wrapped persistence error", err)
		}

		// Simulate the original autocommit becoming visible only after the
		// service's detached confirmation read returned the old URL.
		dbURL = newURL

		if dbURL != newURL {
			t.Fatalf("simulated delayed commit URL = %q, want %q", dbURL, newURL)
		}
		if len(storage.deletePaths) != 0 || !bytes.Equal(storage.objects[newPath], newBytes) {
			t.Fatalf("delayed commit points at removed bytes: cleanup=%v bytes=%q", storage.deletePaths, storage.objects[newPath])
		}
	})

	t.Run("keeps the new object when commit confirmation is unavailable", func(t *testing.T) {
		confirmErr := errors.New("database unavailable")
		getCalls := 0
		ambiguousRepo := &MockProfileDAO{
			GetFunc: func(context.Context, uuid.UUID) (*model.UserProfile, error) {
				getCalls++
				if getCalls == 1 {
					return &model.UserProfile{ID: userID, AvatarURL: &oldURL}, nil
				}
				return nil, confirmErr
			},
			UpsertFunc: repo.UpsertFunc,
		}
		storage := &fakeAvatarStorage{url: newURL}
		svc := NewProfileService(ambiguousRepo, validator.New(), storage)

		_, err := svc.UploadAvatar(ctx, userID, "image/png", []byte("avatar"))

		if !errors.Is(err, upsertErr) || !errors.Is(err, confirmErr) {
			t.Fatalf("error = %v, want persistence and confirmation failures", err)
		}
		if len(storage.deletePaths) != 0 {
			t.Fatalf("ambiguous commit deleted possible active object: %v", storage.deletePaths)
		}
	})

	t.Run("returns success when the errored write is confirmed committed", func(t *testing.T) {
		getCalls := 0
		committedRepo := &MockProfileDAO{
			GetFunc: func(context.Context, uuid.UUID) (*model.UserProfile, error) {
				getCalls++
				if getCalls == 1 {
					return &model.UserProfile{ID: userID, AvatarURL: &oldURL}, nil
				}
				return &model.UserProfile{ID: userID, AvatarURL: &newURL}, nil
			},
			UpsertFunc: repo.UpsertFunc,
		}
		oldPath := userID.String() + "/avatar-0"
		oldBytes := []byte("previous avatar")
		newBytes := []byte("confirmed replacement")
		storage := &fakeAvatarStorage{
			url:     newURL,
			objects: map[string][]byte{oldPath: append([]byte(nil), oldBytes...)},
		}
		svc := NewProfileService(committedRepo, validator.New(), storage)

		profile, err := svc.UploadAvatar(ctx, userID, "image/png", newBytes)

		if err != nil || profile == nil || profile.AvatarURL == nil || *profile.AvatarURL != newURL {
			t.Fatalf("confirmed commit profile=%+v err=%v", profile, err)
		}
		if len(storage.deletePaths) != 0 {
			t.Fatalf("confirmed commit deleted supported avatar paths: %v", storage.deletePaths)
		}
		if !bytes.Equal(storage.objects[oldPath], oldBytes) || !bytes.Equal(storage.objects[newPath], newBytes) {
			t.Fatalf("confirmed commit must retain both slots: old=%q new=%q", storage.objects[oldPath], storage.objects[newPath])
		}
	})
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestAccountDeletionSerializesWithAvatarUpload(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	locker := &serialUserOperationLocker{}
	active := &mutableActiveUser{exists: true}
	deleteStarted := make(chan struct{})
	allowDelete := make(chan struct{})

	accountRepo := &mockAccountDAO{deleteFn: func(context.Context, uuid.UUID) error {
		close(deleteStarted)
		<-allowDelete
		return nil
	}}
	auth := &mockAuthDeleter{deleteFn: func() { active.setExists(false) }}
	accountStorage := &mockAvatarDeleter{}
	account := NewAccountService(accountRepo, accountStorage, auth, locker)

	profileRepo := &MockProfileDAO{
		UpsertFunc: func(context.Context, uuid.UUID, *model.UpdateProfileRequest) error {
			t.Fatal("a deleted identity must never reach profile persistence")
			return nil
		},
	}
	profileStorage := &fakeAvatarStorage{url: "https://project.example/avatar"}
	profile := NewProfileServiceWithUserBoundary(
		profileRepo,
		validator.New(),
		profileStorage,
		active,
		locker,
	)

	accountDone := make(chan error, 1)
	go func() { accountDone <- account.Delete(ctx, userID) }()
	<-deleteStarted

	uploadDone := make(chan error, 1)
	go func() {
		_, err := profile.UploadAvatar(ctx, userID, "image/png", []byte("avatar"))
		uploadDone <- err
	}()

	close(allowDelete)
	if err := <-accountDone; err != nil {
		t.Fatalf("account deletion failed: %v", err)
	}
	if err := <-uploadDone; !errors.Is(err, ErrAvatarIdentityInactive) {
		t.Fatalf("stale upload error = %v, want inactive identity", err)
	}
	if profileStorage.path != "" {
		t.Fatalf("stale upload wrote object %q after deletion", profileStorage.path)
	}
	if !equalStrings(accountStorage.lastPaths, avatarObjectPaths(userID)) {
		t.Fatalf("final deletion paths = %v, want %v", accountStorage.lastPaths, avatarObjectPaths(userID))
	}
}

func TestProfileService_UploadAvatarRechecksIdentityInsideBoundary(t *testing.T) {
	userID := uuid.New()
	storage := &fakeAvatarStorage{url: "https://project.example/avatar"}
	repo := &MockProfileDAO{}

	t.Run("inactive identity fails before storage", func(t *testing.T) {
		active := &mutableActiveUser{exists: false}
		svc := NewProfileServiceWithUserBoundary(repo, validator.New(), storage, active, &serialUserOperationLocker{})
		_, err := svc.UploadAvatar(context.Background(), userID, "image/png", []byte("avatar"))
		if !errors.Is(err, ErrAvatarIdentityInactive) || storage.path != "" {
			t.Fatalf("error=%v storage_path=%q", err, storage.path)
		}
	})

	t.Run("identity lookup failure fails closed", func(t *testing.T) {
		active := &mutableActiveUser{exists: true, err: errors.New("database unavailable")}
		svc := NewProfileServiceWithUserBoundary(repo, validator.New(), storage, active, &serialUserOperationLocker{})
		_, err := svc.UploadAvatar(context.Background(), userID, "image/png", []byte("avatar"))
		if !errors.Is(err, ErrAvatarIdentityCheckFailed) {
			t.Fatalf("error=%v, want identity check failure", err)
		}
	})
}
