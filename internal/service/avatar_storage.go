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
)

type AvatarStorage interface {
	Upload(ctx context.Context, objectPath, contentType string, body []byte) (string, error)
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
	publicPath := path.Join(s.bucket, objectPath)
	return s.baseURL + "/storage/v1/object/public/" + publicPath + "?v=" + strconv.FormatInt(s.now().UnixNano(), 10), nil
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
