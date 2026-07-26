package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

var (
	ErrAvatarStorageUnavailable = errors.New("avatar storage unavailable")
	ErrAvatarUploadFailed       = errors.New("avatar upload failed")
)

type AvatarStorage interface {
	Upload(ctx context.Context, objectPath, contentType string, body []byte) (string, error)
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
