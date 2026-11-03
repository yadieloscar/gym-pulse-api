package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type mobilityRepo struct {
	sessions []model.MobilitySession
	created  *model.MobilitySession
	hash     string
	outcome  model.MobilitySessionCreateOutcome
	deleted  uuid.UUID
	listFrom string
	listTo   string
	err      error
}

func (r *mobilityRepo) List(_ context.Context, _ uuid.UUID, from, to string) ([]model.MobilitySession, error) {
	r.listFrom, r.listTo = from, to
	return r.sessions, r.err
}

func (r *mobilityRepo) Get(context.Context, uuid.UUID, uuid.UUID) (*model.MobilitySession, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &r.sessions[0], nil
}

func (r *mobilityRepo) Create(_ context.Context, _ uuid.UUID, session *model.MobilitySession, hash string) (*model.MobilitySession, model.MobilitySessionCreateOutcome, error) {
	r.created, r.hash = session, hash
	if r.err != nil {
		return nil, model.MobilitySessionCreated, r.err
	}
	result := *session
	result.ID = uuid.New()
	return &result, r.outcome, nil
}

func (r *mobilityRepo) Delete(_ context.Context, _ uuid.UUID, id uuid.UUID) error {
	r.deleted = id
	return r.err
}

func newMobilityServiceForTest(repo *mobilityRepo, timezone string) *mobilitySessionService {
	svc := &mobilitySessionService{sessions: repo, profiles: sportProfileRepo{profile: &model.TrainingProfile{Timezone: timezone}}}
	svc.now = func() time.Time { return time.Date(2026, 8, 3, 1, 30, 0, 0, time.UTC) }
	return svc
}

func validMobilityRequest() model.CreateMobilitySessionRequest {
	return model.CreateMobilitySessionRequest{
		Date: "2026-08-02", RoutineID: "hips-and-hamstrings", RoutineName: "Hips and hamstrings",
		Context: model.MobilityContextCooldown, DurationSeconds: 540, MovesCompleted: 7, MovesTotal: 8,
		ContentVersion: "2026.11.1", OperationKey: "mob-op",
	}
}

