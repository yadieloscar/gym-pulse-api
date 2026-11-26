package service

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ProgramRecapService sums up one program from the sessions, participation,
// weeks off, and sets already stored for it. See docs/CONTRACTS.md "Program
// recap".
type ProgramRecapService interface {
	// Get returns the recap of a program the athlete owns, active or not.
	// A missing or foreign program is a *model.NotFoundError.
	Get(ctx context.Context, userID, programID uuid.UUID) (*model.ProgramRecap, error)
}

type programRecapService struct {
	recaps dao.ProgramRecapDAO
	now    func() time.Time
}

// NewProgramRecapService returns a ProgramRecapService.
func NewProgramRecapService(recaps dao.ProgramRecapDAO) ProgramRecapService {
	return &programRecapService{recaps: recaps, now: time.Now}
}

func (s *programRecapService) Get(ctx context.Context, userID, programID uuid.UUID) (*model.ProgramRecap, error) {
	read, err := s.recaps.Read(ctx, userID, programID)
	if err != nil {
		return nil, err
	}
	// A saved timezone the server cannot load falls back to UTC: a recap is
	// a summary, and a day's difference at its edge is better than an error.
	location := time.UTC
	if read.Timezone != "" {
		if loaded, err := time.LoadLocation(read.Timezone); err == nil {
			location = loaded
		}
	}
	recap := buildProgramRecap(read, s.now().In(location).Format(time.DateOnly))
	recap.ProgramID = programID
	return recap, nil
}

// buildProgramRecap reduces the stored facts to a recap as of today
// (YYYY-MM-DD). Dates compare as strings because they share that form.
func buildProgramRecap(read *dao.ProgramRecapRead, today string) *model.ProgramRecap {
	recap := &model.ProgramRecap{Name: read.Name, Active: read.Active, Lifts: recapLifts(read.LiftDays)}
	if len(read.Sessions) == 0 {
		return recap
	}

	// The days the program had a session, per week, and its whole span.
	// Sessions arrive oldest first.
	sessionDays := map[string]map[string]bool{}
	for _, session := range read.Sessions {
		week := recapWeekStart(session.Date)
		if sessionDays[week] == nil {
			sessionDays[week] = map[string]bool{}
		}
		sessionDays[week][session.Date] = true
		if session.Date > today {
			continue
		}
		if recap.FirstDate == nil {
			recap.FirstDate = &session.Date
		}
		recap.LastDate = &session.Date
		if session.Status == model.WorkoutStatusCompleted || session.Status == model.WorkoutStatusIncomplete {
			recap.WorkoutsDone++
		}
	}
	if recap.FirstDate == nil {
		return recap
	}

	spanFirst, spanLast := read.Sessions[0].Date, read.Sessions[len(read.Sessions)-1].Date
	trainedDays := map[string]int{}
	for _, date := range read.TrainedDates {
		if date >= spanFirst && date <= spanLast {
			trainedDays[recapWeekStart(date)]++
		}
	}

	firstWeek, err := model.ParseDate(recapWeekStart(*recap.FirstDate))
	if err != nil {
		return recap
	}
	lastWeek := recapWeekStart(*recap.LastDate)
	for monday := firstWeek; ; monday = monday.AddDate(0, 0, 7) {
		week := monday.Format(time.DateOnly)
		if week > lastWeek {
			break
		}
		target := len(sessionDays[week])
		if target == 0 {
			continue
		}
		if read.TrainingDays > 0 {
			target = min(target, read.TrainingDays)
		}
		onTarget := trainedDays[week] >= target
		// A week off that fell short and a week that is not over yet are
		// left out instead of counting against the athlete.
		unfinished := monday.AddDate(0, 0, 6).Format(time.DateOnly) > today
		if !onTarget && (unfinished || slices.Contains(read.WeeksOff, week)) {
			continue
		}
		recap.Weeks.Total++
		if onTarget {
			recap.Weeks.OnTarget++
		}
	}
	return recap
}

// recapLifts keeps the lifts done on the most days, each with its heaviest
// set on the first and on the last day. days holds one row per lift and day,
// ordered by lift then date. The result is never nil.
func recapLifts(days []dao.ProgramRecapLiftDay) []model.ProgramRecapLift {
	lifts := []model.ProgramRecapLift{}
	var key string
	for _, day := range days {
		set := model.ProgramRecapSet{Weight: day.Weight, Reps: day.Reps, Date: day.Date}
		if dayKey := strings.ToLower(day.Name); len(lifts) == 0 || dayKey != key {
			key = dayKey
			lifts = append(lifts, model.ProgramRecapLift{Name: day.Name, First: set})
		}
		lift := &lifts[len(lifts)-1]
		// The latest spelling of the name is the one shown.
		lift.Name, lift.Last = day.Name, set
		lift.Days++
	}
	slices.SortStableFunc(lifts, func(a, b model.ProgramRecapLift) int {
		return cmp.Or(cmp.Compare(b.Days, a.Days), cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)))
	})
	if len(lifts) > model.MaxProgramRecapLifts {
		lifts = lifts[:model.MaxProgramRecapLifts]
	}
	return lifts
}

// recapWeekStart returns the Monday of date's week. date is a stored
// YYYY-MM-DD value; one that does not parse is returned unchanged.
func recapWeekStart(date string) string {
	day, err := model.ParseDate(date)
	if err != nil {
		return date
	}
	return mondayOf(day).Format(time.DateOnly)
}
