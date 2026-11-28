-- Spec 016 catalog amendment (2026-11-28): 34 exercises a large club floor
-- has and the catalog lacked, mostly selectorized, plate-loaded, Smith, and
-- cable work. Seed data only; no schema change.
--
-- sort_order continues after each category's current last entry. The program
-- engine orders candidates by sort_order, so the new entries sit behind every
-- existing one and a first draft (variant 0) keeps its opening picks.
--
-- Hip Adduction is filed under 'glutes' because the muscle group list has no
-- adductor value; it carries no sub_pattern, so only the general lower
-- isolation slot can choose it.
INSERT INTO exercise_catalog (name, category, modality, mechanic, sort_order, pattern, muscle_group, equipment_required, impact, sub_pattern) VALUES
    -- push
    ('Machine Shoulder Press',     'push',   'strength', 'compound',  16, 'vertical_push',   'shoulders',  ARRAY['machines']::text[],       'low', NULL),
    ('Incline Machine Press',      'push',   'strength', 'compound',  17, 'horizontal_push', 'chest',      ARRAY['machines']::text[],       'low', 'incline_push'),
    ('Smith Machine Bench Press',  'push',   'strength', 'compound',  18, 'horizontal_push', 'chest',      ARRAY['machines']::text[],       'low', NULL),
    ('Assisted Dip',               'push',   'strength', 'compound',  19, 'vertical_push',   'chest',      ARRAY['machines']::text[],       'low', NULL),
    ('Machine Dip',                'push',   'strength', 'compound',  20, 'vertical_push',   'chest',      ARRAY['machines']::text[],       'low', NULL),
    ('Pec Deck',                   'push',   'strength', 'isolation', 21, 'isolation',       'chest',      ARRAY['machines']::text[],       'low', 'chest_isolation'),
    ('Machine Lateral Raise',      'push',   'strength', 'isolation', 22, 'isolation',       'shoulders',  ARRAY['machines']::text[],       'low', 'shoulder_isolation'),
    ('Cable Lateral Raise',        'push',   'strength', 'isolation', 23, 'isolation',       'shoulders',  ARRAY['machines']::text[],       'low', 'shoulder_isolation'),
    ('Rope Triceps Extension',     'push',   'strength', 'isolation', 24, 'isolation',       'arms',       ARRAY['machines']::text[],       'low', 'triceps'),
    ('Skull Crusher',              'push',   'strength', 'isolation', 25, 'isolation',       'arms',       ARRAY['barbell']::text[],        'low', 'triceps'),
    -- pull
    ('Assisted Pull-Up',           'pull',   'strength', 'compound',  15, 'vertical_pull',   'back',       ARRAY['machines']::text[],       'low', NULL),
    ('Machine High Row',           'pull',   'strength', 'compound',  16, 'vertical_pull',   'back',       ARRAY['machines']::text[],       'low', NULL),
    ('Chest-Supported Row',        'pull',   'strength', 'compound',  17, 'horizontal_pull', 'back',       ARRAY['machines']::text[],       'low', NULL),
    ('Single-Arm Cable Row',       'pull',   'strength', 'compound',  18, 'horizontal_pull', 'back',       ARRAY['machines']::text[],       'low', NULL),
    ('Straight-Arm Pulldown',      'pull',   'strength', 'isolation', 19, 'isolation',       'back',       ARRAY['machines']::text[],       'low', NULL),
    ('Reverse Pec Deck',           'pull',   'strength', 'isolation', 20, 'isolation',       'shoulders',  ARRAY['machines']::text[],       'low', 'rear_delt'),
    ('Dumbbell Shrug',             'pull',   'strength', 'isolation', 21, 'isolation',       'back',       ARRAY['dumbbells']::text[],      'low', NULL),
    ('Cable Curl',                 'pull',   'strength', 'isolation', 22, 'isolation',       'arms',       ARRAY['machines']::text[],       'low', 'biceps'),
    ('Machine Preacher Curl',      'pull',   'strength', 'isolation', 23, 'isolation',       'arms',       ARRAY['machines']::text[],       'low', 'biceps'),
    -- legs
    ('Smith Machine Squat',        'legs',   'strength', 'compound',  15, 'squat',           'quads',      ARRAY['machines']::text[],       'low', NULL),
    ('Pendulum Squat',             'legs',   'strength', 'compound',  16, 'squat',           'quads',      ARRAY['machines']::text[],       'low', NULL),
    ('Belt Squat',                 'legs',   'strength', 'compound',  17, 'squat',           'quads',      ARRAY['machines']::text[],       'low', NULL),
    ('Machine Hip Thrust',         'legs',   'strength', 'compound',  18, 'hinge',           'glutes',     ARRAY['machines']::text[],       'low', NULL),
    ('Seated Leg Curl',            'legs',   'strength', 'isolation', 19, 'isolation',       'hamstrings', ARRAY['machines']::text[],       'low', 'hamstring_isolation'),
    ('Glute-Ham Raise',            'legs',   'strength', 'isolation', 20, 'isolation',       'hamstrings', ARRAY['full_gym']::text[],       'low', 'hamstring_isolation'),
    ('Back Extension',             'legs',   'strength', 'isolation', 21, 'isolation',       'hamstrings', ARRAY['full_gym']::text[],       'low', NULL),
    ('Cable Kickback',             'legs',   'strength', 'isolation', 22, 'isolation',       'glutes',     ARRAY['machines']::text[],       'low', 'glute_isolation'),
    ('Hip Adduction',              'legs',   'strength', 'isolation', 23, 'isolation',       'glutes',     ARRAY['machines']::text[],       'low', NULL),
    ('Leg Press Calf Raise',       'legs',   'strength', 'isolation', 24, 'isolation',       'calves',     ARRAY['machines']::text[],       'low', 'calf'),
    -- core
    ('Captain''s Chair Leg Raise', 'core',   'strength', 'compound',  9,  'core',            'core',       ARRAY['full_gym']::text[],       'low', NULL),
    ('Ab Crunch Machine',          'core',   'strength', 'isolation', 10, 'core',            'core',       ARRAY['machines']::text[],       'low', NULL),
    ('Rotary Torso',               'core',   'strength', 'isolation', 11, 'core',            'core',       ARRAY['machines']::text[],       'low', NULL),
    ('Pallof Press',               'core',   'strength', 'isolation', 12, 'core',            'core',       ARRAY['machines']::text[],       'low', NULL),
    -- cardio
    ('SkiErg',                     'cardio', 'cardio',   NULL,        13, 'locomotion',      'full_body',  ARRAY['cardio_machine']::text[], 'low', NULL)
