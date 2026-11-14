-- Programming metadata for the rule-based program engine (spec 016).
-- Adds pattern, muscle group, required equipment, and impact to every catalog
-- entry, seeds all current rows by (category, name), then constrains the
-- columns so a future seed cannot omit them.
ALTER TABLE exercise_catalog
    ADD COLUMN pattern            TEXT,
    ADD COLUMN muscle_group       TEXT,
    ADD COLUMN equipment_required TEXT[],
    ADD COLUMN impact             TEXT;

UPDATE exercise_catalog AS c
SET pattern = s.pattern,
    muscle_group = s.muscle_group,
    equipment_required = s.equipment_required,
    impact = s.impact
FROM (VALUES
    ('push', 'Barbell Bench Press', 'horizontal_push', 'chest', ARRAY['barbell']::text[], 'low'),
    ('push', 'Incline Barbell Bench Press', 'horizontal_push', 'chest', ARRAY['barbell']::text[], 'low'),
    ('push', 'Dumbbell Bench Press', 'horizontal_push', 'chest', ARRAY['dumbbells']::text[], 'low'),
    ('push', 'Incline Dumbbell Press', 'horizontal_push', 'chest', ARRAY['dumbbells']::text[], 'low'),
    ('push', 'Overhead Press', 'vertical_push', 'shoulders', ARRAY['barbell']::text[], 'low'),
    ('push', 'Seated Dumbbell Shoulder Press', 'vertical_push', 'shoulders', ARRAY['dumbbells']::text[], 'low'),
    ('push', 'Close-Grip Bench Press', 'horizontal_push', 'arms', ARRAY['barbell']::text[], 'low'),
    ('push', 'Dips', 'vertical_push', 'chest', ARRAY['full_gym']::text[], 'low'),
    ('push', 'Push-Up', 'horizontal_push', 'chest', ARRAY['bodyweight']::text[], 'low'),
    ('push', 'Cable Fly', 'isolation', 'chest', ARRAY['machines']::text[], 'low'),
    ('push', 'Dumbbell Fly', 'isolation', 'chest', ARRAY['dumbbells']::text[], 'low'),
    ('push', 'Lateral Raise', 'isolation', 'shoulders', ARRAY['dumbbells']::text[], 'low'),
    ('push', 'Triceps Pushdown', 'isolation', 'arms', ARRAY['machines']::text[], 'low'),
    ('push', 'Overhead Triceps Extension', 'isolation', 'arms', ARRAY['dumbbells']::text[], 'low'),
    ('pull', 'Deadlift', 'hinge', 'hamstrings', ARRAY['barbell']::text[], 'low'),
    ('pull', 'Pull-Up', 'vertical_pull', 'back', ARRAY['full_gym']::text[], 'low'),
    ('pull', 'Chin-Up', 'vertical_pull', 'back', ARRAY['full_gym']::text[], 'low'),
    ('pull', 'Lat Pulldown', 'vertical_pull', 'back', ARRAY['machines']::text[], 'low'),
    ('pull', 'Barbell Row', 'horizontal_pull', 'back', ARRAY['barbell']::text[], 'low'),
    ('pull', 'Dumbbell Row', 'horizontal_pull', 'back', ARRAY['dumbbells']::text[], 'low'),
    ('pull', 'Seated Cable Row', 'horizontal_pull', 'back', ARRAY['machines']::text[], 'low'),
    ('pull', 'T-Bar Row', 'horizontal_pull', 'back', ARRAY['full_gym']::text[], 'low'),
    ('pull', 'Face Pull', 'isolation', 'shoulders', ARRAY['machines']::text[], 'low'),
    ('pull', 'Rear Delt Fly', 'isolation', 'shoulders', ARRAY['dumbbells']::text[], 'low'),
    ('pull', 'Barbell Curl', 'isolation', 'arms', ARRAY['barbell']::text[], 'low'),
    ('pull', 'Dumbbell Curl', 'isolation', 'arms', ARRAY['dumbbells']::text[], 'low'),
    ('pull', 'Hammer Curl', 'isolation', 'arms', ARRAY['dumbbells']::text[], 'low'),
    ('pull', 'Preacher Curl', 'isolation', 'arms', ARRAY['machines']::text[], 'low'),
    ('legs', 'Back Squat', 'squat', 'quads', ARRAY['barbell']::text[], 'low'),
    ('legs', 'Front Squat', 'squat', 'quads', ARRAY['barbell']::text[], 'low'),
    ('legs', 'Goblet Squat', 'squat', 'quads', ARRAY['dumbbells']::text[], 'low'),
    ('legs', 'Romanian Deadlift', 'hinge', 'hamstrings', ARRAY['barbell']::text[], 'low'),
    ('legs', 'Leg Press', 'squat', 'quads', ARRAY['machines']::text[], 'low'),
    ('legs', 'Walking Lunge', 'lunge', 'quads', ARRAY['bodyweight']::text[], 'low'),
    ('legs', 'Bulgarian Split Squat', 'lunge', 'quads', ARRAY['bodyweight']::text[], 'low'),
    ('legs', 'Hip Thrust', 'hinge', 'glutes', ARRAY['barbell']::text[], 'low'),
    ('legs', 'Step-Up', 'lunge', 'quads', ARRAY['bodyweight']::text[], 'low'),
    ('legs', 'Leg Extension', 'isolation', 'quads', ARRAY['machines']::text[], 'low'),
    ('legs', 'Lying Leg Curl', 'isolation', 'hamstrings', ARRAY['machines']::text[], 'low'),
    ('legs', 'Standing Calf Raise', 'isolation', 'calves', ARRAY['bodyweight']::text[], 'low'),
    ('legs', 'Seated Calf Raise', 'isolation', 'calves', ARRAY['machines']::text[], 'low'),
    ('legs', 'Hip Abduction', 'isolation', 'glutes', ARRAY['machines']::text[], 'low'),
    ('lower', 'Sumo Deadlift', 'hinge', 'hamstrings', ARRAY['barbell']::text[], 'low'),
    ('lower', 'Hack Squat', 'squat', 'quads', ARRAY['machines']::text[], 'low'),
    ('lower', 'Good Morning', 'hinge', 'hamstrings', ARRAY['barbell']::text[], 'low'),
    ('lower', 'Single-Leg Press', 'squat', 'quads', ARRAY['machines']::text[], 'low'),
    ('lower', 'Nordic Hamstring Curl', 'isolation', 'hamstrings', ARRAY['bodyweight']::text[], 'low'),
    ('upper', 'Arnold Press', 'vertical_push', 'shoulders', ARRAY['dumbbells']::text[], 'low'),
    ('upper', 'Machine Chest Press', 'horizontal_push', 'chest', ARRAY['machines']::text[], 'low'),
    ('upper', 'Machine Row', 'horizontal_pull', 'back', ARRAY['machines']::text[], 'low'),
    ('upper', 'Landmine Press', 'vertical_push', 'shoulders', ARRAY['barbell']::text[], 'low'),
    ('upper', 'Cable Crossover', 'isolation', 'chest', ARRAY['machines']::text[], 'low'),
    ('core', 'Plank', 'core', 'core', ARRAY['bodyweight']::text[], 'low'),
    ('core', 'Side Plank', 'core', 'core', ARRAY['bodyweight']::text[], 'low'),
    ('core', 'Hanging Leg Raise', 'core', 'core', ARRAY['full_gym']::text[], 'low'),
    ('core', 'Ab Wheel Rollout', 'core', 'core', ARRAY['full_gym']::text[], 'low'),
    ('core', 'Cable Crunch', 'core', 'core', ARRAY['machines']::text[], 'low'),
    ('core', 'Russian Twist', 'core', 'core', ARRAY['bodyweight']::text[], 'low'),
    ('core', 'Dead Bug', 'core', 'core', ARRAY['bodyweight']::text[], 'low'),
    ('core', 'Bicycle Crunch', 'core', 'core', ARRAY['bodyweight']::text[], 'low'),
    ('full', 'Clean and Press', 'explosive', 'full_body', ARRAY['barbell']::text[], 'high'),
    ('full', 'Power Clean', 'explosive', 'full_body', ARRAY['barbell']::text[], 'high'),
    ('full', 'Kettlebell Swing', 'explosive', 'glutes', ARRAY['dumbbells']::text[], 'low'),
    ('full', 'Thruster', 'explosive', 'full_body', ARRAY['barbell']::text[], 'low'),
    ('full', 'Burpee', 'locomotion', 'full_body', ARRAY['bodyweight']::text[], 'high'),
    ('full', 'Farmer''s Carry', 'carry', 'full_body', ARRAY['dumbbells']::text[], 'low'),
    ('full', 'Turkish Get-Up', 'carry', 'full_body', ARRAY['dumbbells']::text[], 'low'),
    ('cardio', 'Treadmill Run', 'locomotion', 'full_body', ARRAY['cardio_machine']::text[], 'high'),
    ('cardio', 'Incline Treadmill Walk', 'locomotion', 'full_body', ARRAY['cardio_machine']::text[], 'low'),
    ('cardio', 'Outdoor Run', 'locomotion', 'full_body', ARRAY['bodyweight']::text[], 'high'),
    ('cardio', 'Stationary Bike', 'locomotion', 'full_body', ARRAY['cardio_machine']::text[], 'low'),
    ('cardio', 'Assault Bike', 'locomotion', 'full_body', ARRAY['cardio_machine']::text[], 'low'),
    ('cardio', 'Rowing Machine', 'locomotion', 'full_body', ARRAY['cardio_machine']::text[], 'low'),
    ('cardio', 'Elliptical', 'locomotion', 'full_body', ARRAY['cardio_machine']::text[], 'low'),
    ('cardio', 'Stairmaster', 'locomotion', 'full_body', ARRAY['cardio_machine']::text[], 'low'),
    ('cardio', 'Jump Rope', 'locomotion', 'full_body', ARRAY['bodyweight']::text[], 'high'),
    ('cardio', 'Swimming', 'locomotion', 'full_body', ARRAY['full_gym']::text[], 'low'),
    ('cardio', 'HIIT Circuit', 'locomotion', 'full_body', ARRAY['bodyweight']::text[], 'high'),
    ('cardio', 'Sled Push', 'locomotion', 'full_body', ARRAY['full_gym']::text[], 'low')
) AS s(category, name, pattern, muscle_group, equipment_required, impact)
WHERE c.category = s.category AND c.name = s.name;

