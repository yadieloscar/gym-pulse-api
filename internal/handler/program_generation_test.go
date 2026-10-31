package handler

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type fakeGenerationService struct {
	status *model.GenerationStatus
	resp   *model.GenerateProgramResponse
	err    error
	got    model.GenerateProgramRequest
}

func (f *fakeGenerationService) Status(context.Context, uuid.UUID) (*model.GenerationStatus, error) {
	return f.status, f.err
}

func (f *fakeGenerationService) Generate(_ context.Context, _ uuid.UUID, req model.GenerateProgramRequest) (*model.GenerateProgramResponse, error) {
	f.got = req
	return f.resp, f.err
}

func TestProgramGenerationHandlerStatus(t *testing.T) {
	resets := time.Date(2026, 10, 31, 4, 0, 0, 0, time.UTC)
	h := NewProgramGenerationHandler(&fakeGenerationService{status: &model.GenerationStatus{Enabled: true, Limit: 5, RemainingToday: 2, ResetsAt: &resets}}, 45*time.Second)
	rec := httptest.NewRecorder()
	h.Status(rec, newReq(t, http.MethodGet, "/api/v1/program-generation", nil, uuid.New()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"enabled":true`) || !strings.Contains(body, `"remaining_today":2`) || !strings.Contains(body, `"resets_at":"2026-10-31T04:00:00Z"`) {
		t.Fatalf("body: %s", body)
	}
}

func TestProgramGenerationHandlerGenerate(t *testing.T) {
	draft := model.ProgramDraft{Name: "Full body", PrimaryGoal: "strength", Roadmap: map[string]any{"origin": "generated", "model": "m", "request": nil}, Workouts: []model.DraftWorkout{}}
	svc := &fakeGenerationService{resp: &model.GenerateProgramResponse{Draft: draft, Rationale: "r", RemainingToday: 4, ResetsAt: time.Date(2026, 10, 31, 4, 0, 0, 0, time.UTC)}}
	h := NewProgramGenerationHandler(svc, 45*time.Second)

	for _, body := range []any{nil, "{}", map[string]any{"request": "no deadlifts"}} {
		rec := httptest.NewRecorder()
		req := newReq(t, http.MethodPost, "/api/v1/program-generation", body, uuid.New())
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		h.Generate(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("body %v: status %d %s", body, rec.Code, rec.Body.String())
		}
	}
	if svc.got.Request == nil || *svc.got.Request != "no deadlifts" {
		t.Fatalf("request text not passed through: %+v", svc.got)
	}
	var out map[string]any
	rec := httptest.NewRecorder()
	h.Generate(rec, newReq(t, http.MethodPost, "/api/v1/program-generation", nil, uuid.New()))
	decodeBody(t, rec, &out)
	for _, key := range []string{"draft", "rationale", "request", "remaining_today", "resets_at"} {
		if _, ok := out[key]; !ok {
			t.Errorf("response missing %s", key)
		}
	}
	if out["request"] != nil {
		t.Fatalf("request must be null when the service returned none: %v", out["request"])
	}

	rec = httptest.NewRecorder()
	h.Generate(rec, newReq(t, http.MethodPost, "/api/v1/program-generation", "{} {}", uuid.New()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON must be 400, got %d", rec.Code)
	}
}

func TestProgramGenerationHandlerErrorMapping(t *testing.T) {
	resets := time.Date(2026, 10, 31, 4, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		err    error
		status int
		want   []string
	}{
		{"no profile", &model.ValidationError{Message: "save a training profile before building a program", Field: "training_profile"}, 422, []string{`"code":"VALIDATION_ERROR"`, `"field":"training_profile"`}},
		{"limit", &model.GenerationLimitError{Limit: 5, ResetsAt: resets}, 429, []string{`"code":"GENERATION_LIMIT_REACHED"`, `"limit":5`, `"resets_at":"2026-10-31T04:00:00Z"`}},
		{"disabled", &model.GenerationUnavailableError{Reason: model.GenerationDisabled}, 503, []string{`"code":"GENERATION_UNAVAILABLE"`, `"reason":"disabled"`}},
		{"unavailable", &model.GenerationUnavailableError{Reason: model.GenerationUnavailable}, 503, []string{`"code":"GENERATION_UNAVAILABLE"`, `"reason":"unavailable"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewProgramGenerationHandler(&fakeGenerationService{err: tc.err}, 45*time.Second)
			rec := httptest.NewRecorder()
			h.Generate(rec, newReq(t, http.MethodPost, "/api/v1/program-generation", nil, uuid.New()))
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("body missing %s: %s", want, rec.Body.String())
				}
			}
		})
	}
}

// TestProgramGenerationHandlerExtendsTheWriteDeadline proves a slow generation
// survives a server whose WriteTimeout is shorter than the provider call.
func TestProgramGenerationHandlerExtendsTheWriteDeadline(t *testing.T) {
	slow := &slowGenerationService{delay: 400 * time.Millisecond}
	h := NewProgramGenerationHandler(slow, time.Second)
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.Generate(w, r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uuid.New().String())))
		}),
		WriteTimeout:      100 * time.Millisecond,
		ReadHeaderTimeout: time.Second,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	resp, err := http.Post("http://"+listener.Addr().String()+"/api/v1/program-generation", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("request failed under the short WriteTimeout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"rationale":"slow"`) {
		t.Fatalf("body truncated or wrong: %s", body)
	}
}

type slowGenerationService struct{ delay time.Duration }

func (s *slowGenerationService) Status(context.Context, uuid.UUID) (*model.GenerationStatus, error) {
	return &model.GenerationStatus{}, nil
}

func (s *slowGenerationService) Generate(ctx context.Context, _ uuid.UUID, _ model.GenerateProgramRequest) (*model.GenerateProgramResponse, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &model.GenerateProgramResponse{Draft: model.ProgramDraft{Roadmap: map[string]any{}, Workouts: []model.DraftWorkout{}}, Rationale: "slow"}, nil
}