func TestMobilityServiceCreateNormalizesAndDefaultsToProfileCivilDate(t *testing.T) {
	repo := &mobilityRepo{}
	svc := newMobilityServiceForTest(repo, "America/Los_Angeles")
	req := validMobilityRequest()
	req.Date = ""
	req.RoutineID, req.RoutineName, req.ContentVersion, req.OperationKey = " hips-and-hamstrings ", "  Hips and hamstrings ", " 2026.11.1 ", " mob-op "
	got, _, err := svc.Create(context.Background(), uuid.New(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || repo.created.Date != "2026-08-02" {
		t.Fatalf("date=%+v want athlete-local 2026-08-02", repo.created)
	}
	c := repo.created
	if c.RoutineID != "hips-and-hamstrings" || c.RoutineName != "Hips and hamstrings" || c.ContentVersion != "2026.11.1" || c.OperationKey != "mob-op" {
		t.Fatalf("request not normalized: %+v", c)
	}
	if c.Context != model.MobilityContextCooldown || c.DurationSeconds != 540 || c.MovesCompleted != 7 || c.MovesTotal != 8 || repo.hash == "" {
		t.Fatalf("fields or hash lost: %+v hash=%q", c, repo.hash)
	}
}

func TestMobilityServiceHashIgnoresDateDefaultAndWhitespace(t *testing.T) {
	first, second := &mobilityRepo{}, &mobilityRepo{}
	req := validMobilityRequest()
	req.Date = ""
	if _, _, err := newMobilityServiceForTest(first, "UTC").Create(context.Background(), uuid.New(), req); err != nil {
		t.Fatal(err)
	}
	// The same omitted-date retry the next local day must hash identically.
	svc := newMobilityServiceForTest(second, "UTC")
	svc.now = func() time.Time { return time.Date(2026, 8, 4, 0, 5, 0, 0, time.UTC) }
	req.RoutineName = " " + req.RoutineName + " "
	if _, _, err := svc.Create(context.Background(), uuid.New(), req); err != nil {
		t.Fatal(err)
	}
	if first.hash != second.hash {
		t.Fatal("identical retries must hash identically")
	}
	changed := &mobilityRepo{}
	req.MovesCompleted = 6
	if _, _, err := newMobilityServiceForTest(changed, "UTC").Create(context.Background(), uuid.New(), req); err != nil {
		t.Fatal(err)
	}
	if changed.hash == first.hash {
		t.Fatal("a changed body must hash differently")
	}
}

func TestMobilityServiceCreateValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.CreateMobilitySessionRequest)
		field  string
	}{
		{"future date", func(r *model.CreateMobilitySessionRequest) { r.Date = "2026-08-04" }, "date"},
		{"bad date", func(r *model.CreateMobilitySessionRequest) { r.Date = "08/02/2026" }, "date"},
		{"routine id uppercase", func(r *model.CreateMobilitySessionRequest) { r.RoutineID = "Hips" }, "routine_id"},
		{"routine id underscore", func(r *model.CreateMobilitySessionRequest) { r.RoutineID = "hips_open" }, "routine_id"},
		{"routine id empty", func(r *model.CreateMobilitySessionRequest) { r.RoutineID = "" }, "routine_id"},
		{"routine id long", func(r *model.CreateMobilitySessionRequest) { r.RoutineID = strings.Repeat("a", 65) }, "routine_id"},
		{"routine name blank", func(r *model.CreateMobilitySessionRequest) { r.RoutineName = "   " }, "routine_name"},
		{"routine name long", func(r *model.CreateMobilitySessionRequest) { r.RoutineName = strings.Repeat("é", 81) }, "routine_name"},
		{"context unknown", func(r *model.CreateMobilitySessionRequest) { r.Context = "evening" }, "context"},
		{"context missing", func(r *model.CreateMobilitySessionRequest) { r.Context = "" }, "context"},
		{"duration zero", func(r *model.CreateMobilitySessionRequest) { r.DurationSeconds = 0 }, "duration_seconds"},
		{"duration high", func(r *model.CreateMobilitySessionRequest) { r.DurationSeconds = 7201 }, "duration_seconds"},
		{"moves completed zero", func(r *model.CreateMobilitySessionRequest) { r.MovesCompleted = 0 }, "moves_completed"},
		{"moves total high", func(r *model.CreateMobilitySessionRequest) { r.MovesTotal = 101 }, "moves_total"},
		{"moves total zero", func(r *model.CreateMobilitySessionRequest) { r.MovesTotal = 0 }, "moves_total"},
		{"completed above total", func(r *model.CreateMobilitySessionRequest) { r.MovesCompleted, r.MovesTotal = 9, 8 }, "moves_completed"},
		{"content version empty", func(r *model.CreateMobilitySessionRequest) { r.ContentVersion = " " }, "content_version"},
		{"content version long", func(r *model.CreateMobilitySessionRequest) { r.ContentVersion = strings.Repeat("v", 33) }, "content_version"},
		{"operation key missing", func(r *model.CreateMobilitySessionRequest) { r.OperationKey = "" }, "operation_key"},
		{"operation key long", func(r *model.CreateMobilitySessionRequest) { r.OperationKey = strings.Repeat("k", 129) }, "operation_key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := validMobilityRequest()
			test.mutate(&req)
			repo := &mobilityRepo{}
			_, _, err := newMobilityServiceForTest(repo, "UTC").Create(context.Background(), uuid.New(), req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("error=%v want validation field %s", err, test.field)
			}
			if repo.created != nil {
				t.Fatal("invalid request reached the DAO")
			}
		})
	}
}

