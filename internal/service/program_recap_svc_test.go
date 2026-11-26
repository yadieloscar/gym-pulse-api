package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// recapSessions returns one session per date with the same status.
func recapSessions(status string, dates ...string) []dao.ProgramRecapSession {
	sessions := make([]dao.ProgramRecapSession, 0, len(dates))
	for _, date := range dates {
		sessions = append(sessions, dao.ProgramRecapSession{Date: date, Status: status})
	}
	return sessions
}

// Four weeks of Monday, Wednesday, Friday from 2026-09-07 (a Monday).
var recapFourWeeks = []string{
	"2026-09-07", "2026-09-09", "2026-09-11",
	"2026-09-14", "2026-09-16", "2026-09-18",
	"2026-09-21", "2026-09-23", "2026-09-25",
	"2026-09-28", "2026-09-30", "2026-10-02",
}

func recapDate(date string) *string { return &date }

func TestBuildProgramRecap(t *testing.T) {
	const later = "2026-10-20"
	done, missed, planned := model.WorkoutStatusCompleted, model.WorkoutStatusMissed, model.WorkoutStatusPlanned

	tests := []struct {
		name  string
		read  dao.ProgramRecapRead
		today string
		want  model.ProgramRecap
	}{
		{
			name: "full program",
			read: dao.ProgramRecapRead{
				TrainingDays: 3,
				Sessions:     recapSessions(done, recapFourWeeks...),
				TrainedDates: recapFourWeeks,
				LiftDays: []dao.ProgramRecapLiftDay{
					{Name: "Back Squat", Date: "2026-09-07", Weight: 185, Reps: 5},
					{Name: "Back Squat", Date: "2026-09-18", Weight: 195, Reps: 5},
					{Name: "Back Squat", Date: "2026-10-02", Weight: 205, Reps: 3},
				},
			},
			today: later,
			want: model.ProgramRecap{
				FirstDate: recapDate("2026-09-07"), LastDate: recapDate("2026-10-02"),
				Weeks: model.ProgramRecapWeeks{Total: 4, OnTarget: 4}, WorkoutsDone: 12,
				Lifts: []model.ProgramRecapLift{{
					Name: "Back Squat", Days: 3,
					First: model.ProgramRecapSet{Weight: 185, Reps: 5, Date: "2026-09-07"},
					Last:  model.ProgramRecapSet{Weight: 205, Reps: 3, Date: "2026-10-02"},
				}},
			},
		},
		{
			// Week 1 is done in full. Week 2 has two of three. The athlete
			// started another program on the Tuesday of week 3, so that week
			// has one session and is held to one day.
			name: "partly done and left early",
			read: dao.ProgramRecapRead{
				TrainingDays: 3,
				Sessions: append(append(
					recapSessions(done, "2026-09-07", "2026-09-09", "2026-09-11", "2026-09-14"),
					dao.ProgramRecapSession{Date: "2026-09-16", Status: model.WorkoutStatusIncomplete},
					dao.ProgramRecapSession{Date: "2026-09-18", Status: missed}),
					recapSessions(done, "2026-09-21")...),
				TrainedDates: []string{"2026-09-07", "2026-09-09", "2026-09-11", "2026-09-14", "2026-09-16", "2026-09-21"},
			},
			today: later,
			want: model.ProgramRecap{
				FirstDate: recapDate("2026-09-07"), LastDate: recapDate("2026-09-21"),
				Weeks: model.ProgramRecapWeeks{Total: 3, OnTarget: 2}, WorkoutsDone: 6,
				Lifts: []model.ProgramRecapLift{},
			},
		},
		{
			// Week 2 is off and untouched: it is not one of the program's
			// weeks. Week 3 is off but reached the target: it counts.
			name: "week off",
			read: dao.ProgramRecapRead{
				TrainingDays: 3,
				Sessions: append(append(append(
					recapSessions(done, recapFourWeeks[0:3]...),
					recapSessions(missed, recapFourWeeks[3:6]...)...),
					recapSessions(done, recapFourWeeks[6:9]...)...),
					recapSessions(missed, recapFourWeeks[9:12]...)...),
				TrainedDates: append(append([]string{}, recapFourWeeks[0:3]...), recapFourWeeks[6:9]...),
				WeeksOff:     []string{"2026-09-14", "2026-09-21"},
			},
			today: later,
			want: model.ProgramRecap{
				FirstDate: recapDate("2026-09-07"), LastDate: recapDate("2026-10-02"),
				Weeks: model.ProgramRecapWeeks{Total: 3, OnTarget: 2}, WorkoutsDone: 6,
				Lifts: []model.ProgramRecapLift{},
			},
		},
		{
			// Sport sessions and bodyweight work: weeks and workouts are
			// still counted, and lifts is an empty list.
			name: "no logged lifts",
			read: dao.ProgramRecapRead{
				TrainingDays: 2,
				Sessions:     recapSessions(done, "2026-09-08", "2026-09-10", "2026-09-15", "2026-09-17"),
				TrainedDates: []string{"2026-09-08", "2026-09-10", "2026-09-15", "2026-09-17"},
			},
			today: later,
			want: model.ProgramRecap{
				FirstDate: recapDate("2026-09-08"), LastDate: recapDate("2026-09-17"),
				Weeks: model.ProgramRecapWeeks{Total: 2, OnTarget: 2}, WorkoutsDone: 4,
				Lifts: []model.ProgramRecapLift{},
			},
		},
		{
			name:  "no sessions",
			read:  dao.ProgramRecapRead{TrainingDays: 3},
			today: later,
			want:  model.ProgramRecap{Lifts: []model.ProgramRecapLift{}},
		},
		{
			name:  "only sessions still to come",
			read:  dao.ProgramRecapRead{TrainingDays: 3, Sessions: recapSessions(planned, "2026-10-21", "2026-10-23")},
			today: later,
			want:  model.ProgramRecap{Lifts: []model.ProgramRecapLift{}},
		},
		{
			// Wednesday of week 2 of an active program. Week 2 is not over
			// and not on target yet, so it is left out.
			name: "unfinished week is left out",
			read: dao.ProgramRecapRead{
				Active: true, TrainingDays: 3,
				Sessions: append(
					recapSessions(done, "2026-09-07", "2026-09-09", "2026-09-11", "2026-09-14"),
					recapSessions(planned, "2026-09-16", "2026-09-18", "2026-09-21")...),
				TrainedDates: []string{"2026-09-07", "2026-09-09", "2026-09-11", "2026-09-14"},
			},
			today: "2026-09-16",
			want: model.ProgramRecap{
				Active:    true,
				FirstDate: recapDate("2026-09-07"), LastDate: recapDate("2026-09-16"),
				Weeks: model.ProgramRecapWeeks{Total: 1, OnTarget: 1}, WorkoutsDone: 4,
				Lifts: []model.ProgramRecapLift{},
			},
		},
		{
			// The same week once it reached the target on Friday.
			name: "unfinished week counts once on target",
			read: dao.ProgramRecapRead{
				Active: true, TrainingDays: 3,
				Sessions:     recapSessions(done, recapFourWeeks[0:6]...),
				TrainedDates: recapFourWeeks[0:6],
			},
			today: "2026-09-18",
			want: model.ProgramRecap{
				Active:    true,
				FirstDate: recapDate("2026-09-07"), LastDate: recapDate("2026-09-18"),
				Weeks: model.ProgramRecapWeeks{Total: 2, OnTarget: 2}, WorkoutsDone: 6,
				Lifts: []model.ProgramRecapLift{},
			},
		},
		{
			// Started on a Friday: the first week had one session, so it is
			// held to one day. Training on the Monday before the program
			// began is not credited to it, so week 1 still needs its Friday.
			name: "edge weeks are held to the days the program had",
			read: dao.ProgramRecapRead{
				TrainingDays: 3,
				Sessions: append(
					recapSessions(missed, "2026-09-11"),
					recapSessions(done, "2026-09-14", "2026-09-16", "2026-09-18")...),
				TrainedDates: []string{"2026-09-07", "2026-09-14", "2026-09-16", "2026-09-18"},
			},
			today: later,
			want: model.ProgramRecap{
				FirstDate: recapDate("2026-09-11"), LastDate: recapDate("2026-09-18"),
				Weeks: model.ProgramRecapWeeks{Total: 2, OnTarget: 1}, WorkoutsDone: 3,
				Lifts: []model.ProgramRecapLift{},
			},
		},
		{
			// A week with no session of this program (it was not the active
			// program then) is not one of its weeks. A program that was
			// never placed is held to the days it had sessions.
			name: "a gap and an unplaced program",
			read: dao.ProgramRecapRead{
				Sessions:     recapSessions(done, "2026-09-07", "2026-09-09", "2026-09-21"),
				TrainedDates: []string{"2026-09-07", "2026-09-15", "2026-09-21"},
			},
			today: later,
			want: model.ProgramRecap{
				FirstDate: recapDate("2026-09-07"), LastDate: recapDate("2026-09-21"),
				Weeks: model.ProgramRecapWeeks{Total: 2, OnTarget: 1}, WorkoutsDone: 3,
				Lifts: []model.ProgramRecapLift{},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := buildProgramRecap(&test.read, test.today)
			if !reflect.DeepEqual(*got, test.want) {
				t.Errorf("buildProgramRecap() = %+v\nwant %+v", *got, test.want)
			}
		})
	}
}

