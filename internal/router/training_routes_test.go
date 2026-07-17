package router

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/config"
)

func TestTrainingRoutesRegistered(t *testing.T) {
	r := New(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	routes, ok := r.(chi.Routes)
	if !ok {
		t.Fatal("router does not expose chi routes")
	}
	found := map[string]bool{}
	if err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		found[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{
		"GET /api/v1/training-profile", "PUT /api/v1/training-profile",
		"GET /api/v1/starter-programs", "POST /api/v1/programs/from-starter",
		"POST /api/v1/schedule/materialize", "PUT /api/v1/scheduled-workouts/{id}/sets/{set_id}",
		"POST /api/v1/scheduled-workouts/{id}/complete", "GET /api/v1/participation",
	} {
		if !found[route] {
			t.Errorf("missing route %s", route)
		}
	}
}
