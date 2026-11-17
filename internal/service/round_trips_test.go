package service

import (
	"context"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// countingProfileRepo counts profile reads; each one is a database round trip.
type countingProfileRepo struct {
	dao.TrainingProfileDAO
	gets int
}

func (r *countingProfileRepo) Get(ctx context.Context, userID uuid.UUID) (*model.TrainingProfile, error) {
	r.gets++
	return r.TrainingProfileDAO.Get(ctx, userID)
}

// countingScheduleRepo counts schedule list reads.
type countingScheduleRepo struct {
	dao.ScheduleDAO
	lists int
}

func (r *countingScheduleRepo) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.ScheduledWorkout, error) {
	r.lists++
	return r.ScheduleDAO.List(ctx, userID, from, to)
}

// TestScheduleListCostDoesNotGrowWithPlannedSessions guards the schedule read
// the app makes on every screen. It used to read the training profile once
// per unfinished session and list the range twice, so a program laid out to
// the horizon cost dozens of round trips to a database in another region.
func TestScheduleListCostDoesNotGrowWithPlannedSessions(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	stored := newCoverageScheduleRepo()
	for day := 20; day <= 31; day++ {
		id := uuid.New()
		stored.workouts[id] = model.ScheduledWorkout{
			ID: id, Date: time.Date(2026, 7, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), Name: "Planned",
			Status: model.WorkoutStatusPlanned, Revision: 1, RequiredSets: []model.ScheduledSet{}, ExtraSets: []model.PerformedSet{},
		}
	}
	profile := coverageProfileFixture()
	profiles := &countingProfileRepo{TrainingProfileDAO: &trainingProfileRepoStub{profile: &profile}}
	schedules := &countingScheduleRepo{ScheduleDAO: stored}
	sessions := newCoverageSessionRepo()
	mutations := newScheduleMutationCapture()
	contract := NewScheduleService(
		schedules, newCoverageProgramRepo(), profiles, sessions,
		&coverageSetRepo{schedules: stored, sessions: sessions}, &coverageParticipationRepo{},
		mutations, newCoverageIdempotencyRepo(), validator.New(),
	)
	svc, ok := contract.(*scheduleService)
	if !ok {
		t.Fatal("unexpected schedule service implementation")
	}
	svc.now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }

	workouts, err := svc.List(ctx, userID, "2026-07-20", "2026-07-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(workouts) != 12 {
		t.Fatalf("listed %d sessions, want 12", len(workouts))
	}
	if profiles.gets != 1 {
		t.Errorf("the profile was read %d times for 12 planned sessions, want 1", profiles.gets)
	}
	if schedules.lists != 1 {
		t.Errorf("the range was listed %d times with nothing to finalize, want 1", schedules.lists)
	}
	if len(mutations.records) != 0 {
		t.Errorf("sessions from today on were finalized: %+v", mutations.records)
	}
}

// yearStatsRepo answers the streak from one list of trained dates.
type yearStatsRepo struct {
	*MockStatsDAO
	dates      []time.Time
	rangeReads int
}

func (r *yearStatsRepo) PerformedDates(_ context.Context, _ uuid.UUID, from, to time.Time) ([]time.Time, error) {
	r.rangeReads++
	const day = "2006-01-02"
	var within []time.Time
	for _, date := range r.dates {
		if key := date.Format(day); key >= from.Format(day) && key <= to.Format(day) {
			within = append(within, date)
		}
	}
	return within, nil
}

// TestWeekStreakReadsAYearOfWeeksPerRoundTrip covers a streak longer than one
// chunk: 60 weeks on target is two range reads, where it was 61 weekly counts.
func TestWeekStreakReadsAYearOfWeeksPerRoundTrip(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	thisMonday := model.MondayOfWeek(time.Now().UTC()).Truncate(24 * time.Hour)
	weeklyCounts := 0
	repo := &yearStatsRepo{MockStatsDAO: &MockStatsDAO{
		GetWeeklyCountFunc: func(context.Context, uuid.UUID, time.Time, time.Time) (int, error) {
			weeklyCounts++
			return 1, nil
		},
	}}
	// This week has one training day; the 59 weeks before it have three each.
	repo.dates = append(repo.dates, thisMonday)
	for week := 1; week <= 59; week++ {
		monday := thisMonday.AddDate(0, 0, -7*week)
		repo.dates = append(repo.dates, monday, monday.AddDate(0, 0, 2), monday.AddDate(0, 0, 4))
	}
	settings := &MockSettingsDAO{GetFunc: func(context.Context, uuid.UUID) (*model.UserSettings, error) {
		return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, nil
	}}

	got, err := NewStatsService(repo, settings).GetSummary(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StreakWeeks != 60 {
		t.Errorf("StreakWeeks = %d, want 60", got.StreakWeeks)
	}
	if repo.rangeReads != 2 {
		t.Errorf("the streak took %d range reads, want 2", repo.rangeReads)
	}
	if weeklyCounts > 1 {
		t.Errorf("the streak still counted %d single weeks, want at most the one for this week's total", weeklyCounts)
	}
}
