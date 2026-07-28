-- This security boundary is intentionally irreversible. Re-enabling Data API
-- grants or disabling RLS would expose user data during a rollback. Roll
-- forward with a new reviewed migration if the access model changes.
SELECT 1;
