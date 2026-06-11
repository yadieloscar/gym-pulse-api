package router

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/config"
	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/handler"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// --- minimal in-memory DAO stubs satisfying the dao interfaces ---

type fakeTemplateDAO struct{}

func (fakeTemplateDAO) List(ctx context.Context, u uuid.UUID, tf, sf string) ([]model.TemplateSummary, error) {
	return []model.TemplateSummary{}, nil
}
func (fakeTemplateDAO) GetByID(ctx context.Context, u, id uuid.UUID) (*model.WorkoutTemplate, error) {
	return &model.WorkoutTemplate{ID: id}, nil
}
func (fakeTemplateDAO) Create(ctx context.Context, u uuid.UUID, t *model.WorkoutTemplate) error {
	return nil
}
func (fakeTemplateDAO) Update(ctx context.Context, u uuid.UUID, t *model.WorkoutTemplate) error {
	return nil
}
func (fakeTemplateDAO) Delete(ctx context.Context, u, id uuid.UUID) error { return nil }

type fakeLogDAO struct{}

func (fakeLogDAO) ListByWeek(ctx context.Context, u uuid.UUID, monday time.Time) ([]model.DayLogSummary, error) {
	return []model.DayLogSummary{}, nil
}
func (fakeLogDAO) GetByDate(ctx context.Context, u uuid.UUID, d string) (*model.DayLog, error) {
	return &model.DayLog{Date: d}, nil
}
func (fakeLogDAO) Create(ctx context.Context, u uuid.UUID, l *model.DayLog) error { return nil }
func (fakeLogDAO) Update(ctx context.Context, u uuid.UUID, d string, o []model.ExerciseOverride, n *string) error {
	return nil
}
func (fakeLogDAO) Delete(ctx context.Context, u uuid.UUID, d string) error { return nil }

type fakeStatsDAO struct{}

func (fakeStatsDAO) GetWeeklyCount(ctx context.Context, u uuid.UUID, s, e time.Time) (int, error) {
	return 0, nil
}
func (fakeStatsDAO) GetTotalWorkouts(ctx context.Context, u uuid.UUID) (int, error) { return 0, nil }
func (fakeStatsDAO) GetDistribution(ctx context.Context, u uuid.UUID) ([]model.TypeDistribution, error) {
	return nil, nil
}
func (fakeStatsDAO) GetDayStreak(ctx context.Context, u uuid.UUID) (int, error) { return 0, nil }

type fakeSettingsDAO struct{}

func (fakeSettingsDAO) Get(ctx context.Context, u uuid.UUID) (*model.UserSettings, error) {
	return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 5}, nil
}
func (fakeSettingsDAO) Upsert(ctx context.Context, u uuid.UUID, s *model.UserSettings) error {
	return nil
}

type fakeProfileDAO struct{}

func (fakeProfileDAO) Get(ctx context.Context, u uuid.UUID) (*model.UserProfile, error) {
	return &model.UserProfile{ID: u}, nil
}
func (fakeProfileDAO) Upsert(ctx context.Context, u uuid.UUID, p *model.UpdateProfileRequest) error {
	return nil
}

type fakeBodyWeightDAO struct{}

func (fakeBodyWeightDAO) Upsert(ctx context.Context, u uuid.UUID, w *model.BodyWeight) (*model.BodyWeight, error) {
	return w, nil
}
func (fakeBodyWeightDAO) List(ctx context.Context, u uuid.UUID) ([]model.BodyWeight, error) {
	return []model.BodyWeight{}, nil
}
func (fakeBodyWeightDAO) Delete(ctx context.Context, u, e uuid.UUID) error { return nil }

// compile-time checks
var (
	_ dao.TemplateDAO   = fakeTemplateDAO{}
	_ dao.LogDAO        = fakeLogDAO{}
	_ dao.StatsDAO      = fakeStatsDAO{}
	_ dao.SettingsDAO   = fakeSettingsDAO{}
	_ dao.ProfileDAO    = fakeProfileDAO{}
	_ dao.BodyWeightDAO = fakeBodyWeightDAO{}
)

func TestRouter_RoutesAndAuth(t *testing.T) {
	// Build the full router with real handlers + service wiring on fake DAOs.
	v := validator.New()

	// service package types are needed; the constructors live there.
	tplH := handler.NewTemplateHandler(newTemplateSvc(fakeTemplateDAO{}, v))
	logH := handler.NewLogHandler(newLogSvc(fakeLogDAO{}, fakeTemplateDAO{}, v))
	statsH := handler.NewStatsHandler(newStatsSvc(fakeStatsDAO{}, fakeSettingsDAO{}))
	setH := handler.NewSettingsHandler(newSettingsSvc(fakeSettingsDAO{}, v))
	profH := handler.NewProfileHandler(newProfileSvc(fakeProfileDAO{}, v))
	bwH := handler.NewBodyWeightHandler(newBodyWeightSvc(fakeBodyWeightDAO{}, v))

	cfg := &config.Config{
		SupabaseJWTSecret: "test-secret",
		AllowedOrigins:    []string{"https://app.example.com"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	r := New(cfg, logger, tplH, logH, statsH, setH, profH, bwH)
	srv := httptest.NewServer(r)
	defer srv.Close()

	mintToken := func(sub string) string {
		claims := jwt.MapClaims{"sub": sub, "exp": time.Now().Add(time.Hour).Unix()}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		s, _ := tok.SignedString([]byte(cfg.SupabaseJWTSecret))
		return s
	}

	userID := uuid.New().String()
	token := mintToken(userID)

	t.Run("health is public", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/health")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("protected without token -> 401", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/settings")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	authed := func(method, path, body string) *http.Response {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	t.Run("GET /api/v1/settings", func(t *testing.T) {
		resp := authed("GET", "/api/v1/settings", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/stats (alias for summary)", func(t *testing.T) {
		resp := authed("GET", "/api/v1/stats", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/stats/summary", func(t *testing.T) {
		resp := authed("GET", "/api/v1/stats/summary", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/stats/distribution returns {types:...}", func(t *testing.T) {
		resp := authed("GET", "/api/v1/stats/distribution", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var got map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if _, ok := got["types"]; !ok {
			t.Errorf("expected 'types' wrapper, got %+v", got)
		}
	})

	t.Run("GET /api/v1/templates", func(t *testing.T) {
		resp := authed("GET", "/api/v1/templates", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/logs requires week param -> 400", func(t *testing.T) {
		resp := authed("GET", "/api/v1/logs", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/profile", func(t *testing.T) {
		resp := authed("GET", "/api/v1/profile", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/body/weight", func(t *testing.T) {
		resp := authed("GET", "/api/v1/body/weight", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("CORS preflight handled", func(t *testing.T) {
		req, _ := http.NewRequest("OPTIONS", srv.URL+"/api/v1/settings", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "Authorization")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		// chi/cors returns 204 for preflight
		if resp.StatusCode >= 400 {
			t.Errorf("preflight failed: %d", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got == "" {
			t.Errorf("missing CORS allow-origin header")
		}
	})
}
