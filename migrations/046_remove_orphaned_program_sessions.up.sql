-- One-time repair for schedules created before migration 045.
--
-- Before contract 1.10.0 a program save replaced its workouts with new rows, and the sessions
-- already scheduled from the old rows kept their date while their program_workout_id became
-- NULL (ON DELETE SET NULL). The rolling fill treats every future session of the active program
-- as valid and continues after the latest one, so those orphans stayed on the calendar, on days
-- and with names the program no longer has.
--
-- Remove a session only when all of these hold: it belongs to a program, its workout is gone,
-- it is dated after the athlete's today, and nothing has touched it (planned, not finalized, no
-- started or finished workout). Past and started days are history and stay.
--
-- Then set the active program's watermark to the athlete's today, so the next schedule read
-- fills from tomorrow. Sessions the fill already created are kept: its insert skips a slot that
-- exists.

WITH athlete_today AS (
    SELECT p.id AS program_id,
           (now() AT TIME ZONE COALESCE(tp.timezone, 'UTC'))::date AS today
    FROM programs p
    LEFT JOIN training_profiles tp ON tp.user_id = p.user_id
),
removed AS (
    DELETE FROM scheduled_workouts sw
    USING athlete_today t
    WHERE sw.program_id = t.program_id
      AND sw.program_workout_id IS NULL
      AND sw.date > t.today
      AND sw.status = 'planned'
      AND sw.finalized_at IS NULL
      AND NOT EXISTS (
          SELECT 1 FROM workout_sessions ws
          WHERE ws.scheduled_workout_id = sw.id AND ws.status IN ('active', 'completed')
      )
    RETURNING sw.program_id
)
UPDATE programs p
SET scheduled_through = t.today
FROM athlete_today t
WHERE p.id = t.program_id
  AND p.active
  AND p.id IN (SELECT program_id FROM removed);
