-- Removes the helper only. Tables it provisioned keep their RLS, grants, and
-- policies, which are ordinary catalog state independent of the function.
DROP FUNCTION IF EXISTS gym_pulse_private.secure_table(regclass);
DROP SCHEMA IF EXISTS gym_pulse_private;
