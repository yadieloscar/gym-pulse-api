package router

import (
	"context"
	"encoding/json"
	"errors"
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
func (fakeLogDAO) Update(ctx context.Context, u uuid.UUID, d string, update model.DayLogUpdate) error {
	return nil
}
func (fakeLogDAO) Delete(ctx context.Context, u uuid.UUID, d string) error { return nil }
func (fakeLogDAO) ExerciseHistory(ctx context.Context, u uuid.UUID, ids []uuid.UUID) ([]model.ExerciseHistory, error) {
	return []model.ExerciseHistory{}, nil
}
func (fakeLogDAO) RecordSets(ctx context.Context, u uuid.UUID, ids []uuid.UUID) ([]model.SetPerf, error) {
	return []model.SetPerf{}, nil
}

type fakeStatsDAO struct{}

func (fakeStatsDAO) GetWeeklyCount(ctx context.Context, u uuid.UUID, s, e time.Time) (int, error) {
	return 0, nil
}
func (fakeStatsDAO) GetTotalWorkouts(ctx context.Context, u uuid.UUID) (int, error) { return 0, nil }
func (fakeStatsDAO) GetDistribution(ctx context.Context, u uuid.UUID) ([]model.TypeDistribution, error) {
	return nil, nil
}
func (fakeStatsDAO) GetDayStreak(ctx context.Context, u uuid.UUID) (int, error) { return 0, nil }
func (fakeStatsDAO) GetWeeklyVolume(ctx context.Context, u uuid.UUID, since time.Time) ([]model.WeeklyVolume, error) {
	return []model.WeeklyVolume{}, nil
}

type fakeSettingsDAO struct{}

func (fakeSettingsDAO) Get(ctx context.Context, u uuid.UUID) (*model.UserSettings, error) {
	return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 5}, nil
}
func (fakeSettingsDAO) Upsert(ctx context.Context, u uuid.UUID, s *model.UserSettings) error {
	return nil
}
func (fakeSettingsDAO) Patch(ctx context.Context, u uuid.UUID, r model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
	s := model.DefaultUserSettings()
	return &s, nil
}

type fakeProfileDAO struct{}

func (fakeProfileDAO) Get(ctx context.Context, u uuid.UUID) (*model.UserProfile, error) {
	return &model.UserProfile{ID: u}, nil
}
func (fakeProfileDAO) Upsert(ctx context.Context, u uuid.UUID, p *model.UpdateProfileRequest) error {
	return nil
}
func (fakeProfileDAO) ReplaceAvatar(ctx context.Context, u uuid.UUID, avatarURL string) (*model.UserProfile, error) {
	return &model.UserProfile{ID: u, AvatarURL: &avatarURL}, nil
}

type fakeBodyWeightDAO struct{}

func (fakeBodyWeightDAO) Upsert(ctx context.Context, u uuid.UUID, w *model.BodyWeight) (*model.BodyWeight, error) {
	return w, nil
}
func (fakeBodyWeightDAO) List(ctx context.Context, u uuid.UUID) ([]model.BodyWeight, error) {
	return []model.BodyWeight{}, nil
}
func (fakeBodyWeightDAO) Delete(ctx context.Context, u, e uuid.UUID) error { return nil }

type fakePlanDAO struct{}

func (fakePlanDAO) GetWeekly(ctx context.Context, u uuid.UUID) ([]model.WeeklyPlanDay, error) {
	return []model.WeeklyPlanDay{}, nil
}
func (fakePlanDAO) GetOverrides(ctx context.Context, u uuid.UUID, from, to time.Time) ([]model.PlanOverride, error) {
	return []model.PlanOverride{}, nil
}
func (fakePlanDAO) PutWeekly(ctx context.Context, u uuid.UUID, days []model.WeeklyPlanDay) error {
	return nil
}
func (fakePlanDAO) UpsertOverride(ctx context.Context, u uuid.UUID, date string, o model.PutPlanOverrideRequest) error {
	return nil
}
func (fakePlanDAO) DeleteOverride(ctx context.Context, u uuid.UUID, date string) error { return nil }

type fakeExerciseCatalogDAO struct{}

func (fakeExerciseCatalogDAO) List(ctx context.Context, category string) ([]model.CatalogExercise, error) {
	return []model.CatalogExercise{}, nil
}

type fakeAccountDAO struct{}

func (fakeAccountDAO) DeleteUserData(ctx context.Context, userID uuid.UUID) error {
	return nil
}

