package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type fakeGenProfileDAO struct {
	profile *model.TrainingProfile
	calls   *[]string
}

func (f *fakeGenProfileDAO) Get(context.Context, uuid.UUID) (*model.TrainingProfile, error) {
	*f.calls = append(*f.calls, "profile")
	if f.profile == nil {
		return nil, &model.NotFoundError{Message: "training profile not found"}
	}
	return f.profile, nil
}

func (f *fakeGenProfileDAO) Put(context.Context, uuid.UUID, *model.TrainingProfile, int64) error {
	*f.calls = append(*f.calls, "profile-write")
	return nil
}

type fakeGenCatalogDAO struct {
	catalog []model.CatalogExercise
	calls   *[]string
}

func (f *fakeGenCatalogDAO) List(context.Context, string) ([]model.CatalogExercise, error) {
	*f.calls = append(*f.calls, "catalog")
	return f.catalog, nil
}

type fakeLedger struct {
	used      int
	limitHit  bool
	completed []string
	calls     *[]string
}

func (f *fakeLedger) Reserve(_ context.Context, _ uuid.UUID, localDate string, limit int, _ string) (*dao.ProgramGenerationReservation, error) {
	*f.calls = append(*f.calls, "reserve:"+localDate)
	if f.limitHit || f.used >= limit {
		return nil, &model.GenerationLimitError{Limit: limit}
	}
	f.used++
	return &dao.ProgramGenerationReservation{ID: uuid.New(), Used: f.used}, nil
}

func (f *fakeLedger) Complete(_ context.Context, _ uuid.UUID, status string) error {
	*f.calls = append(*f.calls, "complete:"+status)
	f.completed = append(f.completed, status)
	if status != model.GenerationSucceeded {
		f.used--
	}
	return nil
}

func (f *fakeLedger) CountToday(context.Context, uuid.UUID, string) (int, error) {
	*f.calls = append(*f.calls, "count")
	return f.used, nil
}

type fakeGenerator struct {
	raw   *model.RawGeneratedDraft
	err   error
	input GenerationInput
	calls *[]string
	block bool
}

func (f *fakeGenerator) Generate(ctx context.Context, input GenerationInput) (*model.RawGeneratedDraft, GenerationUsage, error) {
	*f.calls = append(*f.calls, "generate")
	f.input = input
	if f.block {
		<-ctx.Done()
		return nil, GenerationUsage{}, ErrGeneratorUnavailable
	}
	return f.raw, GenerationUsage{InputTokens: 10, OutputTokens: 5}, f.err
}

func genCatalog() []model.CatalogExercise {
	compound := "compound"
	return []model.CatalogExercise{
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Name: "Barbell Bench Press", Category: "push", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), Name: "Barbell Row", Category: "pull", Modality: "strength", Mechanic: &compound},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000003"), Name: "Back Squat", Category: "legs", Modality: "strength", Mechanic: &compound},
	}
}

func genRaw() *model.RawGeneratedDraft {
	reps := 8
	ex := func(id string) model.RawGeneratedExercise {
		return model.RawGeneratedExercise{CatalogID: "00000000-0000-0000-0000-00000000000" + id, TargetSets: 3, TargetReps: &reps}
	}
	return &model.RawGeneratedDraft{Name: "Full body", Rationale: "Three compound lifts.", Workouts: []model.RawGeneratedWorkout{{Name: "Day A", Exercises: []model.RawGeneratedExercise{ex("1"), ex("2"), ex("3")}}}}
}

type genFixture struct {
	svc       *programGenerationService
	profiles  *fakeGenProfileDAO
	ledger    *fakeLedger
	generator *fakeGenerator
	calls     []string
}

