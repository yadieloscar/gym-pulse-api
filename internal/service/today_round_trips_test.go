package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// The Today screen reads the schedule and participation on every open. These
// tests pin how many reads each costs; every read is a round trip to a
// database in another region.

const todayFixture = "2026-07-20" // a Monday in UTC

// batchScheduleRepo answers ReadSchedule from its stored sessions, the way the
// DAO does in one round trip, and counts every read.
type batchScheduleRepo struct {
	*rollingScheduleRepo
	profile          *model.TrainingProfile
	activeProgram    bool
	scheduledThrough string
	reads, lists     int
}

func (r *batchScheduleRepo) ReadSchedule(ctx context.Context, userID uuid.UUID, from, to string) (*dao.ScheduleRead, error) {
	r.reads++
	read := &dao.ScheduleRead{Profile: r.profile, ActiveProgram: r.activeProgram, ScheduledThrough: r.scheduledThrough}
	if r.profile == nil {
		read.ProfileErr = &model.NotFoundError{Message: "training profile not found"}
	}
	workouts, err := r.coverageScheduleRepo.List(ctx, userID, from, to)
	read.Workouts = workouts
	return read, err
}

func (r *batchScheduleRepo) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.ScheduledWorkout, error) {
	r.lists++
	return r.coverageScheduleRepo.List(ctx, userID, from, to)
}

// batchParticipationRepo answers ReadParticipation from the schedule and
// participation fixtures, the way the DAO does in one round trip.
type batchParticipationRepo struct {
	*coverageParticipationRepo
	schedule *batchScheduleRepo
	reads    int
	lists    int
}

func (r *batchParticipationRepo) ReadParticipation(ctx context.Context, userID uuid.UUID, from, to string) (*dao.ParticipationRead, error) {
	r.reads++
	read := &dao.ParticipationRead{Profile: r.schedule.profile, Unfinalized: []model.ScheduledWorkout{}}
	if r.schedule.profile == nil {
		read.ProfileErr = &model.NotFoundError{Message: "training profile not found"}
	}
	workouts, _ := r.schedule.coverageScheduleRepo.List(ctx, userID, from, to)
	for _, w := range workouts {
		if w.FinalizedAt == nil {
			read.Unfinalized = append(read.Unfinalized, model.ScheduledWorkout{ID: w.ID, Date: w.Date, Revision: w.Revision})
		}
	}
	read.Days, _ = r.coverageParticipationRepo.List(ctx, userID, from, to)
	return read, nil
}

func (r *batchParticipationRepo) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.DayParticipation, error) {
	r.lists++
	return r.coverageParticipationRepo.List(ctx, userID, from, to)
}

type todayFixtures struct {
	schedules     *batchScheduleRepo
	participation *batchParticipationRepo
	profiles      *countingProfileRepo
	mutations     *scheduleMutationCapture
	schedule      *scheduleService
	reader        ParticipationService
}

func newTodayFixtures(t *testing.T, dates ...string) *todayFixtures {
	t.Helper()
	profile := coverageProfileFixture()
	stored := newCoverageScheduleRepo()
	for _, date := range dates {
		id := uuid.New()
		stored.workouts[id] = model.ScheduledWorkout{
			ID: id, Date: date, Name: "Planned", Status: model.WorkoutStatusPlanned, Revision: 1,
			RequiredSets: []model.ScheduledSet{}, ExtraSets: []model.PerformedSet{},
		}
	}
	f := &todayFixtures{
		schedules: &batchScheduleRepo{
			rollingScheduleRepo: &rollingScheduleRepo{coverageScheduleRepo: stored},
			profile:             &profile, activeProgram: true, scheduledThrough: "2026-12-31",
		},
		profiles:  &countingProfileRepo{TrainingProfileDAO: &trainingProfileRepoStub{profile: &profile}},
		mutations: newScheduleMutationCapture(),
	}
	f.participation = &batchParticipationRepo{coverageParticipationRepo: &coverageParticipationRepo{}, schedule: f.schedules}
	sessions := newCoverageSessionRepo()
	contract := NewScheduleService(f.schedules, newCoverageProgramRepo(), f.profiles, sessions,
		&coverageSetRepo{schedules: stored, sessions: sessions}, f.participation, f.mutations,
		newCoverageIdempotencyRepo(), validator.New())
	schedule, ok := contract.(*scheduleService)
	if !ok {
		t.Fatal("unexpected schedule service implementation")
	}
	f.schedule = schedule
	f.schedule.now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }
	f.reader = NewParticipationService(f.schedule, f.participation)
	return f
}

