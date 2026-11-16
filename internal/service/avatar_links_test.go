package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestSupabaseAvatarStorageSign(t *testing.T) {
	storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
	storage.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/storage/v1/object/sign/avatars/user/avatar-0" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("apikey") != "key" {
			t.Errorf("missing storage credentials")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"expiresIn":3600}` {
			t.Errorf("body = %s, want a one-hour expiry", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"signedURL":"/object/sign/avatars/user/avatar-0?token=abc"}`))}, nil
	})

	link, err := storage.Sign(context.Background(), "user/avatar-0", time.Hour)
	if err != nil || link != "https://project.example/storage/v1/object/sign/avatars/user/avatar-0?token=abc" {
		t.Fatalf("link=%q err=%v", link, err)
	}
}

func TestSupabaseAvatarStorageSignFailures(t *testing.T) {
	respond := func(status int, body string) *SupabaseAvatarStorage {
		storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
		storage.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		return storage
	}
	tests := []struct {
		name    string
		storage *SupabaseAvatarStorage
		ttl     time.Duration
	}{
		{name: "storage refuses", storage: respond(http.StatusBadRequest, `{"error":"Object not found"}`), ttl: time.Hour},
		{name: "response is not JSON", storage: respond(http.StatusOK, `nope`), ttl: time.Hour},
		{name: "response points outside the signed route", storage: respond(http.StatusOK, `{"signedURL":"https://elsewhere.example/x"}`), ttl: time.Hour},
		{name: "ttl below one second", storage: respond(http.StatusOK, `{}`), ttl: time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.storage.Sign(context.Background(), "user/avatar-0", tt.ttl); !errors.Is(err, ErrAvatarSignFailed) {
				t.Fatalf("err = %v, want ErrAvatarSignFailed", err)
			}
		})
	}
}

func TestSupabaseAvatarStorageObjectPath(t *testing.T) {
	storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
	tests := []struct {
		name, stored, want string
		ours               bool
	}{
		{name: "stored public form", stored: "https://project.example/storage/v1/object/public/avatars/u/avatar-1?v=9", want: "u/avatar-1", ours: true},
		{name: "signed form a client sends back", stored: "https://project.example/storage/v1/object/sign/avatars/u/avatar-0?token=t", want: "u/avatar-0", ours: true},
		{name: "another host", stored: "https://images.example/storage/v1/object/public/avatars/u/avatar-0"},
		{name: "another bucket", stored: "https://project.example/storage/v1/object/public/other/u/avatar-0"},
		{name: "bucket with no object", stored: "https://project.example/storage/v1/object/public/avatars/"},
		{name: "external picture", stored: "https://images.example/me.png"},
		{name: "not a URL", stored: "://"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ours := storage.ObjectPath(tt.stored)
			if got != tt.want || ours != tt.ours {
				t.Fatalf("ObjectPath = (%q, %v), want (%q, %v)", got, ours, tt.want, tt.ours)
			}
		})
	}
}

// signingAvatarStorage is a storage double that can also sign.
type signingAvatarStorage struct {
	fakeAvatarStorage
	signErr   error
	signCalls int
	signed    []string
}

const signingBase = "https://project.example/storage/v1/object/"

func (s *signingAvatarStorage) ObjectPath(stored string) (string, bool) {
	for _, access := range []string{"public", "sign"} {
		if rest, found := strings.CutPrefix(stored, signingBase+access+"/avatars/"); found {
			objectPath, _, _ := strings.Cut(rest, "?")
			return objectPath, objectPath != ""
		}
	}
	return "", false
}

func (s *signingAvatarStorage) CanonicalURL(objectPath string) string {
	return signingBase + "public/avatars/" + objectPath + "?v=canonical"
}

func (s *signingAvatarStorage) Sign(_ context.Context, objectPath string, ttl time.Duration) (string, error) {
	s.signCalls++
	if ttl != avatarLinkTTL {
		return "", errors.New("unexpected ttl")
	}
	if s.signErr != nil {
		return "", s.signErr
	}
	s.signed = append(s.signed, objectPath)
	return signingBase + "sign/avatars/" + objectPath + "?token=" + string(rune('a'+s.signCalls-1)), nil
}

