-- Enforce the one-active-program invariant at the database boundary.
UPDATE programs SET active=false
WHERE active=true AND id NOT IN (
  SELECT DISTINCT ON (user_id) id FROM programs WHERE active=true ORDER BY user_id, updated_at DESC, created_at DESC
);
CREATE UNIQUE INDEX idx_programs_one_active_per_user ON programs(user_id) WHERE active=true;
