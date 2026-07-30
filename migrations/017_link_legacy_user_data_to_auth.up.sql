-- Early GymPulse tables predated the Supabase Auth foreign-key convention used
-- by the training domain. Enforce the same identity root so an in-flight write
-- cannot survive authoritative auth deletion.
ALTER TABLE public.workout_templates
    ADD CONSTRAINT workout_templates_user_auth_fk
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE public.day_logs
    ADD CONSTRAINT day_logs_user_auth_fk
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE public.user_settings
    ADD CONSTRAINT user_settings_user_auth_fk
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE public.body_weights
    ADD CONSTRAINT body_weights_user_auth_fk
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE public.weekly_plans
    ADD CONSTRAINT weekly_plans_user_auth_fk
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE public.plan_overrides
    ADD CONSTRAINT plan_overrides_user_auth_fk
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE NOT VALID;

-- NOT VALID keeps each lock window short while the constraint is installed;
-- validation still makes the migration fail loudly if staging contains an
-- orphan that must be reconciled before production.
ALTER TABLE public.workout_templates VALIDATE CONSTRAINT workout_templates_user_auth_fk;
ALTER TABLE public.day_logs VALIDATE CONSTRAINT day_logs_user_auth_fk;
ALTER TABLE public.user_settings VALIDATE CONSTRAINT user_settings_user_auth_fk;
ALTER TABLE public.body_weights VALIDATE CONSTRAINT body_weights_user_auth_fk;
ALTER TABLE public.weekly_plans VALIDATE CONSTRAINT weekly_plans_user_auth_fk;
ALTER TABLE public.plan_overrides VALIDATE CONSTRAINT plan_overrides_user_auth_fk;