func TestProfileServiceReturnsSignedAvatarLinks(t *testing.T) {
	ctx := context.Background()
	userID, otherID := uuid.New(), uuid.New()
	stored := func(owner uuid.UUID, slot string) *string {
		value := signingBase + "public/avatars/" + owner.String() + "/" + slot + "?v=1"
		return &value
	}
	external := "https://images.example/me.png"

	newService := func(avatar *string, storage *signingAvatarStorage) (ProfileService, *model.UserProfile) {
		current := &model.UserProfile{ID: userID, AvatarURL: avatar}
		repo := &MockProfileDAO{
			GetFunc: func(context.Context, uuid.UUID) (*model.UserProfile, error) {
				copied := *current
				return &copied, nil
			},
			UpsertFunc: func(_ context.Context, _ uuid.UUID, req *model.UpdateProfileRequest) error {
				if req.AvatarURL != nil {
					current.AvatarURL = req.AvatarURL
				}
				return nil
			},
		}
		return NewProfileService(repo, validator.New(), storage), current
	}

	t.Run("a stored reference becomes a signed link and the database value is untouched", func(t *testing.T) {
		storage := &signingAvatarStorage{}
		svc, current := newService(stored(userID, "avatar-0"), storage)
		profile, err := svc.Get(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		want := signingBase + "sign/avatars/" + userID.String() + "/avatar-0?token=a"
		if profile.AvatarURL == nil || *profile.AvatarURL != want {
			t.Fatalf("avatar_url = %v, want %s", profile.AvatarURL, want)
		}
		if !strings.Contains(*current.AvatarURL, "/public/") {
			t.Fatalf("stored reference changed to %s", *current.AvatarURL)
		}
	})

	t.Run("the same link is reused until its reuse window ends, then signed again", func(t *testing.T) {
		storage := &signingAvatarStorage{}
		svc, _ := newService(stored(userID, "avatar-0"), storage)
		clock := time.Unix(1_800_000_000, 0)
		concrete, ok := svc.(*profileService)
		if !ok {
			t.Fatal("NewProfileService no longer returns *profileService")
		}
		concrete.links.now = func() time.Time { return clock }

		first, _ := svc.Get(ctx, userID)
		clock = clock.Add(avatarLinkReuse - time.Second)
		second, _ := svc.Get(ctx, userID)
		if storage.signCalls != 1 || *first.AvatarURL != *second.AvatarURL {
			t.Fatalf("signCalls = %d, links %s and %s; want one link reused", storage.signCalls, *first.AvatarURL, *second.AvatarURL)
		}
		clock = clock.Add(2 * time.Second)
		third, _ := svc.Get(ctx, userID)
		if storage.signCalls != 2 || *third.AvatarURL == *first.AvatarURL {
			t.Fatalf("signCalls = %d; want a fresh link after the reuse window", storage.signCalls)
		}
	})

	t.Run("a profile still loads without a photo when signing fails", func(t *testing.T) {
		storage := &signingAvatarStorage{signErr: ErrAvatarSignFailed}
		svc, _ := newService(stored(userID, "avatar-0"), storage)
		profile, err := svc.Get(ctx, userID)
		if err != nil || profile == nil || profile.AvatarURL != nil {
			t.Fatalf("profile = %+v err = %v, want a profile with no avatar_url", profile, err)
		}
	})

	t.Run("another athlete's object is never signed", func(t *testing.T) {
		storage := &signingAvatarStorage{}
		svc, _ := newService(stored(otherID, "avatar-0"), storage)
		profile, err := svc.Get(ctx, userID)
		if err != nil || profile.AvatarURL != nil || storage.signCalls != 0 {
			t.Fatalf("avatar_url = %v signCalls = %d err = %v, want none", profile.AvatarURL, storage.signCalls, err)
		}
	})

	t.Run("an external https picture is returned as set", func(t *testing.T) {
		storage := &signingAvatarStorage{}
		svc, _ := newService(&external, storage)
		profile, _ := svc.Get(ctx, userID)
		if profile.AvatarURL == nil || *profile.AvatarURL != external || storage.signCalls != 0 {
			t.Fatalf("avatar_url = %v signCalls = %d, want the external URL unsigned", profile.AvatarURL, storage.signCalls)
		}
	})

	t.Run("a signed link sent back is stored as the stable reference", func(t *testing.T) {
		storage := &signingAvatarStorage{}
		svc, current := newService(stored(userID, "avatar-0"), storage)
		sentBack := signingBase + "sign/avatars/" + userID.String() + "/avatar-1?token=old"
		profile, err := svc.Update(ctx, userID, model.UpdateProfileRequest{AvatarURL: &sentBack})
		if err != nil {
			t.Fatal(err)
		}
		if want := signingBase + "public/avatars/" + userID.String() + "/avatar-1?v=canonical"; *current.AvatarURL != want {
			t.Fatalf("stored = %s, want %s", *current.AvatarURL, want)
		}
		if profile.AvatarURL == nil || !strings.Contains(*profile.AvatarURL, "/sign/avatars/"+userID.String()+"/avatar-1?token=") {
			t.Fatalf("avatar_url = %v, want a signed link to avatar-1", profile.AvatarURL)
		}
	})

	t.Run("pointing avatar_url at another athlete's object is refused", func(t *testing.T) {
		storage := &signingAvatarStorage{}
		svc, current := newService(nil, storage)
		theirs := signingBase + "sign/avatars/" + otherID.String() + "/avatar-0?token=x"
		_, err := svc.Update(ctx, userID, model.UpdateProfileRequest{AvatarURL: &theirs})
		var validation *model.ValidationError
		if !errors.As(err, &validation) || validation.Field != "avatar_url" || current.AvatarURL != nil {
			t.Fatalf("err = %v stored = %v, want a validation error on avatar_url and nothing stored", err, current.AvatarURL)
		}
	})

	t.Run("an upload returns a fresh signed link for the new slot", func(t *testing.T) {
		storage := &signingAvatarStorage{}
		storage.url = *stored(userID, "avatar-0")
		current := &model.UserProfile{ID: userID}
		repo := &MockProfileDAO{
			GetFunc: func(context.Context, uuid.UUID) (*model.UserProfile, error) { copied := *current; return &copied, nil },
			ReplaceAvatarFunc: func(_ context.Context, _ uuid.UUID, avatarURL string) (*model.UserProfile, error) {
				current.AvatarURL = &avatarURL
				copied := *current
				return &copied, nil
			},
		}
		svc := NewProfileService(repo, validator.New(), storage)
		profile, err := svc.UploadAvatar(ctx, userID, "image/png", []byte("png"))
		if err != nil {
			t.Fatal(err)
		}
		if profile.AvatarURL == nil || !strings.Contains(*profile.AvatarURL, "/sign/avatars/"+userID.String()+"/avatar-0?token=") {
			t.Fatalf("avatar_url = %v, want a signed link", profile.AvatarURL)
		}
		if !strings.Contains(*current.AvatarURL, "/public/avatars/") {
			t.Fatalf("stored = %s, want the stable reference", *current.AvatarURL)
		}
	})
}