func TestScheduleListIsOneReadWhenNothingNeedsWork(t *testing.T) {
	f := newTodayFixtures(t, "2026-07-20", "2026-07-22", "2026-07-24")

	workouts, err := f.schedule.List(context.Background(), uuid.New(), todayFixture, "2026-08-09")
	if err != nil {
		t.Fatal(err)
	}
	if len(workouts) != 3 {
		t.Fatalf("listed %d sessions, want 3", len(workouts))
	}
	if f.schedules.reads != 1 || f.schedules.lists != 0 || f.profiles.gets != 0 || len(f.schedules.fills) != 0 {
		t.Fatalf("reads=%d lists=%d profile gets=%d fills=%d; want 1, 0, 0, 0",
			f.schedules.reads, f.schedules.lists, f.profiles.gets, len(f.schedules.fills))
	}
}

func TestScheduleListFillsOnlyWhenTheWatermarkIsShort(t *testing.T) {
	horizon := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC).AddDate(0, 0, model.ScheduleHorizonDays).Format(time.DateOnly)
	for _, tc := range []struct {
		name    string
		active  bool
		through string
		to      string
		fills   int
	}{
		{name: "filled to the horizon", active: true, through: horizon, to: "2026-08-09"},
		{name: "one day short", active: true, through: time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC).AddDate(0, 0, model.ScheduleHorizonDays).Format(time.DateOnly), to: "2026-08-09", fills: 1},
		{name: "never filled", active: true, to: "2026-08-09", fills: 1},
		{name: "no active program", active: false, to: "2026-08-09"},
		{name: "a range before today", active: true, to: "2026-07-19"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTodayFixtures(t, "2026-07-22")
			f.schedules.activeProgram, f.schedules.scheduledThrough = tc.active, tc.through

			if _, err := f.schedule.List(context.Background(), uuid.New(), "2026-07-13", tc.to); err != nil {
				t.Fatal(err)
			}
			if len(f.schedules.fills) != tc.fills || f.schedules.lists != tc.fills {
				t.Fatalf("fills=%d lists=%d, want %d each", len(f.schedules.fills), f.schedules.lists, tc.fills)
			}
			if tc.fills == 1 && f.schedules.fills[0] != todayFixture {
				t.Fatalf("filled from %s, want %s", f.schedules.fills[0], todayFixture)
			}
		})
	}
}

func TestScheduleListFinalizesThePastThenReadsItBack(t *testing.T) {
	f := newTodayFixtures(t, "2026-07-15", "2026-07-22")

	if _, err := f.schedule.List(context.Background(), uuid.New(), "2026-07-13", "2026-08-09"); err != nil {
		t.Fatal(err)
	}
	if len(f.mutations.records) != 1 || f.schedules.reads != 1 || f.schedules.lists != 1 {
		t.Fatalf("finalizes=%d reads=%d lists=%d, want 1 each", len(f.mutations.records), f.schedules.reads, f.schedules.lists)
	}
}

func TestScheduleListWithoutAProfile(t *testing.T) {
	f := newTodayFixtures(t, "2026-07-22")
	f.schedules.profile = nil
	var notFound *model.NotFoundError
	if _, err := f.schedule.List(context.Background(), uuid.New(), "2026-07-13", "2026-08-09"); !errors.As(err, &notFound) {
		t.Fatalf("an unfinalized session without a profile: err = %v, want NotFoundError", err)
	}

	empty := newTodayFixtures(t)
	empty.schedules.profile = nil
	if workouts, err := empty.schedule.List(context.Background(), uuid.New(), "2026-07-13", "2026-08-09"); err != nil || len(workouts) != 0 {
		t.Fatalf("nothing to judge without a profile: %v, %v; want no sessions and no error", workouts, err)
	}
}