// The five lifts done on the most days are kept, ties go by name, and a
// lift's name is matched without regard to case.
func TestBuildProgramRecapMainLifts(t *testing.T) {
	day := func(name, date string, weight float64) dao.ProgramRecapLiftDay {
		return dao.ProgramRecapLiftDay{Name: name, Date: date, Weight: weight, Reps: 5}
	}
	read := &dao.ProgramRecapRead{LiftDays: []dao.ProgramRecapLiftDay{
		day("back squat", "2026-09-07", 185), day("Back Squat", "2026-09-14", 190), day("Back Squat", "2026-09-21", 195),
		day("Bench Press", "2026-09-07", 135), day("Bench Press", "2026-09-14", 140),
		day("Curl", "2026-09-07", 30),
		day("Deadlift", "2026-09-09", 225), day("Deadlift", "2026-09-16", 245),
		day("Lunge", "2026-09-09", 50),
		day("Overhead Press", "2026-09-09", 95),
		day("Row", "2026-09-11", 115), day("Row", "2026-09-18", 110),
	}}
	lifts := buildProgramRecap(read, "2026-10-20").Lifts

	var names []string
	for _, lift := range lifts {
		names = append(names, lift.Name)
	}
	if want := []string{"Back Squat", "Bench Press", "Deadlift", "Row", "Curl"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("lifts = %v, want %v", names, want)
	}
	squat := lifts[0]
	if squat.Days != 3 || squat.First.Weight != 185 || squat.Last.Weight != 195 || squat.First.Date != "2026-09-07" || squat.Last.Date != "2026-09-21" {
		t.Errorf("squat = %+v; want 3 days, 185 on 09-07 to 195 on 09-21", squat)
	}
	// A lift that went down is reported as it happened.
	if row := lifts[3]; row.First.Weight != 115 || row.Last.Weight != 110 {
		t.Errorf("row = %+v; want 115 then 110", row)
	}
	// A lift done on one day has the same first and last set.
	if curl := lifts[4]; curl.Days != 1 || curl.First != curl.Last {
		t.Errorf("curl = %+v; want one day with equal first and last", curl)
	}
}

