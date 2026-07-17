-- Stable baseline starters. Their versioned IDs are generated once per database;
-- clients discover them through the catalog rather than hard-coding UUIDs.
WITH inserted AS (
    INSERT INTO starter_programs (
        slug, version, name, description, primary_goal, min_days, max_days,
        experience, equipment, duration_minutes, rationale, roadmap)
    SELECT
        goal || '-foundation', 1,
        initcap(replace(goal, '_', ' ')) || ' Foundation',
        'Adaptable full-body foundation with two weekly exposures.',
        goal, 1, 7,
        ARRAY['beginner','intermediate','advanced']::TEXT[],
        ARRAY[]::TEXT[], 45,
        'Builds repeatable full-body practice while remaining equipment-flexible.',
        '{"cadence":"two_full_body_exposures","weeks":8}'::JSONB
    FROM unnest(ARRAY[
        'general_health','strength','hypertrophy','conditioning','power','body_composition'
    ]) AS goal
    ON CONFLICT (slug, version) DO NOTHING
    RETURNING id
), workouts AS (
    INSERT INTO starter_workouts (starter_program_id, name, weekday, sequence_position)
    SELECT sp.id, 'Full Body A', 1, 1
    FROM starter_programs sp WHERE sp.version=1 AND sp.slug LIKE '%-foundation'
    UNION ALL
    SELECT sp.id, 'Full Body B', 4, 2
    FROM starter_programs sp WHERE sp.version=1 AND sp.slug LIKE '%-foundation'
    ON CONFLICT (starter_program_id, sequence_position) DO NOTHING
    RETURNING id
)
INSERT INTO starter_exercises (
    starter_workout_id, catalog_id, name, category, modality, exercise_order,
    target_sets, target_reps, rest_seconds)
SELECT sw.id, ec.id, exercise.name, exercise.category, 'strength', exercise.exercise_order,
       2, exercise.target_reps, 60
FROM starter_workouts sw
JOIN starter_programs sp ON sp.id=sw.starter_program_id
CROSS JOIN (VALUES
    ('Push-Up', 'push', 1, 10),
    ('Goblet Squat', 'legs', 2, 10),
    ('Plank', 'core', 3, 30)
) AS exercise(name, category, exercise_order, target_reps)
LEFT JOIN exercise_catalog ec ON ec.name=exercise.name AND ec.category=exercise.category
WHERE sp.version=1 AND sp.slug LIKE '%-foundation'
ON CONFLICT (starter_workout_id, exercise_order) DO NOTHING;
