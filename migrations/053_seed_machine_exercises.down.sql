-- Removes the 34 entries migration 053 seeded. Saved exercises and logged sets
-- that point at one keep their name; their catalog_id becomes NULL (ON DELETE
-- SET NULL).
DELETE FROM exercise_catalog WHERE (category, name) IN (
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
