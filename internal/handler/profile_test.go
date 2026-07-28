package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

func TestProfileHandler_Get(t *testing.T) {
	uid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockProfileService{
			GetFunc: func(ctx context.Context, u uuid.UUID) (*model.UserProfile, error) {
				return &model.UserProfile{ID: u}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewProfileHandler(svc).Get(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("error -> 500", func(t *testing.T) {
		svc := &MockProfileService{
			GetFunc: func(ctx context.Context, u uuid.UUID) (*model.UserProfile, error) {
				return nil, errors.New("db")
			},
		}
		rec := httptest.NewRecorder()
		NewProfileHandler(svc).Get(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}

func TestProfileHandler_UploadAvatar(t *testing.T) {
	uid := uuid.New()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "avatar.png")
	_, _ = part.Write(append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 512)...))
	_ = writer.Close()
	svc := &MockProfileService{UploadAvatarFunc: func(_ context.Context, id uuid.UUID, contentType string, _ []byte) (*model.UserProfile, error) {
		if id != uid || contentType != "image/png" {
			t.Fatalf("unexpected upload identity/type")
		}
		return &model.UserProfile{ID: id}, nil
	}}
	req := newReq(t, http.MethodPut, "/", nil, uid)
	req.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	NewProfileHandler(svc).UploadAvatar(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProfileHandler_UploadAvatarFailures(t *testing.T) {
	uid := uuid.New()
	validPNG := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 512)...)

	multipartRequest := func(t *testing.T, field, filename string, data []byte) *http.Request {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile(field, filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := newReq(t, http.MethodPut, "/", nil, uid)
		req.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
		req.Header.Set("Content-Type", writer.FormDataContentType())
		return req
	}

	tests := []struct {
		name   string
		req    func(*testing.T) *http.Request
		svcErr error
		status int
	}{
		{
			name: "missing multipart content type",
			req: func(t *testing.T) *http.Request {
				return newReq(t, http.MethodPut, "/", nil, uid)
			},
			status: http.StatusBadRequest,
		},
		{
			name: "wrong file field",
			req: func(t *testing.T) *http.Request {
				return multipartRequest(t, "avatar", "avatar.png", validPNG)
			},
			status: http.StatusBadRequest,
		},
		{
			name: "unsupported media type",
			req: func(t *testing.T) *http.Request {
				return multipartRequest(t, "file", "avatar.txt", []byte("plain text"))
			},
			status: http.StatusUnsupportedMediaType,
		},
		{
			name: "storage unavailable",
			req: func(t *testing.T) *http.Request {
				return multipartRequest(t, "file", "avatar.png", validPNG)
			},
			svcErr: service.ErrAvatarStorageUnavailable,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "storage upload failed",
			req: func(t *testing.T) *http.Request {
				return multipartRequest(t, "file", "avatar.png", validPNG)
			},
			svcErr: service.ErrAvatarUploadFailed,
			status: http.StatusBadGateway,
		},
		{
			name: "identity became inactive",
			req: func(t *testing.T) *http.Request {
				return multipartRequest(t, "file", "avatar.png", validPNG)
			},
			svcErr: service.ErrAvatarIdentityInactive,
			status: http.StatusUnauthorized,
		},
		{
			name: "identity recheck unavailable",
			req: func(t *testing.T) *http.Request {
				return multipartRequest(t, "file", "avatar.png", validPNG)
			},
			svcErr: service.ErrAvatarIdentityCheckFailed,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "unexpected service failure",
			req: func(t *testing.T) *http.Request {
				return multipartRequest(t, "file", "avatar.png", validPNG)
			},
			svcErr: errors.New("database unavailable"),
			status: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &MockProfileService{UploadAvatarFunc: func(context.Context, uuid.UUID, string, []byte) (*model.UserProfile, error) {
				return nil, tt.svcErr
			}}
			rec := httptest.NewRecorder()
			NewProfileHandler(svc).UploadAvatar(rec, tt.req(t))
			if rec.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
}

func TestWriteMultipartError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "too large", err: &http.MaxBytesError{Limit: maxAvatarBytes}, status: http.StatusRequestEntityTooLarge},
		{name: "malformed", err: errors.New("malformed multipart body"), status: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeMultipartError(rec, tt.err)
			if rec.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
}

func TestProfileHandler_Update(t *testing.T) {
	uid := uuid.New()
	name := "Jane"
	body := model.UpdateProfileRequest{DisplayName: &name}

	t.Run("success", func(t *testing.T) {
		svc := &MockProfileService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UpdateProfileRequest) (*model.UserProfile, error) {
				return &model.UserProfile{ID: u, DisplayName: r.DisplayName}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewProfileHandler(svc).Update(rec, newReq(t, "PUT", "/", body, uid))
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("bad json -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewProfileHandler(&MockProfileService{}).Update(rec, newReq(t, "PUT", "/", "junk", uid))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("validation -> 422", func(t *testing.T) {
		svc := &MockProfileService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UpdateProfileRequest) (*model.UserProfile, error) {
				return nil, &model.ValidationError{Message: "bad", Field: "body"}
			},
		}
		rec := httptest.NewRecorder()
		NewProfileHandler(svc).Update(rec, newReq(t, "PUT", "/", body, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422, got %d", rec.Code)
		}
	})
}