func TestMobilityServiceCreateAcceptsBounds(t *testing.T) {
	for _, contextValue := range []model.MobilityContext{model.MobilityContextWarmup, model.MobilityContextCooldown, model.MobilityContextRestDay, model.MobilityContextAnytime} {
		req := validMobilityRequest()
		req.Context = contextValue
		req.RoutineID, req.RoutineName = strings.Repeat("a", 64), strings.Repeat("n", 80)
		req.DurationSeconds, req.MovesCompleted, req.MovesTotal = 7200, 100, 100
		req.ContentVersion, req.OperationKey = strings.Repeat("v", 32), strings.Repeat("k", 128)
		if _, _, err := newMobilityServiceForTest(&mobilityRepo{}, "UTC").Create(context.Background(), uuid.New(), req); err != nil {
			t.Fatalf("context %s at the bounds must pass: %v", contextValue, err)
		}
	}
}

func TestMobilityServiceCreatePropagatesProfileAndTimezoneErrors(t *testing.T) {
	svc := newMobilityServiceForTest(&mobilityRepo{}, "Mars/Olympus")
	var validation *model.ValidationError
	if _, _, err := svc.Create(context.Background(), uuid.New(), validMobilityRequest()); !errors.As(err, &validation) || validation.Field != "timezone" {
		t.Fatalf("error=%v want timezone validation", err)
	}
	svc.profiles = sportProfileRepo{err: &model.NotFoundError{Message: "training profile not found"}}
	var notFound *model.NotFoundError
	if _, _, err := svc.Create(context.Background(), uuid.New(), validMobilityRequest()); !errors.As(err, &notFound) {
		t.Fatalf("error=%v want profile not found", err)
	}
}

func TestMobilityServicePassesThroughOutcomeAndErrors(t *testing.T) {
	repo := &mobilityRepo{outcome: model.MobilitySessionReplayed}
	svc := newMobilityServiceForTest(repo, "UTC")
	if _, outcome, err := svc.Create(context.Background(), uuid.New(), validMobilityRequest()); err != nil || outcome != model.MobilitySessionReplayed {
		t.Fatalf("outcome=%v err=%v", outcome, err)
	}
	repo.err = &model.ConflictError{Message: "idempotency key was already used with a different payload"}
	var conflict *model.ConflictError
	if _, _, err := svc.Create(context.Background(), uuid.New(), validMobilityRequest()); !errors.As(err, &conflict) {
		t.Fatalf("error=%v want conflict", err)
	}
}

func TestMobilityServiceListGetDelete(t *testing.T) {
	entry := model.MobilitySession{ID: uuid.New()}
	repo := &mobilityRepo{sessions: []model.MobilitySession{entry}}
	svc := newMobilityServiceForTest(repo, "UTC")
	ctx := context.Background()
	if got, err := svc.List(ctx, uuid.New(), "2026-01-01", "2027-01-01"); err != nil || len(got) != 1 || repo.listTo != "2027-01-01" {
		t.Fatalf("366-day inclusive range must pass: %v %v", got, err)
	}
	var validation *model.ValidationError
	for _, bad := range [][2]string{{"2026-01-01", "2027-01-02"}, {"2026-02-02", "2026-02-01"}, {"", "2026-02-01"}, {"2026-02-01", "bad"}} {
		if _, err := svc.List(ctx, uuid.New(), bad[0], bad[1]); !errors.As(err, &validation) {
			t.Fatalf("range %v must be a validation error, got %v", bad, err)
		}
	}
	if got, err := svc.Get(ctx, uuid.New(), entry.ID); err != nil || got.ID != entry.ID {
		t.Fatalf("get=%v err=%v", got, err)
	}
	if err := svc.Delete(ctx, uuid.New(), entry.ID); err != nil || repo.deleted != entry.ID {
		t.Fatalf("delete err=%v deleted=%v", err, repo.deleted)
	}
}

func TestNewMobilitySessionServiceUsesWallClock(t *testing.T) {
	svc, ok := NewMobilitySessionService(&mobilityRepo{}, sportProfileRepo{}).(*mobilitySessionService)
	if !ok || svc.now == nil {
		t.Fatal("constructor must return a wired service")
	}
}