type fakeActiveUserChecker struct {
	exists bool
	err    error
	calls  int
}

func (c *fakeActiveUserChecker) Exists(ctx context.Context, userID uuid.UUID) (bool, error) {
	c.calls++
	return c.exists, c.err
}

// compile-time checks
var (
	_ dao.AccountDAO         = fakeAccountDAO{}
	_ dao.TemplateDAO        = fakeTemplateDAO{}
	_ dao.LogDAO             = fakeLogDAO{}
	_ dao.StatsDAO           = fakeStatsDAO{}
	_ dao.SettingsDAO        = fakeSettingsDAO{}
	_ dao.ProfileDAO         = fakeProfileDAO{}
	_ dao.BodyWeightDAO      = fakeBodyWeightDAO{}
	_ dao.ExerciseCatalogDAO = fakeExerciseCatalogDAO{}
	_ dao.PlanDAO            = fakePlanDAO{}
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
	exH := handler.NewExerciseCatalogHandler(newExerciseCatalogSvc(fakeExerciseCatalogDAO{}))
	planH := handler.NewPlanHandler(newPlanSvc(fakePlanDAO{}, fakeTemplateDAO{}, v))
	acctH := handler.NewAccountHandler(newAccountSvc(fakeAccountDAO{}))

	cfg := &config.Config{
		SupabaseJWTSecret: "test-secret",
		AllowedOrigins:    []string{"https://app.example.com"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	activeUsers := &fakeActiveUserChecker{exists: true}

	r := New(cfg, logger, activeUsers, tplH, logH, statsH, setH, profH, bwH, exH, planH, acctH, nil, nil, nil, nil, nil, nil, nil)
	do := func(req *http.Request) *http.Response {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Result()
	}

	mintToken := func(sub string) string {
		claims := jwt.MapClaims{"sub": sub, "exp": time.Now().Add(time.Hour).Unix()}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		s, _ := tok.SignedString([]byte(cfg.SupabaseJWTSecret))
		return s
	}

	userID := uuid.New().String()
	token := mintToken(userID)

	t.Run("health is public", func(t *testing.T) {
		resp := do(httptest.NewRequest(http.MethodGet, "/health", nil))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("protected without token -> 401", func(t *testing.T) {
		callsBefore := activeUsers.calls
		resp := do(httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
		if activeUsers.calls != callsBefore {
			t.Errorf("active-user check ran before JWT authentication")
		}
	})

	authed := func(method, path, body string) *http.Response {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return do(req)
	}

	t.Run("GET /api/v1/settings", func(t *testing.T) {
		resp := authed("GET", "/api/v1/settings", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("verified token for deleted auth user -> 401", func(t *testing.T) {
		activeUsers.exists = false
		defer func() { activeUsers.exists = true }()

		resp := authed(http.MethodGet, "/api/v1/settings", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
		var got map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["code"] != "AUTHENTICATION_REQUIRED" {
			t.Fatalf("code = %q, want AUTHENTICATION_REQUIRED", got["code"])
		}
	})

	t.Run("active-user lookup failure -> 503", func(t *testing.T) {
		activeUsers.err = errors.New("database unavailable")
		defer func() { activeUsers.err = nil }()

		resp := authed(http.MethodGet, "/api/v1/settings", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.StatusCode)
		}
		var got map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["code"] != "AUTHENTICATION_UNAVAILABLE" {
			t.Fatalf("code = %q, want AUTHENTICATION_UNAVAILABLE", got["code"])
		}
	})

	t.Run("DELETE /api/v1/account", func(t *testing.T) {
		resp := authed("DELETE", "/api/v1/account", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
		}
	})

	t.Run("deleted auth user can retry only account cleanup", func(t *testing.T) {
		activeUsers.exists = false
		defer func() { activeUsers.exists = true }()
		callsBefore := activeUsers.calls

		resp := authed(http.MethodDelete, "/api/v1/account", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected deletion retry 204, got %d", resp.StatusCode)
		}
		if activeUsers.calls != callsBefore {
			t.Fatal("account cleanup retry must not depend on the already-deleted auth row")
		}

		protected := authed(http.MethodGet, "/api/v1/settings", "")
		defer protected.Body.Close()
		if protected.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected every non-deletion route to remain 401, got %d", protected.StatusCode)
		}
	})

	t.Run("DELETE /api/v1/account without token -> 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account", nil)
		resp := do(req)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
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
		req := httptest.NewRequest("OPTIONS", "/api/v1/settings", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "Authorization")
		resp := do(req)
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
