-- Spec 016 evidence amendment: an optional sub_pattern so the engine selects
-- isolation and push work by data, not by name; and four power entries so
-- power drafts stop depending on Olympic-lift variants.
ALTER TABLE exercise_catalog
    ADD COLUMN sub_pattern TEXT,
    ADD CONSTRAINT exercise_catalog_sub_pattern_check CHECK (sub_pattern IS NULL OR sub_pattern IN (
        'incline_push', 'chest_isolation', 'shoulder_isolation', 'rear_delt', 'triceps', 'biceps',
        'quad_isolation', 'hamstring_isolation', 'glute_isolation', 'calf'));

UPDATE exercise_catalog AS c
SET sub_pattern = s.sub_pattern
FROM (VALUES
    ('push',  'Incline Barbell Bench Press',  'incline_push'),
    ('push',  'Incline Dumbbell Press',       'incline_push'),
    ('push',  'Cable Fly',                    'chest_isolation'),
    ('push',  'Dumbbell Fly',                 'chest_isolation'),
    ('upper', 'Cable Crossover',              'chest_isolation'),
    ('push',  'Lateral Raise',                'shoulder_isolation'),
    ('pull',  'Face Pull',                    'rear_delt'),
    ('pull',  'Rear Delt Fly',                'rear_delt'),
    ('push',  'Triceps Pushdown',             'triceps'),
    ('push',  'Overhead Triceps Extension',   'triceps'),
    ('pull',  'Barbell Curl',                 'biceps'),
    ('pull',  'Dumbbell Curl',                'biceps'),
    ('pull',  'Hammer Curl',                  'biceps'),
    ('pull',  'Preacher Curl',                'biceps'),
    ('legs',  'Leg Extension',                'quad_isolation'),
    ('legs',  'Lying Leg Curl',               'hamstring_isolation'),
    ('lower', 'Nordic Hamstring Curl',        'hamstring_isolation'),
    ('legs',  'Hip Abduction',                'glute_isolation'),
    ('legs',  'Standing Calf Raise',          'calf'),
    ('legs',  'Seated Calf Raise',            'calf')
) AS s(category, name, sub_pattern)
WHERE c.category = s.category AND c.name = s.name;

INSERT INTO exercise_catalog (name, category, modality, mechanic, sort_order, pattern, muscle_group, equipment_required, impact) VALUES
    ('Box Jump',           'full', 'strength', 'compound', 8,  'explosive', 'quads',     ARRAY['bodyweight']::text[], 'high'),
    ('Broad Jump',         'full', 'strength', 'compound', 9,  'explosive', 'glutes',    ARRAY['bodyweight']::text[], 'high'),
    ('Medicine Ball Slam', 'full', 'strength', 'compound', 10, 'explosive', 'full_body', ARRAY['full_gym']::text[],   'low'),
    ('Push Press',         'push', 'strength', 'compound', 15, 'explosive', 'shoulders', ARRAY['barbell']::text[],    'low')
ON CONFLICT (category, name) DO NOTHING;
