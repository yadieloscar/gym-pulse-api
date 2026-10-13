-- Removes the catalog entry only. starter_workouts and starter_exercises cascade;
-- user programs copied from it keep their snapshot and get starter_program_id NULL.
DELETE FROM starter_programs WHERE slug='push-pull-legs' AND version=1;