func newGenFixture(t *testing.T, profile *model.TrainingProfile, generator *fakeGenerator) *genFixture {
	t.Helper()
	f := &genFixture{}
	f.profiles = &fakeGenProfileDAO{profile: profile, calls: &f.calls}
	f.ledger = &fakeLedger{calls: &f.calls}
	if generator != nil {
		generator.calls = &f.calls
	}
	f.generator = generator
	var gen DraftGenerator
	if generator != nil {
		gen = generator
	}
	svc, ok := NewProgramGenerationService(f.profiles, &fakeGenCatalogDAO{catalog: genCatalog(), calls: &f.calls}, f.ledger, gen, 5, "claude-opus-5-5").(*programGenerationService)
	if !ok {
		t.Fatal("unexpected service type")
	}
	f.svc = svc
	f.svc.now = func() time.Time { return time.Date(2026, 10, 31, 3, 30, 0, 0, time.UTC) } // 2026-10-30 23:30 New York
	return f
}

func genProfile() *model.TrainingProfile {
	return &model.TrainingProfile{PrimaryGoal: "hypertrophy", AvailableDays: []int{1, 3, 5}, UsualActivity: "moderate", Experience: "intermediate", Equipment: []string{"barbell"}, SessionDurationMinutes: 60, Timezone: "America/New_York"}
}

var newYorkMidnight = time.Date(2026, 10, 31, 4, 0, 0, 0, time.UTC)

func TestProgramGenerationStatus(t *testing.T) {
	f := newGenFixture(t, genProfile(), &fakeGenerator{raw: genRaw()})
	f.ledger.used = 2
	status, err := f.svc.Status(context.Background(), uuid.New())
	if err != nil || !status.Enabled || status.Limit != 5 || status.RemainingToday != 3 || status.ResetsAt == nil || !status.ResetsAt.Equal(newYorkMidnight) {
		t.Fatalf("status: %+v %v", status, err)
	}

	disabled := newGenFixture(t, nil, nil)
	status, err = disabled.svc.Status(context.Background(), uuid.New())
	if err != nil || status.Enabled || status.RemainingToday != 5 || status.ResetsAt != nil {
		t.Fatalf("disabled status without profile: %+v %v", status, err)
	}
	for _, call := range disabled.calls {
		if call == "count" {
			t.Fatal("no profile means no ledger read")
		}
	}
}

func TestProgramGenerationGenerateHappyPath(t *testing.T) {
	f := newGenFixture(t, genProfile(), &fakeGenerator{raw: genRaw()})
	request := "  no deadlifts "
	resp, err := f.svc.Generate(context.Background(), uuid.New(), model.GenerateProgramRequest{Request: &request})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want := []string{"profile", "reserve:2026-10-30", "catalog", "generate", "complete:succeeded"}
	if len(f.calls) != len(want) {
		t.Fatalf("call order %v, want %v", f.calls, want)
	}
	for i := range want {
		if f.calls[i] != want[i] {
			t.Fatalf("call order %v, want %v", f.calls, want)
		}
	}
	if resp.RemainingToday != 4 || !resp.ResetsAt.Equal(newYorkMidnight) || *resp.Request != "no deadlifts" || resp.Draft.Roadmap["request"] != "no deadlifts" {
		t.Fatalf("response bookkeeping wrong: %+v", resp)
	}
	if resp.Draft.PrimaryGoal != "hypertrophy" || len(resp.Draft.Workouts) != 1 || resp.Draft.Workouts[0].PreferredWeekday != 1 {
		t.Fatalf("draft wrong: %+v", resp.Draft)
	}
	in := f.generator.input
	if in.WorkoutCap != 3 || in.Model != "claude-opus-5-5" || len(in.Catalog) != 3 || *in.Request != "no deadlifts" || in.SessionDurationMinutes != 60 {
		t.Fatalf("generation input wrong: %+v", in)
	}
	// Nothing but the ledger is written.
	for _, call := range f.calls {
		if call == "profile-write" {
			t.Fatal("generation must not write the profile")
		}
	}
}

