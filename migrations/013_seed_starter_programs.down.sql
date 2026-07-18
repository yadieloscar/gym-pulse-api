DELETE FROM starter_programs
WHERE version=1 AND slug IN (
    'general_health-foundation', 'strength-foundation',
    'hypertrophy-foundation', 'conditioning-foundation',
    'power-foundation', 'body_composition-foundation'
);
