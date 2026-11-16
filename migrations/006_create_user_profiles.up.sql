-- Hosted Supabase already has auth.users, owned by supabase_auth_admin, and
-- the postgres role may not create objects in the auth schema. Even
-- CREATE ... IF NOT EXISTS checks that privilege first, so the local stub is
-- created only where the table is missing (plain PostgreSQL in development,
-- test, and CI).
DO $$
BEGIN
    IF to_regclass('auth.users') IS NULL THEN
        CREATE SCHEMA IF NOT EXISTS auth;
        CREATE TABLE auth.users (
            id UUID PRIMARY KEY
        );
    END IF;
END
$$;

CREATE TABLE user_profiles (
    id UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    display_name TEXT,
    avatar_url TEXT,
    onboarding_completed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
