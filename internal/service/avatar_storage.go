package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrAvatarStorageUnavailable = errors.New("avatar storage unavailable")
	ErrAvatarUploadFailed       = errors.New("avatar upload failed")
	ErrAvatarDeleteFailed       = errors.New("avatar delete failed")
	ErrAvatarSignFailed         = errors.New("avatar link signing failed")
)

type AvatarStorage interface {
	Upload(ctx context.Context, objectPath, contentType string, body []byte) (string, error)
}

// AvatarSigner turns a stored avatar reference into a short-lived link. The
// bucket is private, so the stored public-form URL is never handed to a client.
type AvatarSigner interface {
	// ObjectPath returns the bucket-relative path when storedURL points into
	// this deployment's avatar bucket, in either its stored or its signed form.
	ObjectPath(storedURL string) (string, bool)
	// CanonicalURL is the form the database stores for an object path.
	CanonicalURL(objectPath string) string
	Sign(ctx context.Context, objectPath string, ttl time.Duration) (string, error)
}

type AvatarDeleter interface {
	Delete(ctx context.Context, objectPaths ...string) error
}

const legacyAvatarObjectName = "avatar"

var avatarSlotObjectNames = [...]string{"avatar-0", "avatar-1"}

// avatarObjectPaths is the complete bounded object set supported for one user:
// the pre-versioning legacy path plus two alternating replacement slots.
func avatarObjectPaths(userID uuid.UUID) []string {
	return []string{
		path.Join(userID.String(), legacyAvatarObjectName),
		path.Join(userID.String(), avatarSlotObjectNames[0]),
		path.Join(userID.String(), avatarSlotObjectNames[1]),
	}
}

func nextAvatarObjectPath(userID uuid.UUID, currentURL *string) string {
	slotZero := path.Join(userID.String(), avatarSlotObjectNames[0])
	if currentURL != nil {
		if parsed, err := url.Parse(*currentURL); err == nil &&
			strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/"+slotZero) {
			return path.Join(userID.String(), avatarSlotObjectNames[1])
		}
	}
	return slotZero
}

type SupabaseAvatarStorage struct {
	baseURL, serviceRoleKey, bucket string
	client                          *http.Client
	now                             func() time.Time
}

func NewSupabaseAvatarStorage(baseURL, serviceRoleKey, bucket string) *SupabaseAvatarStorage {
	if bucket == "" {
		bucket = "avatars"
	}
	return &SupabaseAvatarStorage{baseURL: strings.TrimRight(baseURL, "/"), serviceRoleKey: serviceRoleKey, bucket: bucket, client: &http.Client{Timeout: 10 * time.Second}, now: time.Now}
}

func (s *SupabaseAvatarStorage) Upload(ctx context.Context, objectPath, contentType string, body []byte) (string, error) {
	endpoint := s.baseURL + "/storage/v1/object/" + url.PathEscape(s.bucket) + "/" + strings.TrimLeft(objectPath, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAvatarUploadFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceRoleKey)
	req.Header.Set("apikey", s.serviceRoleKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAvatarUploadFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("%w: storage status %d", ErrAvatarUploadFailed, resp.StatusCode)
	}
	return s.CanonicalURL(objectPath), nil
}

// CanonicalURL is the stable reference kept in user_profiles.avatar_url. It has
// the shape of a public object URL, which a private bucket does not serve; the
// version query tells one upload to a reused slot from the next.
func (s *SupabaseAvatarStorage) CanonicalURL(objectPath string) string {
	publicPath := path.Join(s.bucket, strings.TrimLeft(objectPath, "/"))
	return s.baseURL + "/storage/v1/object/public/" + publicPath + "?v=" + strconv.FormatInt(s.now().UnixNano(), 10)
}

func (s *SupabaseAvatarStorage) ObjectPath(storedURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(storedURL))
	if err != nil {
		return "", false
	}
	base, err := url.Parse(s.baseURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Host, base.Host) {
		return "", false
	}
	for _, access := range [...]string{"public", "sign", "authenticated"} {
		prefix := "/storage/v1/object/" + access + "/" + s.bucket + "/"
		if objectPath, found := strings.CutPrefix(parsed.Path, prefix); found && objectPath != "" {
			return objectPath, true
		}
	}
	return "", false
}

// Sign asks Supabase Storage for a link to one object that stops working after ttl.
func (s *SupabaseAvatarStorage) Sign(ctx context.Context, objectPath string, ttl time.Duration) (string, error) {
	seconds := int(ttl / time.Second)
	if seconds < 1 {
		return "", fmt.Errorf("%w: ttl must be at least one second", ErrAvatarSignFailed)
	}
	payload, err := json.Marshal(map[string]int{"expiresIn": seconds})
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAvatarSignFailed, err)
	}
	endpoint := s.baseURL + "/storage/v1/object/sign/" + url.PathEscape(s.bucket) + "/" + strings.TrimLeft(objectPath, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAvatarSignFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceRoleKey)
	req.Header.Set("apikey", s.serviceRoleKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAvatarSignFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("%w: storage status %d", ErrAvatarSignFailed, resp.StatusCode)
	}
	var signed struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&signed); err != nil {
		return "", fmt.Errorf("%w: decoding response: %w", ErrAvatarSignFailed, err)
	}
	// Storage answers with a path relative to its own root, token included.
	if !strings.HasPrefix(signed.SignedURL, "/object/sign/") {
		return "", fmt.Errorf("%w: unexpected response", ErrAvatarSignFailed)
	}
	return s.baseURL + "/storage/v1" + signed.SignedURL, nil
}

func (s *SupabaseAvatarStorage) Delete(ctx context.Context, objectPaths ...string) error {
	prefixes := make([]string, 0, len(objectPaths))
	seen := make(map[string]struct{}, len(objectPaths))
	for _, objectPath := range objectPaths {
		normalized := strings.TrimLeft(strings.TrimSpace(objectPath), "/")
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		prefixes = append(prefixes, normalized)
	}
	if len(prefixes) == 0 {
		return nil
	}

	requestBody, err := json.Marshal(struct {
		Prefixes []string `json:"prefixes"`
	}{
		Prefixes: prefixes,
	})
	if err != nil {
		return fmt.Errorf("%w: encoding request: %w", ErrAvatarDeleteFailed, err)
	}

	endpoint := s.baseURL + "/storage/v1/object/" + url.PathEscape(s.bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("%w: building request: %w", ErrAvatarDeleteFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceRoleKey)
	req.Header.Set("apikey", s.serviceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: calling storage api: %w", ErrAvatarDeleteFailed, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	// A missing object or bucket already satisfies the requested end state.
	if (resp.StatusCode < 200 || resp.StatusCode >= 300) && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("%w: storage status %d", ErrAvatarDeleteFailed, resp.StatusCode)
	}
	return nil
}
