//go:build integration

package dao

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// exerciseChangeWorkout is today's Push with two more exercises behind the
// fixture's own: "Row" (two sets) and "Curl" (one set).
func exerciseChangeWorkout(t *testing.T, f *releaseFixture) *model.ScheduledWorkout {
	t.Helper()
	program := f.startOwnWeek(t)
	push := f.sessionOn(t, program.ID, fillToday, "Push")
	if len(push.RequiredSets) != 2 || push.RequiredSets[1].ExerciseOrder != 1 {
		t.Fatalf("fixture Push = %+v, want one exercise of two sets", push.RequiredSets)
	}
	f.exec(t, `INSERT INTO scheduled_sets (scheduled_workout_id, exercise_name, exercise_category, exercise_modality,
		exercise_order, set_index, target_reps, target_weight)
		VALUES ($1,'Row','pull','strength',2,1,10,80), ($1,'Row','pull','strength',2,2,10,80),
		       ($1,'Curl','pull','strength',3,1,12,25)`, push.ID)
	return f.load(t, push.ID)
}

// logSet records a performed result on one required set, as the player does.
func logSet(t *testing.T, f *releaseFixture, workout *model.ScheduledWorkout, set model.ScheduledSet) {
	t.Helper()
	var sessionID uuid.UUID
	err := f.pool.QueryRow(f.ctx, `SELECT id FROM workout_sessions WHERE scheduled_workout_id=$1`, workout.ID).Scan(&sessionID)
	if err != nil {
		if err := f.pool.QueryRow(f.ctx, `INSERT INTO workout_sessions (user_id,date,name,status,scheduled_workout_id) VALUES ($1,$2,$3,'active',$4) RETURNING id`, f.userID, workout.Date, workout.Name, workout.ID).Scan(&sessionID); err != nil {
			t.Fatal(err)
		}
	}
	f.exec(t, `INSERT INTO set_logs (workout_session_id, scheduled_set_id, is_extra, exercise_name, exercise_category,
		exercise_modality, set_index, actual_reps, actual_weight, completed)
		VALUES ($1,$2,false,$3,$4,'strength',$5,8,100,true)`, sessionID, set.ID, set.ExerciseName, set.ExerciseCategory, set.SetIndex)
	f.exec(t, `UPDATE scheduled_workouts SET status='in_progress' WHERE id=$1`, workout.ID)
}

// layout prints a workout's sets as "order.index name" for comparison.
func layout(workout *model.ScheduledWorkout) []string {
	var out []string
	for _, set := range workout.RequiredSets {
		out = append(out, fmt.Sprintf("%d.%d %s", set.ExerciseOrder, set.SetIndex, set.ExerciseName))
	}
	return out
}

func swapTo(name string, revision int64) model.SwapScheduledExerciseRequest {
	return model.SwapScheduledExerciseRequest{OperationKey: uuid.NewString(), ExpectedRevision: revision, ExerciseName: name, ExerciseCategory: "push"}
}

func TestSwappingAnUnstartedExerciseRenamesItsSetsInPlace(t *testing.T) {
	f := newReleaseFixture(t)
	before := exerciseChangeWorkout(t, f)

	after, replayed, err := f.mutations.SwapScheduledExercise(f.ctx, f.userID, before.ID, 2, swapTo("Lat pulldown", before.Revision), releaseRecord("scheduled-workouts/exercise-swap"))
	if err != nil || replayed {
		t.Fatalf("swap: replayed=%v err=%v", replayed, err)
	}
	lift := before.RequiredSets[0].ExerciseName
	want := []string{"1.1 " + lift, "1.2 " + lift, "2.1 Lat pulldown", "2.2 Lat pulldown", "3.1 Curl"}
	if got := layout(after); !slices.Equal(got, want) {
		t.Fatalf("sets after the swap = %v, want %v", got, want)
	}
	swapped := after.RequiredSets[2]
	if swapped.ID != before.RequiredSets[2].ID || swapped.ExerciseCategory != "push" || swapped.TargetWeight != nil ||
		swapped.TargetReps == nil || *swapped.TargetReps != 10 || swapped.ProgramExerciseID != nil {
		t.Fatalf("the swapped set = %+v", swapped)
	}
	if after.Revision != before.Revision+1 || after.Status != before.Status {
		t.Fatalf("workout after the swap: revision=%d status=%s", after.Revision, after.Status)
	}
	// The program is untouched: next week's Push still has the original plan.
	if n := f.count(t, `SELECT count(*) FROM scheduled_sets ss JOIN scheduled_workouts sw ON sw.id=ss.scheduled_workout_id
		WHERE sw.user_id=$1 AND ss.exercise_name='Lat pulldown' AND sw.id<>$2`, f.userID, before.ID); n != 0 {
		t.Fatalf("the swap reached %d sets on other dates", n)
	}
}

