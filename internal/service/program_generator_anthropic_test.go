package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func testGenerationInput() GenerationInput {
	compound := "compound"
	request := "no deadlifts"
	return GenerationInput{
		PrimaryGoal: "hypertrophy", AvailableDays: []int{1, 3, 5}, UsualActivity: "moderate", Experience: "intermediate",
		Equipment: []string{"barbell", "dumbbells"}, SessionDurationMinutes: 60, WorkoutCap: 3, Request: &request,
		Catalog: []model.CatalogExercise{
			{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Name: "Barbell Bench Press", Category: "push", Modality: "strength", Mechanic: &compound},
			{ID: uuid.MustParse("00000000-0000-0000-0000-000000000005"), Name: "Rowing", Category: "cardio", Modality: "cardio"},
		},
	}
}

func messagesResponse(text, stopReason string) string {
	body, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"content":       []map[string]any{{"type": "text", "text": text}},
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage":         map[string]any{"input_tokens": 1200, "output_tokens": 300},
	})
	return string(body)
}

const goodDraftJSON = `{"name":"Three-day split","rationale":"Push pull legs.","workouts":[{"name":"Push","exercises":[{"catalog_id":"00000000-0000-0000-0000-000000000001","target_sets":4,"target_reps":8,"target_duration_seconds":null,"rest_seconds":120,"notes":null}]}]}`

type recordedRequest struct {
	headers http.Header
	body    map[string]any
	raw     string
}

func newProviderFixture(t *testing.T, status int, respond func(w http.ResponseWriter)) (*httptest.Server, *recordedRequest) {
	t.Helper()
	recorded := &recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		recorded.headers = r.Header.Clone()
		recorded.raw = string(raw)
		_ = json.Unmarshal(raw, &recorded.body)
		w.Header().Set("Content-Type", "application/json")
		if respond != nil {
			respond(w)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server, recorded
}

func TestAnthropicGeneratorParsesDraftAndSendsOnlyAllowedFields(t *testing.T) {
	server, recorded := newProviderFixture(t, 200, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(messagesResponse(goodDraftJSON, "end_turn")))
	})
	generator := NewAnthropicDraftGenerator("sk-test-secret", server.URL, "claude-opus-5-5", 5*time.Second)
	raw, usage, err := generator.Generate(context.Background(), testGenerationInput())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if raw.Name != "Three-day split" || len(raw.Workouts) != 1 || raw.Workouts[0].Exercises[0].CatalogID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("draft not parsed: %+v", raw)
	}
	if usage.InputTokens != 1200 || usage.OutputTokens != 300 {
		t.Fatalf("usage not reported: %+v", usage)
	}
	if recorded.headers.Get("x-api-key") != "sk-test-secret" {
		t.Fatal("key must travel in the x-api-key header")
	}
	if strings.Contains(recorded.raw, "sk-test-secret") {
		t.Fatal("key must not appear in the body")
	}

	// Request shape.
	keys := map[string]bool{}
	for key := range recorded.body {
		keys[key] = true
	}
	for _, want := range []string{"model", "max_tokens", "system", "messages", "output_config"} {
		if !keys[want] {
			t.Errorf("request missing %s", want)
		}
		delete(keys, want)
	}
	if len(keys) != 0 {
		t.Errorf("unexpected top-level request keys: %v", keys)
	}
	maxTokens, _ := recorded.body["max_tokens"].(float64)
	if recorded.body["model"] != "claude-opus-5-5" || maxTokens != generationMaxTokens {
		t.Fatalf("model or max_tokens wrong: %v %v", recorded.body["model"], recorded.body["max_tokens"])
	}
	output, _ := recorded.body["output_config"].(map[string]any)
	if output["effort"] != "low" {
		t.Fatalf("effort must be low: %v", output["effort"])
	}
	format, _ := output["format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("format type: %v", format["type"])
	}
	if !strings.Contains(recorded.raw, `"enum":["00000000-0000-0000-0000-000000000001","00000000-0000-0000-0000-000000000005"]`) {
		t.Fatal("schema must enumerate the catalog ids")
	}
	systemBlocks, _ := recorded.body["system"].([]any)
	if len(systemBlocks) != 1 {
		t.Fatalf("want one system block, got %d", len(systemBlocks))
	}
	system, _ := systemBlocks[0].(map[string]any)
	if system["cache_control"] == nil {
		t.Fatal("system block must carry cache_control")
	}
	systemText, _ := system["text"].(string)
	if !strings.Contains(systemText, "00000000-0000-0000-0000-000000000001 | Barbell Bench Press | push | strength | compound") || !strings.Contains(systemText, "Rowing | cardio | cardio | -") {
		t.Fatal("catalog lines missing from the system block")
	}
	// Privacy: only profile fields, catalog, request, and schema leave the API.
	for _, forbidden := range []string{"user_id", "timezone", "America/", "@", "revision", "history", "body_weight"} {
		if strings.Contains(recorded.raw, forbidden) {
			t.Errorf("request body leaks %q", forbidden)
		}
	}
	for _, want := range []string{"Primary goal: hypertrophy", "ISO weekdays [1 3 5]", "Maximum workouts in the draft: 3", "Equipment available: barbell, dumbbells", "Session length: 60 minutes", "Athlete request: no deadlifts", "could not be honored"} {
		if !strings.Contains(recorded.raw, want) {
			t.Errorf("request body missing %q", want)
		}
	}
	if strings.Count(recorded.raw, "no deadlifts") != 1 {
		t.Fatal("the request text must appear exactly once")
	}
}

func TestAnthropicGeneratorMapsFailures(t *testing.T) {
	cases := []struct {
		name    string
		respond func(w http.ResponseWriter)
		want    error
	}{
		{"refusal", func(w http.ResponseWriter) { _, _ = w.Write([]byte(messagesResponse("", "refusal"))) }, ErrGeneratorRefused},
		{"bad json", func(w http.ResponseWriter) { _, _ = w.Write([]byte(messagesResponse("not json", "end_turn"))) }, ErrGeneratorUnavailable},
		{"429", func(w http.ResponseWriter) {
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
		}, ErrGeneratorUnavailable},
		{"529", func(w http.ResponseWriter) {
			w.WriteHeader(529)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`))
		}, ErrGeneratorUnavailable},
		{"500", func(w http.ResponseWriter) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`))
		}, ErrGeneratorUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newProviderFixture(t, 200, tc.respond)
			generator := NewAnthropicDraftGenerator("k", server.URL, "m", 5*time.Second)
			_, _, err := generator.Generate(context.Background(), testGenerationInput())
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "k\"") {
				t.Fatal("error must not carry the key")
			}
		})
	}
}

func TestAnthropicGeneratorHonoursTheDeadline(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })
	generator := NewAnthropicDraftGenerator("k", server.URL, "m", 150*time.Millisecond)
	started := time.Now()
	_, _, err := generator.Generate(context.Background(), testGenerationInput())
	if !errors.Is(err, ErrGeneratorUnavailable) {
		t.Fatalf("timeout must be unavailable, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("deadline not honoured: %v", elapsed)
	}
}
