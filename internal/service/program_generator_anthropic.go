package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// generationMaxTokens bounds the answer; a six-workout draft is well under it.
const generationMaxTokens = 6000

// anthropicDraftGenerator asks Claude for a draft constrained by a JSON schema
// whose catalog_id values are the catalog itself.
type anthropicDraftGenerator struct {
	client  anthropic.Client
	model   string
	timeout time.Duration
}

// NewAnthropicDraftGenerator builds the provider adapter. baseURL is empty in
// production; tests and staging point it at a fixture.
func NewAnthropicDraftGenerator(apiKey, baseURL, modelID string, timeout time.Duration) DraftGenerator {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithRequestTimeout(timeout),
		option.WithMaxRetries(1),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &anthropicDraftGenerator{client: anthropic.NewClient(opts...), model: modelID, timeout: timeout}
}

func (g *anthropicDraftGenerator) Generate(ctx context.Context, input GenerationInput) (*model.RawGeneratedDraft, GenerationUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	input.Model = g.model
	resp, err := g.client.Messages.New(ctx, buildGenerationRequest(input))
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			slog.WarnContext(ctx, "program generation provider error", "status", apiErr.StatusCode)
			return nil, GenerationUsage{}, fmt.Errorf("%w: provider status %d", ErrGeneratorUnavailable, apiErr.StatusCode)
		}
		return nil, GenerationUsage{}, fmt.Errorf("%w: %w", ErrGeneratorUnavailable, err)
	}
	usage := GenerationUsage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return nil, usage, ErrGeneratorRefused
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if textBlock, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(textBlock.Text)
		}
	}
	var raw model.RawGeneratedDraft
	if err := json.Unmarshal([]byte(strings.TrimSpace(text.String())), &raw); err != nil {
		return nil, usage, fmt.Errorf("%w: unparsable answer: %w", ErrGeneratorUnavailable, err)
	}
	return &raw, usage, nil
}

// buildGenerationRequest is pure so a test can prove what leaves the API: the
// profile fields, the catalog, the request text, and the schema.
func buildGenerationRequest(input GenerationInput) anthropic.MessageNewParams {
	catalogIDs := make([]string, 0, len(input.Catalog))
	var catalog strings.Builder
	for _, entry := range input.Catalog {
		catalogIDs = append(catalogIDs, entry.ID.String())
		mechanic := "-"
		if entry.Mechanic != nil {
			mechanic = *entry.Mechanic
		}
		fmt.Fprintf(&catalog, "%s | %s | %s | %s | %s\n", entry.ID, entry.Name, entry.Category, entry.Modality, mechanic)
	}
	system := systemRules + "\n\nExercise catalog (id | name | category | modality | mechanic):\n" + catalog.String()

	var user strings.Builder
	fmt.Fprintf(&user, "Primary goal: %s\n", input.PrimaryGoal)
	fmt.Fprintf(&user, "Training days per week: %d (ISO weekdays %v)\n", len(input.AvailableDays), input.AvailableDays)
	fmt.Fprintf(&user, "Maximum workouts in the draft: %d\n", input.WorkoutCap)
	fmt.Fprintf(&user, "Usual daily activity: %s\n", input.UsualActivity)
	fmt.Fprintf(&user, "Experience: %s\n", input.Experience)
	fmt.Fprintf(&user, "Equipment available: %s\n", strings.Join(input.Equipment, ", "))
	fmt.Fprintf(&user, "Session length: %d minutes\n", input.SessionDurationMinutes)
	if input.Request != nil {
		fmt.Fprintf(&user, "\nAthlete request: %s\n", *input.Request)
		user.WriteString("If the request asks for anything outside a program of catalog exercises (diet, medical advice, a sport not in the catalog), keep the catalog-only rule and say in one line of the rationale what could not be honored.\n")
	}
	user.WriteString("\nAnswer with the JSON draft only.")

	return anthropic.MessageNewParams{
		Model:     input.Model,
		MaxTokens: generationMaxTokens,
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user.String()))},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: model.GenerationSchema(catalogIDs, input.WorkoutCap)},
		},
	}
}

const systemRules = `You build a simple, sensible strength-and-conditioning program draft for one athlete from their training profile. The athlete reviews and edits it; it is a starting point, not a prescription.

Rules:
- Use only exercises from the catalog below, by catalog_id. Never invent an exercise.
- One workout per training day, at most the maximum given, each with 3 to 8 exercises and no exercise repeated within a workout. Leave at least one rest day; spread muscle groups across the week.
- Fit the equipment listed. bodyweight means no external load; bands means elastic bands only; dumbbells, barbell, machines, cardio_machine, and full_gym allow their equipment. Prefer compound movements first.
- Fit the session length: about 8 to 10 minutes per strength exercise including rest, less for isolation work. A 20-minute session holds 3 exercises; a 60-minute session holds 5 to 7.
- Strength exercises carry target_sets and target_reps (reps between 1 and 50) and null target_duration_seconds. Cardio exercises carry target_sets and target_duration_seconds (30 to 3600) and null target_reps. rest_seconds between 0 and 600. Never give a weight.
- Match the goal: strength favors 3 to 6 reps with longer rest; hypertrophy 6 to 12; conditioning and general_health moderate reps and cardio; power low reps with full rest; body_composition a mix with some cardio. Beginners get fewer exercises and simpler movements.
- Name the program (at most 80 characters) and each workout plainly. Write a rationale of at most 500 characters in plain language addressed to the athlete, with no medical claims.
- Answer only with JSON matching the schema.`
