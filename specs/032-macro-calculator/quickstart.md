# Quickstart: Macro calculator and meal template

```bash
go test ./internal/nutritionplan/... ./internal/service/... ./internal/handler/... ./internal/router/...
go test ./...
golangci-lint run
./scripts/validate-database-design.sh
./scripts/smoke-toggle.sh
GYMPULSE_TEST_DATABASE_URL=<owner url> GYMPULSE_TEST_RUNTIME_DATABASE_URL=<gym_pulse_api url> \
  go test -p 1 -race -tags=integration ./internal/dao ./internal/dbmigrate -count=1
```

Manual walk against a local stack (token in `$T`):

```bash
curl -s -H "Authorization: Bearer $T" localhost:8080/api/v1/nutrition/calculator/options
curl -s -H "Authorization: Bearer $T" -H 'Content-Type: application/json' \
  -d '{"unit_system":"imperial","weight":180,"height":70,"age_years":30,"sex":"male","strength_days":4,"cardio_days":1,"goal":"maintenance"}' \
  localhost:8080/api/v1/nutrition/calculate
```

Expected: `bmr_kcal` 1783, `activity.level` `moderate`, `tdee_kcal` 2763, `macros.protein.g` 153,
`macros.fat.g` 59, four meals under `meal_templates[0]`.

## Rollout

1. API #67 (migration 045) merged on 2026-11-16; this branch has main merged in and adds 046.
2. Deploy; the `./migrate` pre-deploy step applies 046. Additive: older apps are unaffected.
3. The app (spec 038) probes `GET /api/v1/nutrition/calculator/options`; a `404` hides the feature,
   so the app may ship before or after, but shows the calculator only against this API.
