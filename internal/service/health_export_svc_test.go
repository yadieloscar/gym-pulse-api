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

type healthExportRepo struct {
	recorded *model.HealthExport
	// ledger enables idempotency-ledger behavior: stored remembers the first
	// record, repeats of its key replay or conflict on the hash.
	ledger  bool
	stored  *model.HealthExport
	key     string
	hash    string
	outcome model.HealthExportOutcome
	listed  []model.HealthExport
	err     error
}

// Record passes outcome through. With ledger set it mirrors the DAO's
// idempotency ledger: a repeated operation key with the same request hash
// replays the stored record; a different hash conflicts.
func (r *healthExportRepo) Record(_ context.Context, _ uuid.UUID, export *model.HealthExport, key, hash string) (*model.HealthExport, model.HealthExportOutcome, error) {
	if r.err != nil {
		r.recorded, r.key, r.hash = export, key, hash
		return nil, model.HealthExportRecorded, r.err
	}
	if r.ledger && r.stored != nil && r.key == key {
		if r.hash != hash {
			return nil, model.HealthExportRecorded, &model.ConflictError{Message: "operation_key reused with a different payload"}
		}
		return r.stored, model.HealthExportReplayed, nil
	}
	r.recorded, r.key, r.hash = export, key, hash
	result := *export
	result.ID = uuid.New()
	r.stored = &result
	return &result, r.outcome, nil
}

func (r *healthExportRepo) List(context.Context, uuid.UUID, string, string) ([]model.HealthExport, error) {
	return r.listed, r.err
}

func newHealthExportServiceForTest(repo *healthExportRepo) *healthExportService {
	return &healthExportService{exports: repo, now: func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) }}
}

func validExportRequest() model.RecordHealthExportRequest {
	return model.RecordHealthExportRequest{Source: model.HealthExportSourceAppleHealth, ExternalID: "  B5D7-1  ", OperationKey: " hk-export-1 "}
}

func TestHealthExportServiceRecordNormalizesAndDefaultsExportedAt(t *testing.T) {
	repo := &healthExportRepo{outcome: model.HealthExportExisting}
	workoutID := uuid.New()
	got, outcome, err := newHealthExportServiceForTest(repo).Record(context.Background(), uuid.New(), "scheduled_workout", workoutID, validExportRequest())
	if err != nil {
		t.Fatal(err)
	}
	if outcome != model.HealthExportExisting || got == nil {
		t.Fatalf("outcome=%v want DAO outcome passed through", outcome)
	}
	if repo.recorded.ExternalID != "B5D7-1" || repo.key != "hk-export-1" || repo.hash == "" {
		t.Fatalf("not normalized: %+v key=%q hash=%q", repo.recorded, repo.key, repo.hash)
	}
	if repo.recorded.WorkoutKind != model.HealthExportKindScheduledWorkout || repo.recorded.WorkoutID != workoutID {
		t.Fatalf("kind or id not carried: %+v", repo.recorded)
	}
	if !repo.recorded.ExportedAt.Equal(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("exported_at should default to now, got %v", repo.recorded.ExportedAt)
	}
}

func TestHealthExportServiceClampsFutureExportedAt(t *testing.T) {
	repo := &healthExportRepo{}
	future := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2026, 10, 13, 18, 42, 0, 0, time.FixedZone("EDT", -4*3600))
	req := validExportRequest()
	req.ExportedAt = &future
	if _, _, err := newHealthExportServiceForTest(repo).Record(context.Background(), uuid.New(), "day_log", uuid.New(), req); err != nil {
		t.Fatal(err)
	}
	if !repo.recorded.ExportedAt.Equal(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("future exported_at should clamp to now, got %v", repo.recorded.ExportedAt)
	}
	req.ExportedAt = &past
	if _, _, err := newHealthExportServiceForTest(repo).Record(context.Background(), uuid.New(), "day_log", uuid.New(), req); err != nil {
		t.Fatal(err)
	}
	if !repo.recorded.ExportedAt.Equal(past) || repo.recorded.ExportedAt.Location() != time.UTC {
		t.Fatalf("past exported_at should be kept in UTC, got %v", repo.recorded.ExportedAt)
	}
}

