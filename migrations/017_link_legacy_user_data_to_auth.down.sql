ALTER TABLE public.plan_overrides DROP CONSTRAINT IF EXISTS plan_overrides_user_auth_fk;
ALTER TABLE public.weekly_plans DROP CONSTRAINT IF EXISTS weekly_plans_user_auth_fk;
ALTER TABLE public.body_weights DROP CONSTRAINT IF EXISTS body_weights_user_auth_fk;
ALTER TABLE public.user_settings DROP CONSTRAINT IF EXISTS user_settings_user_auth_fk;
ALTER TABLE public.day_logs DROP CONSTRAINT IF EXISTS day_logs_user_auth_fk;
ALTER TABLE public.workout_templates DROP CONSTRAINT IF EXISTS workout_templates_user_auth_fk;