func TestProgramGenerationGenerateRejectsBeforeReserving(t *testing.T) {
	disabled := newGenFixture(t, genProfile(), nil)
	_, err := disabled.svc.Generate(context.Background(), uuid.New(), model.GenerateProgramRequest{})
	var unavailable *model.GenerationUnavailableError
	if !errors.As(err, &unavailable) || unavailable.Reason != model.GenerationDisabled || len(disabled.calls) != 0 {
		t.Fatalf("disabled must answer before any call: %v %v", err, disabled.calls)
	}

	noProfile := newGenFixture(t, nil, &fakeGenerator{raw: genRaw()})
	_, err = noProfile.svc.Generate(context.Background(), uuid.New(), model.GenerateProgramRequest{})
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "training_profile" {
		t.Fatalf("missing profile must be a validation error on training_profile: %v", err)
	}
	if len(noProfile.calls) != 1 || noProfile.calls[0] != "profile" {
		t.Fatalf("missing profile must not reserve or generate: %v", noProfile.calls)
	}

	badRequest := newGenFixture(t, genProfile(), &fakeGenerator{raw: genRaw()})
	blank := "   "
	_, err = badRequest.svc.Generate(context.Background(), uuid.New(), model.GenerateProgramRequest{Request: &blank})
	if !errors.As(err, &validation) || validation.Field != "request" || len(badRequest.calls) != 0 {
		t.Fatalf("blank request must fail before any call: %v %v", err, badRequest.calls)
	}
}

func TestProgramGenerationLimitStopsBeforeTheProvider(t *testing.T) {
	f := newGenFixture(t, genProfile(), &fakeGenerator{raw: genRaw()})
	f.ledger.used = 5
	_, err := f.svc.Generate(context.Background(), uuid.New(), model.GenerateProgramRequest{})
	var limit *model.GenerationLimitError
	if !errors.As(err, &limit) || limit.Limit != 5 || !limit.ResetsAt.Equal(newYorkMidnight) {
		t.Fatalf("want limit error with local midnight, got %v", err)
	}
	for _, call := range f.calls {
		if call == "generate" || call == "catalog" {
			t.Fatalf("limit must stop before the provider: %v", f.calls)
		}
	}
}

func TestProgramGenerationFailuresNeverCount(t *testing.T) {
	cases := map[string]*fakeGenerator{
		"provider unavailable": {err: ErrGeneratorUnavailable},
		"provider refused":     {err: ErrGeneratorRefused},
		"invalid draft":        {raw: &model.RawGeneratedDraft{Name: "x", Workouts: []model.RawGeneratedWorkout{{Name: "y"}}}},
	}
	for name, generator := range cases {
		t.Run(name, func(t *testing.T) {
			f := newGenFixture(t, genProfile(), generator)
			_, err := f.svc.Generate(context.Background(), uuid.New(), model.GenerateProgramRequest{})
			var unavailable *model.GenerationUnavailableError
			if !errors.As(err, &unavailable) || unavailable.Reason != model.GenerationUnavailable {
				t.Fatalf("want unavailable, got %v", err)
			}
			if len(f.ledger.completed) != 1 || f.ledger.completed[0] != model.GenerationFailed || f.ledger.used != 0 {
				t.Fatalf("failure must complete the row as failed and not count: %v used=%d", f.ledger.completed, f.ledger.used)
			}
		})
	}
}

func TestProgramGenerationTimeoutAnswersWithinTheDeadline(t *testing.T) {
	f := newGenFixture(t, genProfile(), &fakeGenerator{block: true})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := f.svc.Generate(ctx, uuid.New(), model.GenerateProgramRequest{})
	var unavailable *model.GenerationUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("service did not return at the deadline")
	}
	if len(f.ledger.completed) != 1 || f.ledger.completed[0] != model.GenerationFailed {
		t.Fatalf("a cancelled call must still be recorded as failed: %v", f.ledger.completed)
	}
}