func TestSwappingAStartedExerciseKeepsLoggedSetsAndMovesTheRest(t *testing.T) {
	f := newReleaseFixture(t)
	before := exerciseChangeWorkout(t, f)
	logged := before.RequiredSets[0]
	logSet(t, f, before, logged)

	after, _, err := f.mutations.SwapScheduledExercise(f.ctx, f.userID, before.ID, 1, swapTo("Push-up", before.Revision), releaseRecord("scheduled-workouts/exercise-swap"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.1 " + logged.ExerciseName, "2.1 Push-up", "3.1 Row", "3.2 Row", "4.1 Curl"}
	if got := layout(after); !slices.Equal(got, want) {
		t.Fatalf("sets after the swap = %v, want %v", got, want)
	}
	if kept := after.RequiredSets[0]; kept.ID != logged.ID || !kept.Checked || kept.ActualReps == nil || *kept.ActualReps != 8 {
		t.Fatalf("the logged set changed: %+v", kept)
	}
	if moved := after.RequiredSets[1]; moved.ID != before.RequiredSets[1].ID || moved.Checked {
		t.Fatalf("the open set = %+v", moved)
	}
}

func TestSkippingAnExerciseRemovesOnlyItsOpenSets(t *testing.T) {
	f := newReleaseFixture(t)
	before := exerciseChangeWorkout(t, f)
	lift := before.RequiredSets[0].ExerciseName

	// Nothing logged on Row: it leaves, and Curl moves up.
	after, _, err := f.mutations.SkipScheduledExercise(f.ctx, f.userID, before.ID, 2, before.Revision, releaseRecord("scheduled-workouts/exercise-skip"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := layout(after), []string{"1.1 " + lift, "1.2 " + lift, "2.1 Curl"}; !slices.Equal(got, want) {
		t.Fatalf("sets after skipping Row = %v, want %v", got, want)
	}

	// One set logged on the lift: that set stays, the open one leaves.
	logged := after.RequiredSets[0]
	logSet(t, f, after, logged)
	after, _, err = f.mutations.SkipScheduledExercise(f.ctx, f.userID, before.ID, 1, after.Revision, releaseRecord("scheduled-workouts/exercise-skip"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := layout(after), []string{"1.1 " + lift, "2.1 Curl"}; !slices.Equal(got, want) {
		t.Fatalf("sets after skipping the lift = %v, want %v", got, want)
	}
	if !after.RequiredSets[0].Checked || after.RequiredSets[0].ID != logged.ID {
		t.Fatalf("the logged set changed: %+v", after.RequiredSets[0])
	}

	// Every set of the lift is logged now: nothing is left to skip or swap.
	var validation *model.ValidationError
	if _, _, err := f.mutations.SkipScheduledExercise(f.ctx, f.userID, before.ID, 1, after.Revision, releaseRecord("scheduled-workouts/exercise-skip")); !errors.As(err, &validation) {
		t.Fatalf("skipping a fully logged exercise: %v", err)
	}
	if _, _, err := f.mutations.SwapScheduledExercise(f.ctx, f.userID, before.ID, 1, swapTo("Push-up", after.Revision), releaseRecord("scheduled-workouts/exercise-swap")); !errors.As(err, &validation) {
		t.Fatalf("swapping a fully logged exercise: %v", err)
	}
	if got := f.load(t, before.ID); got.Revision != after.Revision {
		t.Fatalf("a refused change advanced the revision to %d", got.Revision)
	}
}

func TestExerciseChangesRefuseWhatTheyCannotApply(t *testing.T) {
	f := newReleaseFixture(t)
	before := exerciseChangeWorkout(t, f)

	var notFound *model.NotFoundError
	if _, _, err := f.mutations.SkipScheduledExercise(f.ctx, f.userID, before.ID, 9, before.Revision, releaseRecord("scheduled-workouts/exercise-skip")); !errors.As(err, &notFound) {
		t.Fatalf("an unknown exercise order: %v", err)
	}
	if _, _, err := f.mutations.SkipScheduledExercise(f.ctx, uuid.New(), before.ID, 1, before.Revision, releaseRecord("scheduled-workouts/exercise-skip")); !errors.As(err, &notFound) {
		t.Fatalf("another athlete's workout: %v", err)
	}
	var conflict *model.ConflictError
	if _, _, err := f.mutations.SwapScheduledExercise(f.ctx, f.userID, before.ID, 1, swapTo("Push-up", before.Revision+5), releaseRecord("scheduled-workouts/exercise-swap")); !errors.As(err, &conflict) {
		t.Fatalf("a stale revision: %v", err)
	}

	// A workout keeps at least one set.
	current := before
	for _, order := range []int{3, 2} {
		next, _, err := f.mutations.SkipScheduledExercise(f.ctx, f.userID, before.ID, order, current.Revision, releaseRecord("scheduled-workouts/exercise-skip"))
		if err != nil {
			t.Fatal(err)
		}
		current = next
	}
	var validation *model.ValidationError
	if _, _, err := f.mutations.SkipScheduledExercise(f.ctx, f.userID, before.ID, 1, current.Revision, releaseRecord("scheduled-workouts/exercise-skip")); !errors.As(err, &validation) {
		t.Fatalf("skipping the last exercise: %v", err)
	}

	// A replay returns the recorded response and changes nothing more.
	record := releaseRecord("scheduled-workouts/exercise-swap")
	request := swapTo("Push-up", current.Revision)
	first, _, err := f.mutations.SwapScheduledExercise(f.ctx, f.userID, before.ID, 1, request, record)
	if err != nil {
		t.Fatal(err)
	}
	again, replayed, err := f.mutations.SwapScheduledExercise(f.ctx, f.userID, before.ID, 1, request, record)
	if err != nil || !replayed || again.Revision != first.Revision {
		t.Fatalf("replay: replayed=%v err=%v revision=%d want %d", replayed, err, again.Revision, first.Revision)
	}

	// A finished workout does not change.
	f.exec(t, `UPDATE scheduled_workouts SET finalized_at=now(), status='completed' WHERE id=$1`, before.ID)
	if _, _, err := f.mutations.SwapScheduledExercise(f.ctx, f.userID, before.ID, 1, swapTo("Dip", first.Revision), releaseRecord("scheduled-workouts/exercise-swap")); !errors.As(err, &conflict) {
		t.Fatalf("swapping in a finished workout: %v", err)
	}
}