ON CONFLICT (category, name) DO NOTHING;

DO $$
DECLARE found INTEGER;
BEGIN
    SELECT count(*) INTO found FROM exercise_catalog WHERE (category, name) IN (
        ('push', 'Machine Shoulder Press'), ('push', 'Incline Machine Press'), ('push', 'Smith Machine Bench Press'),
        ('push', 'Assisted Dip'), ('push', 'Machine Dip'), ('push', 'Pec Deck'), ('push', 'Machine Lateral Raise'),
        ('push', 'Cable Lateral Raise'), ('push', 'Rope Triceps Extension'), ('push', 'Skull Crusher'),
        ('pull', 'Assisted Pull-Up'), ('pull', 'Machine High Row'), ('pull', 'Chest-Supported Row'),
        ('pull', 'Single-Arm Cable Row'), ('pull', 'Straight-Arm Pulldown'), ('pull', 'Reverse Pec Deck'),
        ('pull', 'Dumbbell Shrug'), ('pull', 'Cable Curl'), ('pull', 'Machine Preacher Curl'),
        ('legs', 'Smith Machine Squat'), ('legs', 'Pendulum Squat'), ('legs', 'Belt Squat'),
        ('legs', 'Machine Hip Thrust'), ('legs', 'Seated Leg Curl'), ('legs', 'Glute-Ham Raise'),
        ('legs', 'Back Extension'), ('legs', 'Cable Kickback'), ('legs', 'Hip Adduction'),
        ('legs', 'Leg Press Calf Raise'),
        ('core', 'Captain''s Chair Leg Raise'), ('core', 'Ab Crunch Machine'), ('core', 'Rotary Torso'),
        ('core', 'Pallof Press'),
        ('cardio', 'SkiErg'));
    IF found <> 34 THEN
        RAISE EXCEPTION 'exercise_catalog has % of the 34 machine entries', found;
    END IF;
END $$;
