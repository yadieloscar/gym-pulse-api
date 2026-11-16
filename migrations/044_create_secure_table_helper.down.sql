-- Removes the helper only. Tables it provisioned keep their RLS, grants, and
-- policies, which are ordinary catalog state independent of the function.
-- The gym_pulse_private schema belongs to migration 043, whose down drops it.
DROP FUNCTION IF EXISTS gym_pulse_private.secure_table(regclass);
