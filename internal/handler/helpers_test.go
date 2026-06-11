package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
)

// newReq builds an authenticated request with the user ID stamped into context.
func newReq(t *testing.T, method, target string, body any, userID uuid.UUID) *http.Request {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			rdr = strings.NewReader(v)
		default:
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			rdr = bytes.NewReader(b)
		}
	}
	req := httptest.NewRequest(method, target, rdr)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	return req.WithContext(ctx)
}

// withURLParam attaches a chi URL parameter to the request.
func withURLParam(req *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}
