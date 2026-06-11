CREATE TABLE exercise_catalog (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    category    TEXT NOT NULL,
    modality    TEXT NOT NULL CHECK (modality IN ('strength', 'cardio')),
    mechanic    TEXT CHECK (mechanic IN ('compound', 'isolation')),
    sort_order  INTEGER NOT NULL DEFAULT 0,
    UNIQUE (category, name),
    CHECK (
        (modality = 'strength' AND mechanic IS NOT NULL) OR
        (modality = 'cardio' AND mechanic IS NULL)
    )
);

CREATE INDEX idx_exercise_catalog_category ON exercise_catalog(category);

-- Curated seed data. Read-only in v1 — no admin endpoints; edits ship as
-- migrations so they stay reviewable as data.
INSERT INTO exercise_catalog (name, category, modality, mechanic, sort_order) VALUES
    -- push
    ('Barbell Bench Press',          'push',   'strength', 'compound',  1),
    ('Incline Barbell Bench Press',  'push',   'strength', 'compound',  2),
    ('Dumbbell Bench Press',         'push',   'strength', 'compound',  3),
    ('Incline Dumbbell Press',       'push',   'strength', 'compound',  4),
    ('Overhead Press',               'push',   'strength', 'compound',  5),
    ('Seated Dumbbell Shoulder Press', 'push', 'strength', 'compound',  6),
    ('Close-Grip Bench Press',       'push',   'strength', 'compound',  7),
    ('Dips',                         'push',   'strength', 'compound',  8),
    ('Push-Up',                      'push',   'strength', 'compound',  9),
    ('Cable Fly',                    'push',   'strength', 'isolation', 10),
    ('Dumbbell Fly',                 'push',   'strength', 'isolation', 11),
    ('Lateral Raise',                'push',   'strength', 'isolation', 12),
    ('Triceps Pushdown',             'push',   'strength', 'isolation', 13),
    ('Overhead Triceps Extension',   'push',   'strength', 'isolation', 14),
    -- pull
    ('Deadlift',                     'pull',   'strength', 'compound',  1),
    ('Pull-Up',                      'pull',   'strength', 'compound',  2),
    ('Chin-Up',                      'pull',   'strength', 'compound',  3),
    ('Lat Pulldown',                 'pull',   'strength', 'compound',  4),
    ('Barbell Row',                  'pull',   'strength', 'compound',  5),
    ('Dumbbell Row',                 'pull',   'strength', 'compound',  6),
    ('Seated Cable Row',             'pull',   'strength', 'compound',  7),
    ('T-Bar Row',                    'pull',   'strength', 'compound',  8),
    ('Face Pull',                    'pull',   'strength', 'isolation', 9),
    ('Rear Delt Fly',                'pull',   'strength', 'isolation', 10),
    ('Barbell Curl',                 'pull',   'strength', 'isolation', 11),
    ('Dumbbell Curl',                'pull',   'strength', 'isolation', 12),
    ('Hammer Curl',                  'pull',   'strength', 'isolation', 13),
    ('Preacher Curl',                'pull',   'strength', 'isolation', 14),
    -- legs
    ('Back Squat',                   'legs',   'strength', 'compound',  1),
    ('Front Squat',                  'legs',   'strength', 'compound',  2),
    ('Goblet Squat',                 'legs',   'strength', 'compound',  3),
    ('Romanian Deadlift',            'legs',   'strength', 'compound',  4),
    ('Leg Press',                    'legs',   'strength', 'compound',  5),
    ('Walking Lunge',                'legs',   'strength', 'compound',  6),
    ('Bulgarian Split Squat',        'legs',   'strength', 'compound',  7),
    ('Hip Thrust',                   'legs',   'strength', 'compound',  8),
    ('Step-Up',                      'legs',   'strength', 'compound',  9),
    ('Leg Extension',                'legs',   'strength', 'isolation', 10),
    ('Lying Leg Curl',               'legs',   'strength', 'isolation', 11),
    ('Standing Calf Raise',          'legs',   'strength', 'isolation', 12),
    ('Seated Calf Raise',            'legs',   'strength', 'isolation', 13),
    ('Hip Abduction',                'legs',   'strength', 'isolation', 14),
    -- upper
    ('Arnold Press',                 'upper',  'strength', 'compound',  1),
    ('Machine Chest Press',          'upper',  'strength', 'compound',  2),
    ('Machine Row',                  'upper',  'strength', 'compound',  3),
    ('Landmine Press',               'upper',  'strength', 'compound',  4),
    ('Cable Crossover',              'upper',  'strength', 'isolation', 5),
    -- lower
    ('Sumo Deadlift',                'lower',  'strength', 'compound',  1),
    ('Hack Squat',                   'lower',  'strength', 'compound',  2),
    ('Good Morning',                 'lower',  'strength', 'compound',  3),
    ('Single-Leg Press',             'lower',  'strength', 'compound',  4),
    ('Nordic Hamstring Curl',        'lower',  'strength', 'isolation', 5),
    -- full
    ('Clean and Press',              'full',   'strength', 'compound',  1),
    ('Power Clean',                  'full',   'strength', 'compound',  2),
    ('Kettlebell Swing',             'full',   'strength', 'compound',  3),
    ('Thruster',                     'full',   'strength', 'compound',  4),
    ('Burpee',                       'full',   'strength', 'compound',  5),
    ('Farmer''s Carry',              'full',   'strength', 'compound',  6),
    ('Turkish Get-Up',               'full',   'strength', 'compound',  7),
    -- core
    ('Plank',                        'core',   'strength', 'isolation', 1),
    ('Side Plank',                   'core',   'strength', 'isolation', 2),
    ('Hanging Leg Raise',            'core',   'strength', 'compound',  3),
    ('Ab Wheel Rollout',             'core',   'strength', 'compound',  4),
    ('Cable Crunch',                 'core',   'strength', 'isolation', 5),
    ('Russian Twist',                'core',   'strength', 'isolation', 6),
    ('Dead Bug',                     'core',   'strength', 'isolation', 7),
    ('Bicycle Crunch',               'core',   'strength', 'isolation', 8),
    -- cardio
    ('Treadmill Run',                'cardio', 'cardio',   NULL,        1),
    ('Incline Treadmill Walk',       'cardio', 'cardio',   NULL,        2),
    ('Outdoor Run',                  'cardio', 'cardio',   NULL,        3),
    ('Stationary Bike',              'cardio', 'cardio',   NULL,        4),
    ('Assault Bike',                 'cardio', 'cardio',   NULL,        5),
    ('Rowing Machine',               'cardio', 'cardio',   NULL,        6),
    ('Elliptical',                   'cardio', 'cardio',   NULL,        7),
    ('Stairmaster',                  'cardio', 'cardio',   NULL,        8),
    ('Jump Rope',                    'cardio', 'cardio',   NULL,        9),
    ('Swimming',                     'cardio', 'cardio',   NULL,        10),
    ('HIIT Circuit',                 'cardio', 'cardio',   NULL,        11),
    ('Sled Push',                    'cardio', 'cardio',   NULL,        12);
