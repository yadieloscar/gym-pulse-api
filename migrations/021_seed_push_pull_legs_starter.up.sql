-- Push/Pull/Legs starter. Three rotating workouts cover 3-6 available days because
-- schedule materialization cycles workouts across the athlete's chosen weekdays.
-- Versioned IDs are generated once per database; clients discover the starter
-- through the catalog rather than hard-coding UUIDs.
INSERT INTO starter_programs (
    slug, version, name, description, primary_goal, min_days, max_days,
    experience, equipment, duration_minutes, rationale, roadmap)
VALUES (
    'push-pull-legs', 1,
    'Push Pull Legs',
    'Three-day rotation of pressing, pulling, and lower-body sessions.',
    'hypertrophy', 3, 6,
    ARRAY['beginner','intermediate','advanced']::TEXT[],
    ARRAY['barbell','dumbbells','machines','full_gym']::TEXT[], 60,
    'Groups movements by pattern so each muscle group is trained and rested on a repeatable cycle.',
    '{"cadence":"push_pull_legs_rotation","weeks":8}'::JSONB)
ON CONFLICT (slug, version) DO NOTHING;

INSERT INTO starter_workouts (starter_program_id, name, weekday, sequence_position)
SELECT sp.id, workout.name, workout.weekday, workout.sequence_position
FROM starter_programs sp
CROSS JOIN (VALUES
    ('Push', 1, 1),
    ('Pull', 3, 2),
    ('Legs', 5, 3)
) AS workout(name, weekday, sequence_position)
WHERE sp.slug='push-pull-legs' AND sp.version=1
ON CONFLICT (starter_program_id, sequence_position) DO NOTHING;

INSERT INTO starter_exercises (
    starter_workout_id, catalog_id, name, category, modality, exercise_order,
    target_sets, target_reps, rest_seconds)
SELECT sw.id, ec.id, exercise.name, exercise.category, 'strength', exercise.exercise_order,
       exercise.target_sets, exercise.target_reps, exercise.rest_seconds
FROM starter_workouts sw
JOIN starter_programs sp ON sp.id=sw.starter_program_id
JOIN (VALUES
    -- Push
    (1, 'Barbell Bench Press',    'push', 1, 3,  8, 120),
    (1, 'Overhead Press',         'push', 2, 3,  8, 120),
    (1, 'Incline Dumbbell Press', 'push', 3, 3, 10,  90),
    (1, 'Lateral Raise',          'push', 4, 3, 12,  60),
    (1, 'Triceps Pushdown',       'push', 5, 3, 12,  60),
    -- Pull
    (2, 'Deadlift',               'pull', 1, 3,  5, 180),
    (2, 'Lat Pulldown',           'pull', 2, 3, 10,  90),
    (2, 'Barbell Row',            'pull', 3, 3,  8, 120),
    (2, 'Face Pull',              'pull', 4, 3, 15,  60),
    (2, 'Dumbbell Curl',          'pull', 5, 3, 12,  60),
    -- Legs
    (3, 'Back Squat',             'legs', 1, 3,  6, 180),
    (3, 'Romanian Deadlift',      'legs', 2, 3,  8, 120),
    (3, 'Leg Press',              'legs', 3, 3, 10,  90),
    (3, 'Lying Leg Curl',         'legs', 4, 3, 12,  60),
    (3, 'Standing Calf Raise',    'legs', 5, 3, 15,  60)
) AS exercise(sequence_position, name, category, exercise_order, target_sets, target_reps, rest_seconds)
  ON exercise.sequence_position=sw.sequence_position
LEFT JOIN exercise_catalog ec ON ec.name=exercise.name AND ec.category=exercise.category
WHERE sp.slug='push-pull-legs' AND sp.version=1
ON CONFLICT (starter_workout_id, exercise_order) DO NOTHING;
