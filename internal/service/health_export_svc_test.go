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
	key      string
	hash     string
	outcome  model.HealthExportOutcome
	listed   []model.HealthExport
	err      error
}

func (r *healthExportRepo) Record(_ context.Context, _ uuid.UUID, export *model.HealthExport, key, hash string) (*model.HealthExport, model.HealthExportOutcome, error) {
	r.recorded, r.key, r.hash = export, key, hash
	if r.err != nil {
		return nil, model.HealthExportRecorded, r.err
	}
	result := *export
	result.ID = uuid.New()
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