// A client that omits exported_at and retries the same operation after the
// clock moved must replay 201, not hit IDEMPOTENCY_CONFLICT: the request hash
// is computed from what the client sent, not from the server-defaulted time.
func TestHealthExportServiceVerbatimRetryWithoutExportedAtReplays(t *testing.T) {
	repo := &healthExportRepo{ledger: true}
	clock := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	svc := &healthExportService{exports: repo, now: func() time.Time { return clock }}
	userID, workoutID := uuid.New(), uuid.New()
	req := model.RecordHealthExportRequest{Source: model.HealthExportSourceAppleHealth, ExternalID: "B5D7-1", OperationKey: "hk-export-1"}

	first, outcome, err := svc.Record(context.Background(), userID, "scheduled_workout", workoutID, req)
	if err != nil || outcome != model.HealthExportRecorded {
		t.Fatalf("first call outcome=%v err=%v", outcome, err)
	}
	if !first.ExportedAt.Equal(clock) {
		t.Fatalf("exported_at should default to now, got %v", first.ExportedAt)
	}
	firstHash := repo.hash

	clock = clock.Add(90 * time.Second)
	second, outcome, err := svc.Record(context.Background(), userID, "scheduled_workout", workoutID, req)
	if err != nil {
		t.Fatalf("verbatim retry must not conflict: %v", err)
	}
	if outcome != model.HealthExportReplayed || second.ID != first.ID {
		t.Fatalf("retry outcome=%v id=%v want replay of %v", outcome, second.ID, first.ID)
	}
	if repo.hash != firstHash {
		t.Fatalf("request hash changed between verbatim retries: %q then %q", firstHash, repo.hash)
	}

	// A different payload under the same key must still be detected: the hash
	// has to cover the client-sent fields.
	changed := req
	changed.ExternalID = "B5D7-2"
	var conflict *model.ConflictError
	if _, _, err := svc.Record(context.Background(), userID, "scheduled_workout", workoutID, changed); !errors.As(err, &conflict) {
		t.Fatalf("changed payload under the same key should conflict, got %v", err)
	}
	sent := time.Date(2026, 10, 13, 18, 42, 0, 0, time.UTC)
	changed = req
	changed.ExportedAt = &sent
	if _, _, err := svc.Record(context.Background(), userID, "scheduled_workout", workoutID, changed); !errors.As(err, &conflict) {
		t.Fatalf("client-sent exported_at must be part of the hash, got %v", err)
	}
}

func TestHealthExportServiceValidation(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		edit  func(*model.RecordHealthExportRequest)
		field string
	}{
		{"unknown kind", "session", func(*model.RecordHealthExportRequest) {}, "workout_kind"},
		{"unknown source", "day_log", func(r *model.RecordHealthExportRequest) { r.Source = "garmin" }, "source"},
		{"blank identity", "day_log", func(r *model.RecordHealthExportRequest) { r.ExternalID = "   " }, "external_id"},
		{"long identity", "day_log", func(r *model.RecordHealthExportRequest) { r.ExternalID = strings.Repeat("x", 129) }, "external_id"},
		{"missing key", "day_log", func(r *model.RecordHealthExportRequest) { r.OperationKey = "" }, "operation_key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &healthExportRepo{}
			req := validExportRequest()
			test.edit(&req)
			_, _, err := newHealthExportServiceForTest(repo).Record(context.Background(), uuid.New(), test.kind, uuid.New(), req)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("error=%v want validation field %s", err, test.field)
			}
			if repo.recorded != nil {
				t.Fatal("invalid request must not reach the repository")
			}
		})
	}
}

func TestHealthExportServiceListValidatesRangeAndPassesThrough(t *testing.T) {
	want := []model.HealthExport{{ID: uuid.New(), Date: "2026-10-13"}}
	repo := &healthExportRepo{listed: want}
	svc := newHealthExportServiceForTest(repo)
	got, err := svc.List(context.Background(), uuid.New(), "2026-10-01", "2026-10-14")
	if err != nil || len(got) != 1 || got[0].ID != want[0].ID {
		t.Fatalf("list=%+v error=%v", got, err)
	}
	for _, dates := range [][2]string{{"bad", "2026-10-14"}, {"2026-10-15", "2026-10-14"}, {"2025-01-01", "2026-10-14"}} {
		if _, err := svc.List(context.Background(), uuid.New(), dates[0], dates[1]); err == nil {
			t.Fatalf("range %v unexpectedly accepted", dates)
		}
	}
	repo.err = errors.New("database unavailable")
	if _, err := svc.List(context.Background(), uuid.New(), "2026-10-01", "2026-10-14"); err == nil {
		t.Fatal("list error not returned")
	}
	if _, _, err := svc.Record(context.Background(), uuid.New(), "day_log", uuid.New(), validExportRequest()); err == nil {
		t.Fatal("record error not returned")
	}
}
