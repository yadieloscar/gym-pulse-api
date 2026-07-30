-- GymPulse application data is available only through the Go API. RLS is
-- defense in depth; no Data API policies are intentionally created.
ALTER TABLE public.workout_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.exercises ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.day_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.exercise_overrides ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.user_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.user_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.body_weights ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.exercise_catalog ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.weekly_plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.plan_overrides ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.set_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.training_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.starter_programs ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.starter_workouts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.starter_exercises ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.programs ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.program_workouts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.program_exercises ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.scheduled_workouts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.scheduled_sets ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.workout_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.day_participation ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.idempotency_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.legacy_adoptions ENABLE ROW LEVEL SECURITY;

-- Supabase Data API roles exist in hosted projects but not in the lightweight
-- PostgreSQL image used by local and CI smoke tests. Apply the same revocations
-- to every role that exists in the target environment.
DO $$
DECLARE
    data_api_role TEXT;
    default_owner TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname
        FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %I',
            data_api_role
        );
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %I',
            data_api_role
        );
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public FROM %I',
            data_api_role
        );

        -- Default privileges are scoped to the object-creating role. Migrations
        -- run as the current role; hosted Supabase normally uses postgres.
        FOR default_owner IN
            SELECT rolname
            FROM pg_roles
            WHERE rolname IN (current_user, 'postgres')
              AND pg_has_role(current_user, oid, 'USAGE')
        LOOP
            EXECUTE format(
                'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE ALL PRIVILEGES ON TABLES FROM %I',
                default_owner,
                data_api_role
            );
            EXECUTE format(
                'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE ALL PRIVILEGES ON SEQUENCES FROM %I',
                default_owner,
                data_api_role
            );
            EXECUTE format(
                'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE ALL PRIVILEGES ON FUNCTIONS FROM %I',
                default_owner,
                data_api_role
            );
        END LOOP;
    END LOOP;

    -- PostgreSQL grants function execution to PUBLIC by default. Revoking it
    -- prevents Data API roles from inheriting access through PUBLIC.
    REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC;
    FOR default_owner IN
        SELECT rolname
        FROM pg_roles
        WHERE rolname IN (current_user, 'postgres')
          AND pg_has_role(current_user, oid, 'USAGE')
    LOOP
        EXECUTE format(
            'ALTER DEFAULT PRIVILEGES FOR ROLE %I REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC',
            default_owner
        );
    END LOOP;
END
$$;
