-- Exercise notes (spec 040, gympulse-exercise-notes).
--
-- Additive only. No row is rewritten and no athlete has a note until they
-- save one. A row is the athlete's one short private note for an exercise,
-- such as "seat height 4". The exercise is named by exercise_key: its name
-- trimmed, with inner whitespace collapsed, lowercased. The API derives the
-- key, so the same exercise in a saved workout, a program, and a scheduled
-- workout shares one note. exercise_name keeps the name as last saved.
CREATE TABLE IF NOT EXISTS exercise_notes (
    user_id        UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    exercise_key   TEXT NOT NULL CHECK (char_length(exercise_key) BETWEEN 1 AND 120),
    exercise_name  TEXT NOT NULL CHECK (char_length(exercise_name) BETWEEN 1 AND 120),
    note           TEXT NOT NULL CHECK (char_length(note) BETWEEN 1 AND 200),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, exercise_key)
);

SELECT gym_pulse_private.secure_table('public.exercise_notes');
