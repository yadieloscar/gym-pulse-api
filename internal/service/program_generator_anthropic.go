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
	system := systemRules + "\n\n" + goalGuide + "\n\nExercise catalog (id | name | category | modality | mechanic):\n" + catalog.String()

	var user strings.Builder
	fmt.Fprintf(&user, "Primary goal: %s\n", input.PrimaryGoal)
	if len(input.SecondaryGoals) > 0 {
		fmt.Fprintf(&user, "Secondary goals: %s\n", strings.Join(input.SecondaryGoals, ", "))
	}
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
- Follow the goal guide below for the primary goal: it sets the split, exercise order, rep ranges, sets, and rest. Beginners get fewer exercises and simpler movements whatever the goal.
- Secondary goals, when given, take at most about one third of the exercises or one of the days and are placed so they do not blunt the primary goal (see the guide).
- Name the program (at most 80 characters) and each workout plainly. Write a rationale of at most 500 characters in plain language addressed to the athlete, with no medical claims.
- Answer only with JSON matching the schema.`

// goalGuide is the evidence-based programming default per goal. Sources are
// recorded in specs/015-ai-program-generation/research.md ("Goal guide").
const goalGuide = `Goal guide (defaults from the resistance-training and physical-activity position stands; experience, equipment, and session length still override):
- strength: multi-joint lifts first (squat, hinge, press, pull). Main lifts 3 to 6 reps for intermediate and advanced athletes, 6 to 8 for beginners, 3 to 5 sets, rest 120 to 300 s. Train each main pattern about twice a week across 2 to 4 days. Accessory work 6 to 10 reps with 60 to 120 s rest. Little or no cardio.
- hypertrophy: 6 to 12 reps as the main range, 12 to 20 for isolation work, 3 to 4 sets per exercise, aiming at 10 or more weekly sets per major muscle group. Rest 90 to 180 s on compounds, 60 to 90 s on isolation. Train each muscle group twice a week when the days allow (full body on 2 to 3 days, upper/lower on 4, push/pull/legs on 5 or 6). Compounds before isolation.
- power: needs a strength base. Beginners get a strength-style program with one or two jump or throw movements per session. Otherwise explosive movements first while fresh (jumps, throws, kettlebell swings, Olympic-lift variants when in the catalog): 2 to 5 sets of 1 to 5 fast reps, rest 120 to 300 s, then heavy strength work of 2 to 5 reps. Low total volume. No cardio inside the session.
- conditioning: 2 to 3 cardio-led days using cardio catalog entries for 10 to 30 minutes, intervals on at most two of them (work bouts of 1 to 4 minutes with equal or shorter rest), plus 1 to 2 full-body resistance days as circuits of compound movements at 10 to 15 reps with 30 to 60 s rest.
- general_health: 2 to 3 full-body resistance days of 6 to 10 exercises covering every major muscle group, 1 to 3 sets of 8 to 12 reps, rest 60 to 120 s, with 15 to 30 minutes of moderate cardio at the end of a session or on the other days.
- body_composition: resistance training leads (up to 3 days) with compound movements at 8 to 15 reps, 3 sets, rest 60 to 90 s, because it keeps lean mass. Add 10 to 20 minutes of cardio after lifting or on separate days, intervals on one of them when a cardio machine or bodyweight allows. The rationale never mentions diet, deficits, or weight loss.
- secondary goals: the primary goal keeps the structure, split, and rep ranges. A secondary goal gets up to about one third of the exercises or one of the days. Cardio secondary to strength, hypertrophy, or power goes after lifting or on separate days, kept short and low-impact; strength secondary to conditioning or general_health means one heavier compound at 4 to 6 reps per session rather than a new day.`
