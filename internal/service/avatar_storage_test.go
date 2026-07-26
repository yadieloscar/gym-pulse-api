package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSupabaseAvatarStorageUpload(t *testing.T) {
	storage := NewSupabaseAvatarStorage("https://project.example", "key", "avatars")
	storage.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPut || r.URL.Path != "/storage/v1/object/avatars/user/avatar" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-upsert") != "true" || r.Header.Get("apikey") != "key" || r.Header.Get("Content-Type") != "image/png" {
			t.Errorf("missing storage headers")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	storage.now = func() time.Time { return time.Unix(0, 7) }
	url, err := storage.Upload(context.Background(), "user/avatar", "image/png", []byte("png"))
	if err != nil || !strings.HasSuffix(url, "/storage/v1/object/public/avatars/user/avatar?v=7") {
		t.Fatalf("url=%q err=%v", url, err)
	}
}
