-- A planned week off (spec 038, gympulse-planned-week-off).
--
-- Additive only. No row is rewritten and no athlete has a week off until they
-- take one. A row says "this Monday-to-Sunday week is off": the API then
-- finalizes that week's untouched planned workouts as skipped instead of
-- leaving a miss to resolve, and the app leaves the week out of the
-- weeks-on-target streak.
CREATE TABLE IF NOT EXISTS weeks_off (
    user_id     UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    week_start  DATE NOT NULL CHECK (EXTRACT(ISODOW FROM week_start) = 1),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, week_start)
);

SELECT gym_pulse_private.secure_table('public.weeks_off');