// recapStore is an in-memory ProgramRecapDAO that counts its reads.
type recapStore struct {
	read  *dao.ProgramRecapRead
	err   error
	reads int
}

func (s *recapStore) Read(context.Context, uuid.UUID, uuid.UUID) (*dao.ProgramRecapRead, error) {
	s.reads++
	if s.err != nil {
		return nil, s.err
	}
	return s.read, nil
}

func TestProgramRecapRoundTrips(t *testing.T) {
	store := &recapStore{read: &dao.ProgramRecapRead{Name: "Push Pull Legs", Active: true, TrainingDays: 3}}
	svc := NewProgramRecapService(store)
	programID := uuid.New()

	recap, err := svc.Get(t.Context(), uuid.New(), programID)
	if err != nil {
		t.Fatal(err)
	}
	if store.reads != 1 {
		t.Errorf("reads = %d, want one database round trip", store.reads)
	}
	if recap.ProgramID != programID || recap.Name != "Push Pull Legs" || !recap.Active || recap.Lifts == nil {
		t.Errorf("recap = %+v; want the program's id, name, active flag, and a non-nil lifts list", recap)
	}
}

func TestProgramRecapNotFound(t *testing.T) {
	svc := NewProgramRecapService(&recapStore{err: &model.NotFoundError{Message: "program not found"}})
	_, err := svc.Get(t.Context(), uuid.New(), uuid.New())
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v, want a not-found error", err)
	}
}

// At 03:00 UTC on Tuesday it is still Monday evening in New York, so
// Tuesday's session is not part of the recap yet there.
func TestProgramRecapTodayFollowsTheSavedTimezone(t *testing.T) {
	sessions := recapSessions(model.WorkoutStatusPlanned, "2026-09-07", "2026-09-08")
	tests := []struct {
		timezone string
		wantLast string
	}{
		{timezone: "America/New_York", wantLast: "2026-09-07"},
		{timezone: "", wantLast: "2026-09-08"},
		{timezone: "Not/AZone", wantLast: "2026-09-08"},
	}
	for _, test := range tests {
		t.Run(test.timezone, func(t *testing.T) {
			svc := &programRecapService{
				recaps: &recapStore{read: &dao.ProgramRecapRead{Timezone: test.timezone, Sessions: sessions}},
				now:    func() time.Time { return time.Date(2026, 9, 8, 3, 0, 0, 0, time.UTC) },
			}
			recap, err := svc.Get(t.Context(), uuid.New(), uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			if recap.LastDate == nil || *recap.LastDate != test.wantLast {
				t.Errorf("last_date = %v, want %s", recap.LastDate, test.wantLast)
			}
		})
	}
}
