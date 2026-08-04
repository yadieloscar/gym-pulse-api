-- Rollback removes only the additive criteria-based training aggregate.
DROP TABLE IF EXISTS criteria_training_transitions;
DROP TABLE IF EXISTS criteria_training_exposures;
DROP TABLE IF EXISTS criteria_training_stages;
DROP TABLE IF EXISTS criteria_training_blocks;
