-- Records how an athlete resolved a missed scheduled workout: skipped (undoable)
-- or recovered (a copy was planned through POST /schedule/recover). The
-- workout's outcome, participation, and history are never changed.
ALTER TABLE scheduled_workouts
    ADD COLUMN resolution  TEXT,
    ADD COLUMN resolved_at TIMESTAMPTZ,
    ADD CONSTRAINT scheduled_workouts_resolution_check
        CHECK (resolution IS NULL OR resolution IN ('skipped', 'recovered')),
    ADD CONSTRAINT scheduled_workouts_resolution_pair
        CHECK ((resolution IS NULL) = (resolved_at IS NULL));

-- Backfill recoveries made before this migration from their stored replay
-- records. Each record names the copy it created; the source is the missed
-- workout the old selection returned for that copy: same program workout and
-- sequence position, on or before the copy's date, already finalized when the
-- recovery ran, earliest first. Later cycles of the same program workout are
-- never matched because they were not finalized as missed at recovery time or
-- are dated after the copy. Recoveries whose copy was later deleted (for example
-- by regenerate) cannot be traced and stay unresolved; athletes can skip them.
WITH sources AS (
    SELECT DISTINCT ON (r.id) m.id AS source_id, r.created_at AS recovered_at
    FROM idempotency_records r
    JOIN scheduled_workouts c ON c.id = r.resource_id AND c.user_id = r.user_id
    JOIN scheduled_workouts m
      ON m.user_id = c.user_id
     AND m.id <> c.id
     AND m.status = 'missed'
     AND m.program_workout_id IS NOT DISTINCT FROM c.program_workout_id
     AND m.sequence_position IS NOT DISTINCT FROM c.sequence_position
     AND m.date <= c.date
     AND m.finalized_at <= r.created_at
    WHERE r.scope = 'schedule/recover'
    ORDER BY r.id, m.date
), firsts AS (
    SELECT source_id, min(recovered_at) AS resolved_at
    FROM sources
    GROUP BY source_id
)
UPDATE scheduled_workouts w
SET resolution = 'recovered',
    resolved_at = f.resolved_at,
    revision = w.revision + 1,
    updated_at = now()
FROM firsts f
WHERE w.id = f.source_id;