DO $$
DECLARE missing INTEGER;
BEGIN
    SELECT count(*) INTO missing FROM exercise_catalog WHERE pattern IS NULL;
    IF missing > 0 THEN
        RAISE EXCEPTION 'exercise_catalog has % rows without programming metadata', missing;
    END IF;
END $$;

ALTER TABLE exercise_catalog
    ALTER COLUMN pattern SET NOT NULL,
    ALTER COLUMN muscle_group SET NOT NULL,
    ALTER COLUMN equipment_required SET NOT NULL,
    ALTER COLUMN impact SET NOT NULL,
    ADD CONSTRAINT exercise_catalog_pattern_check CHECK (pattern IN (
        'squat', 'hinge', 'lunge', 'horizontal_push', 'vertical_push', 'horizontal_pull',
        'vertical_pull', 'carry', 'core', 'explosive', 'locomotion', 'isolation')),
    ADD CONSTRAINT exercise_catalog_muscle_group_check CHECK (muscle_group IN (
        'chest', 'back', 'shoulders', 'arms', 'quads', 'hamstrings', 'glutes', 'calves', 'core', 'full_body')),
    ADD CONSTRAINT exercise_catalog_equipment_required_check CHECK (
        cardinality(equipment_required) >= 1
        AND equipment_required <@ ARRAY['bodyweight', 'bands', 'dumbbells', 'barbell', 'machines', 'cardio_machine', 'full_gym']::text[]),
    ADD CONSTRAINT exercise_catalog_impact_check CHECK (impact IN ('low', 'high'));

CREATE INDEX idx_exercise_catalog_pattern ON exercise_catalog(pattern);