func TestParticipationIsOneReadWhenNothingNeedsFinalizing(t *testing.T) {
	f := newTodayFixtures(t, "2026-07-20", "2026-07-22")
	f.participation.outcomes = []model.DayParticipation{{Date: "2026-07-17", Participated: true}}

	days, err := f.reader.List(context.Background(), uuid.New(), "2026-07-13", "2026-07-19")
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Date != "2026-07-17" {
		t.Fatalf("days = %+v", days)
	}
	if f.participation.reads != 1 || f.participation.lists != 0 || f.schedules.reads != 0 || f.schedules.lists != 0 || len(f.schedules.fills) != 0 {
		t.Fatalf("participation reads=%d lists=%d, schedule reads=%d lists=%d fills=%d; want 1 and nothing else",
			f.participation.reads, f.participation.lists, f.schedules.reads, f.schedules.lists, len(f.schedules.fills))
	}
}

func TestParticipationFinalizesThePastThenReadsItBack(t *testing.T) {
	f := newTodayFixtures(t, "2026-07-15", "2026-07-20")

	if _, err := f.reader.List(context.Background(), uuid.New(), "2026-07-13", "2026-07-26"); err != nil {
		t.Fatal(err)
	}
	if len(f.mutations.records) != 1 {
		t.Fatalf("finalized %d sessions, want only the past one", len(f.mutations.records))
	}
	if f.participation.reads != 1 || f.participation.lists != 1 {
		t.Fatalf("reads=%d lists=%d, want 1 each", f.participation.reads, f.participation.lists)
	}
}

func TestParticipationRejectsWhatTheScheduleReadRejects(t *testing.T) {
	f := newTodayFixtures(t, "2026-07-15")
	if _, err := f.reader.List(context.Background(), uuid.New(), "2026-07-20", "2026-07-13"); err == nil {
		t.Fatal("a reversed range was accepted")
	}
	f.schedules.profile = nil
	var notFound *model.NotFoundError
	if _, err := f.reader.List(context.Background(), uuid.New(), "2026-07-13", "2026-07-19"); !errors.As(err, &notFound) {
		t.Fatalf("an unfinalized session without a profile: err = %v, want NotFoundError", err)
	}
	f.schedules.profile = func() *model.TrainingProfile { p := coverageProfileFixture(); p.Timezone = "Not/AZone"; return &p }()
	var invalid *model.ValidationError
	if _, err := f.reader.List(context.Background(), uuid.New(), "2026-07-13", "2026-07-19"); !errors.As(err, &invalid) {
		t.Fatalf("an invalid saved timezone: err = %v, want ValidationError", err)
	}
}

// legacyParticipationRepo hides ReadParticipation, so the service takes the
// path it took before the one-round-trip read.
type legacyParticipationRepo struct{ dao.ParticipationDAO }

func TestParticipationAnswersMatchTheFullScheduleRead(t *testing.T) {
	dates := []string{"2026-07-13", "2026-07-15", "2026-07-17", "2026-07-20", "2026-07-22"}
	batched, full := newTodayFixtures(t, dates...), newTodayFixtures(t, dates...)
	legacy := NewParticipationService(full.schedule, legacyParticipationRepo{full.participation})
	// Finalize records a day outcome the way the DAO does, so both paths have
	// something to return.
	for _, f := range []*todayFixtures{batched, full} {
		f.participation.outcomes = []model.DayParticipation{{Date: "2026-07-14", Participated: false}}
	}

	got, err := batched.reader.List(context.Background(), uuid.New(), "2026-07-13", "2026-07-19")
	if err != nil {
		t.Fatal(err)
	}
	want, err := legacy.List(context.Background(), uuid.New(), "2026-07-13", "2026-07-19")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("one-read participation = %+v, full schedule read = %+v", got, want)
	}
	if len(batched.mutations.records) != len(full.mutations.records) {
		t.Fatalf("finalized %d sessions, the full read finalized %d", len(batched.mutations.records), len(full.mutations.records))
	}
	if full.schedules.reads != 1 || batched.schedules.reads != 0 {
		t.Fatalf("schedule reads: full=%d batched=%d, want 1 and 0", full.schedules.reads, batched.schedules.reads)
	}
}
