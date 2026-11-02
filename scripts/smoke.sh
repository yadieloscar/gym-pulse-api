#!/usr/bin/env bash
# scripts/smoke.sh — 5-second contract smoke test for the local API.
#
# What it asserts:
#   1. /health is 200
#   2. PUT /api/v1/settings atomically applies partial changes and preserves
#      omitted values
#   3. PUT /api/v1/settings accepts weekly_goal=1 (regression for the bug
#      where validator required min=3 and broke onboarding)
#   4. GET /api/v1/stats/distribution returns either an array or `null`
#   5. legacy adoption, exact goal-mutation replay, and bounded date ranges
#   6. goal profile → starter copy → dated scheduled workout → required and
#      extra sets → incomplete outcome → participation remains separate
#   7. notes-only and overrides-only day-log edits preserve performed sets
#   8. sport creation is idempotent and records participation for the date
#   9. an imported health workout is created once; re-import answers 200
#  10. a finished workout's health export is recorded once (201, 200, 409)
#  11. a sport unit schedules a session completed once by a linked activity
#  12. a wrong link is repaired by unlink and link-existing (contract 1.3.0)
#  13. nutrition target and days: save, replay, conflicts, Health precedence, clear, ranges
#
# Requirements:
#   - API container running on :8080 (see README local development instructions)
#   - SUPABASE_JWKS_URL **must be unset** in the container, or this script
#     can't authenticate with an HMAC test JWT. Temporary override:
#
#       docker compose stop api
#       SUPABASE_JWKS_URL='' docker compose up -d api   # re-creates without JWKS
#
#     Then re-enable JWKS the same way after smoke tests pass.
#
# Usage:
#   ./scripts/smoke.sh                # uses default localhost:8080
#   API=http://localhost:8080 ./scripts/smoke.sh
#   JWT_SECRET=my-secret ./scripts/smoke.sh

set -uo pipefail

API="${API:-http://localhost:8080}"
JWT_SECRET="${JWT_SECRET:-local-dev-secret-change-me}"
USER_ID="${USER_ID:-00000000-0000-0000-0000-000000000001}"

pass=0
fail=0

ok()   { printf "  \033[32m✓\033[0m %s\n" "$1"; pass=$((pass+1)); }
bad()  { printf "  \033[31m✗\033[0m %s\n    %s\n" "$1" "$2"; fail=$((fail+1)); }
step() { printf "\n\033[1m%s\033[0m\n" "$1"; }

# ---------- mint an HMAC JWT (HS256) ----------
TOKEN=$(python3 - "$USER_ID" "$JWT_SECRET" <<'PY'
import sys, json, time, hmac, hashlib, base64
def b64(b): return base64.urlsafe_b64encode(b).rstrip(b"=").decode()
sub, secret = sys.argv[1], sys.argv[2].encode()
header  = b64(json.dumps({"alg":"HS256","typ":"JWT"}).encode())
payload = b64(json.dumps({"sub":sub,"iat":int(time.time()),"exp":int(time.time())+300}).encode())
sig     = b64(hmac.new(secret, f"{header}.{payload}".encode(), hashlib.sha256).digest())
print(f"{header}.{payload}.{sig}")
PY
)
auth=(-H "Authorization: Bearer $TOKEN")

# ---------- 1. health ----------
step "1. /health"
code=$(curl -s -o /dev/null -w "%{http_code}" "$API/health")
[ "$code" = "200" ] && ok "/health → 200" || bad "/health → $code (is the container up?)" "$code"

# ---------- 2. settings: partial body preserves omitted values ----------
step "2. PUT /api/v1/settings applies only fields present in a partial body"
seed_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/settings" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d '{"weekly_goal":5,"weight_unit":"kg","palette":"obsidianEmber"}')
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/settings" \
  "${auth[@]}" -H "Content-Type: application/json" -d '{"palette":"abyssCerulean"}')
body=$(cat /tmp/smoke.body)
if [ "$seed_status" = "200" ] && [ "$resp" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['weekly_goal']==5 and d['weight_unit']=='kg' and d['palette']=='abyssCerulean'" "$body" 2>/dev/null; then
  ok "partial update changed palette and preserved weekly_goal + weight_unit"
elif [ "$resp" = "401" ]; then
  bad "got 401 — JWKS is still enabled in the container; see header comment" "$body"
else
  bad "partial settings update lost or rejected omitted values (seed=$seed_status update=$resp)" "$body"
fi

# ---------- 3. settings: weekly_goal=1 must succeed ----------
step "3. PUT /api/v1/settings with weekly_goal=1 should 200 (onboarding regression)"
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/settings" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d '{"weekly_goal":1,"weight_unit":"lb"}')
body=$(cat /tmp/smoke.body)
case "$resp" in
  200) ok "weekly_goal=1 accepted (body: $body)" ;;
  400) bad "weekly_goal=1 rejected — validator min reverted to 3?" "$body" ;;
  401) bad "401 — JWKS toggle issue, see header" "$body" ;;
  *)   bad "unexpected status $resp" "$body" ;;
esac

# ---------- 4. distribution shape ----------
step "4. GET /api/v1/stats/distribution returns {types: TypeDistribution[]}"
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/stats/distribution" "${auth[@]}")
body=$(cat /tmp/smoke.body)
if [ "$resp" != "200" ]; then
  bad "expected 200, got $resp" "$body"
elif python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert isinstance(d, dict) and 'types' in d and (d['types'] is None or isinstance(d['types'], list))" "$body" 2>/dev/null; then
  ok "shape is {types: ...} as documented (body: $body)"
else
  bad "shape drift — client unwrap will break" "$body"
fi

# ---------- 5. exercise catalog ----------
step "5. GET /api/v1/exercises returns {exercises: [...]} with seeded catalog"
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/exercises" "${auth[@]}")
body=$(cat /tmp/smoke.body)
if [ "$resp" != "200" ]; then
  bad "expected 200, got $resp" "$body"
elif python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert isinstance(d, dict) and isinstance(d.get('exercises'), list) and len(d['exercises']) > 50 and {'name','category','modality'} <= set(d['exercises'][0])" "$body" 2>/dev/null; then
  count=$(python3 -c "import json,sys; print(len(json.loads(sys.argv[1])['exercises']))" "$body")
  ok "catalog seeded with $count entries in documented shape"
else
  bad "catalog missing, under-seeded (<=50), or shape drift" "$(echo "$body" | head -c 200)"
fi

# ---------- 6. weekly plan round-trip ----------
step "6. PUT /api/v1/plan/weekly then GET /api/v1/plan returns it"
legacy_template_resp=$(curl -s -w $'\n%{http_code}' -X POST "$API/api/v1/templates" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d '{"name":"Smoke Legacy Strength","type_id":"legs","subtype_id":"strength","exercises":[{"name":"Smoke Legacy Squat","sort_order":1,"sets":3,"reps":5}]}')
legacy_template_status=${legacy_template_resp##*$'\n'}
legacy_template_body=${legacy_template_resp%$'\n'*}
legacy_template_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$legacy_template_body" 2>/dev/null || true)
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/plan/weekly" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d "{\"days\":[{\"weekday\":1,\"template_id\":\"$legacy_template_id\",\"rest\":false},{\"weekday\":3,\"rest\":true}]}" )
body=$(cat /tmp/smoke.body)
if [ "$legacy_template_status" != "201" ] || [ -z "$legacy_template_id" ] || [ "$resp" != "200" ]; then
  bad "PUT weekly plan expected 200, got $resp" "$body"
else
  resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/plan" "${auth[@]}")
  body=$(cat /tmp/smoke.body)
  if [ "$resp" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert isinstance(d.get('weekly'), list) and len(d['weekly'])==2 and isinstance(d.get('overrides'), list)" "$body" 2>/dev/null; then
    ok "weekly plan stored and returned (body: $(echo "$body" | head -c 120)...)"
  else
    bad "GET plan shape drift or wrong count" "$(echo "$body" | head -c 200)"
  fi
fi

# ---------- 7. exercise set history wired ----------
step "7. GET /api/v1/exercises/history returns an array"
rand=$(python3 -c "import uuid; print(uuid.uuid4())")
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/exercises/history?ids=$rand" "${auth[@]}")
body=$(cat /tmp/smoke.body)
if [ "$resp" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert isinstance(d, list)" "$body" 2>/dev/null; then
  ok "exercise history endpoint returns an array (body: $(echo "$body" | head -c 80))"
else
  bad "GET exercises/history expected 200 array, got $resp" "$(echo "$body" | head -c 200)"
fi

# ---------- 8. exercise records wired ----------
step "8. GET /api/v1/exercises/records returns an array"
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/exercises/records?ids=$rand" "${auth[@]}")
body=$(cat /tmp/smoke.body)
if [ "$resp" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert isinstance(d, list)" "$body" 2>/dev/null; then
  ok "exercise records endpoint returns an array (body: $(echo "$body" | head -c 60))"
else
  bad "GET exercises/records expected 200 array, got $resp" "$(echo "$body" | head -c 200)"
fi

# ---------- 9. weekly volume series ----------
step "9. GET /api/v1/stats/volume?weeks=4 returns a 4-week series"
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/stats/volume?weeks=4" "${auth[@]}")
body=$(cat /tmp/smoke.body)
if [ "$resp" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert isinstance(d, list) and len(d)==4 and {'week_start','volume'} <= set(d[0])" "$body" 2>/dev/null; then
  ok "volume returns a continuous 4-week series (body: $(echo "$body" | head -c 80))"
else
  bad "GET stats/volume expected 200 4-week series, got $resp" "$(echo "$body" | head -c 200)"
fi

# ---------- 10. goal-based training happy path ----------
step "10. Goal profile → starter program → scheduled workout → participation"
profile_op="smoke-profile-v1"
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/training-profile" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $profile_op" \
  -d "{\"primary_goal\":\"strength\",\"available_days\":[1,4],\"usual_activity\":\"moderate\",\"experience\":\"beginner\",\"equipment\":[\"bodyweight\"],\"session_duration_minutes\":45,\"timezone\":\"UTC\",\"preferences\":{},\"expected_revision\":0,\"operation_key\":\"$profile_op\"}")
body=$(cat /tmp/smoke.body)
if [ "$resp" != "200" ] || ! python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['primary_goal']=='strength' and d['revision']>=1" "$body" 2>/dev/null; then
  bad "training profile expected 200 with primary_goal=strength, got $resp" "$(echo "$body" | head -c 240)"
else
  ok "training profile persisted primary_goal=strength"
  profile_initial_body=$body
  profile_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$body")
  profile_advance_op="smoke-profile-v2"
  profile_advance_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/training-profile" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $profile_advance_op" \
    -d "{\"session_duration_minutes\":50,\"expected_revision\":$profile_revision,\"operation_key\":\"$profile_advance_op\"}")
  profile_replay_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/training-profile" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $profile_op" \
    -d "{\"primary_goal\":\"strength\",\"available_days\":[1,4],\"usual_activity\":\"moderate\",\"experience\":\"beginner\",\"equipment\":[\"bodyweight\"],\"session_duration_minutes\":45,\"timezone\":\"UTC\",\"preferences\":{},\"expected_revision\":0,\"operation_key\":\"$profile_op\"}")
  profile_replay_body=$(cat /tmp/smoke.body)
  profile_conflict_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/training-profile" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $profile_op" \
    -d "{\"primary_goal\":\"strength\",\"available_days\":[1,4],\"usual_activity\":\"moderate\",\"experience\":\"beginner\",\"equipment\":[\"bodyweight\"],\"session_duration_minutes\":46,\"timezone\":\"UTC\",\"preferences\":{},\"expected_revision\":0,\"operation_key\":\"$profile_op\"}")
  profile_conflict_body=$(cat /tmp/smoke.body)
  profile_current_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/training-profile" "${auth[@]}")
  profile_current_body=$(cat /tmp/smoke.body)
  if [ "$profile_advance_status" = "200" ] && [ "$profile_replay_status" = "200" ] && [ "$profile_conflict_status" = "409" ] && [ "$profile_current_status" = "200" ] && \
    python3 -c "import json,sys; original,replay,conflict,current=map(json.loads,sys.argv[1:]); assert original==replay and conflict['code']=='IDEMPOTENCY_CONFLICT' and current['session_duration_minutes']==50 and current['revision']>original['revision']" "$profile_initial_body" "$profile_replay_body" "$profile_conflict_body" "$profile_current_body" 2>/dev/null; then
    ok "profile retry replayed the original response after a later revision without reverting state"
  else
    bad "profile exact replay failed (advance=$profile_advance_status replay=$profile_replay_status conflict=$profile_conflict_status current=$profile_current_status)" "$(echo "$profile_current_body" | head -c 280)"
  fi

  adopt_op="smoke-adopt-legacy-v1"
  adopt_payload="{\"operation_key\":\"$adopt_op\",\"expected_revision\":0}"
  curl -s -w $'\n%{http_code}' -X POST "$API/api/v1/programs/adopt-legacy" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $adopt_op" -d "$adopt_payload" > /tmp/smoke.adopt.1 &
  adopt_pid_1=$!
  curl -s -w $'\n%{http_code}' -X POST "$API/api/v1/programs/adopt-legacy" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $adopt_op" -d "$adopt_payload" > /tmp/smoke.adopt.2 &
  adopt_pid_2=$!
  wait "$adopt_pid_1"
  wait "$adopt_pid_2"
  adopt_result=$(cat /tmp/smoke.adopt.1)
  adopt_status=${adopt_result##*$'\n'}
  adopt_body=${adopt_result%$'\n'*}
  adopt_replay_result=$(cat /tmp/smoke.adopt.2)
  adopt_replay_status=${adopt_replay_result##*$'\n'}
  adopt_replay_body=${adopt_replay_result%$'\n'*}
  adopt_program_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['program']['id'])" "$adopt_body" 2>/dev/null || true)
  adopt_again_op="smoke-adopt-legacy-v2"
  adopt_again_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/programs/adopt-legacy" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $adopt_again_op" \
    -d "{\"operation_key\":\"$adopt_again_op\",\"expected_revision\":0}")
  adopt_again_body=$(cat /tmp/smoke.body)
  if [ "$adopt_status" = "200" ] && [ "$adopt_replay_status" = "200" ] && [ "$adopt_again_status" = "200" ] && [ -n "$adopt_program_id" ] && \
    python3 -c "import json,sys; a,r,n=map(json.loads,sys.argv[1:]); assert a==r and a['adopted'] is True and n['adopted'] is False and a['program']['id']==n['program']['id'] and len(a['schedule'])==1" "$adopt_body" "$adopt_replay_body" "$adopt_again_body" 2>/dev/null; then
    ok "concurrent legacy adoption created once; exact replay and later retry created no duplicates"
  else
    bad "legacy adoption/replay failed (create=$adopt_status replay=$adopt_replay_status again=$adopt_again_status)" "$(echo "$adopt_again_body" | head -c 280)"
  fi

  range_ok_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/schedule?from=2030-01-01&to=2031-01-01" "${auth[@]}")
  range_reject_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/schedule?from=2030-01-01&to=2031-01-02" "${auth[@]}")
  range_reject_body=$(cat /tmp/smoke.body)
  if [ "$range_ok_status" = "200" ] && [ "$range_reject_status" = "422" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['details']['field']=='range'" "$range_reject_body" 2>/dev/null; then
    ok "366 inclusive dates accepted and 367 rejected with field=range"
  else
    bad "shared range boundary failed (366=$range_ok_status 367=$range_reject_status)" "$range_reject_body"
  fi

  resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/starter-programs?primary_goal=strength&available_days=2&experience=beginner&equipment=bodyweight&session_duration_minutes=45" "${auth[@]}")
  body=$(cat /tmp/smoke.body)
  starter_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['starter_programs'][0]['id'])" "$body" 2>/dev/null || true)
  starter_version=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['starter_programs'][0]['version'])" "$body" 2>/dev/null || true)
  if [ "$resp" != "200" ] || [ -z "$starter_id" ]; then
    bad "starter-programs returned no deterministic strength candidate" "$(echo "$body" | head -c 240)"
  else
    clone_op="smoke-clone-strength-v1"
    resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/programs/from-starter" \
      "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $clone_op" \
      -d "{\"starter_program_id\":\"$starter_id\",\"starter_version\":$starter_version,\"name\":\"Smoke Strength\",\"operation_key\":\"$clone_op\"}")
    body=$(cat /tmp/smoke.body)
    program_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$body" 2>/dev/null || true)
    program_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$body" 2>/dev/null || true)
    if [ "$resp" != "201" ] || [ -z "$program_id" ]; then
      bad "starter copy expected 201 Program, got $resp" "$(echo "$body" | head -c 240)"
    else
      read -r week_from week_to <<EOF
$(python3 - <<'PY'
from datetime import date, timedelta
today=date.today(); monday=today+timedelta(days=(7-today.weekday())%7)
print(monday.isoformat(), (monday+timedelta(days=6)).isoformat())
PY
)
EOF
      materialize_op="smoke-materialize-week-v1"
      resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/schedule/materialize" \
        "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $materialize_op" \
        -d "{\"program_id\":\"$program_id\",\"from\":\"$week_from\",\"to\":\"$week_to\",\"operation_key\":\"$materialize_op\",\"expected_revision\":$program_revision}")
      body=$(cat /tmp/smoke.body)
      workout_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['scheduled_workouts'][0]['id'])" "$body" 2>/dev/null || true)
      set_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['scheduled_workouts'][0]['required_sets'][0]['id'])" "$body" 2>/dev/null || true)
      correction_set_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['scheduled_workouts'][0]['required_sets'][1]['id'])" "$body" 2>/dev/null || true)
      set_catalog_json=$(python3 -c "import json,sys; value=json.loads(sys.argv[1])['scheduled_workouts'][0]['required_sets'][0].get('catalog_id'); print(json.dumps(value))" "$body" 2>/dev/null || echo null)
      workout_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['scheduled_workouts'][0]['revision'])" "$body" 2>/dev/null || true)
      if [ "$resp" != "201" ] || [ -z "$workout_id" ] || [ -z "$set_id" ] || [ -z "$correction_set_id" ]; then
        bad "schedule materialize expected dated scheduled_workout, got $resp" "$(echo "$body" | head -c 280)"
      else
        foreign_set_id=$(python3 -c "import uuid; print(uuid.uuid4())")
        rollback_op="smoke-required-rollback-v1"
        rollback_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/scheduled-workouts/$workout_id/sets/$foreign_set_id" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $rollback_op" \
          -d "{\"operation_key\":\"$rollback_op\",\"expected_revision\":$workout_revision,\"actual_reps\":10,\"completed\":true}")
        rollback_sessions_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/workout-sessions?from=$week_from&to=$week_to" "${auth[@]}")
        rollback_sessions_body=$(cat /tmp/smoke.body)
        if [ "$rollback_status" = "404" ] && [ "$rollback_sessions_status" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert not any(x.get('scheduled_workout_id')==sys.argv[2] for x in d['workout_sessions'])" "$rollback_sessions_body" "$workout_id" 2>/dev/null; then
          ok "failed required-set mutation rolled back lazy session creation"
        else
          bad "required-set rollback failed (mutation=$rollback_status sessions=$rollback_sessions_status)" "$(echo "$rollback_sessions_body" | head -c 280)"
        fi

        set_op="smoke-check-required-v1"
        resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/scheduled-workouts/$workout_id/sets/$set_id" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $set_op" \
          -d "{\"operation_key\":\"$set_op\",\"expected_revision\":$workout_revision,\"actual_reps\":10,\"completed\":true}")
        body=$(cat /tmp/smoke.body)
        required_status=$resp
        required_body=$body
        required_revision=$workout_revision
        workout_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$body" 2>/dev/null || true)
        extra_op="smoke-extra-set-v1"
        resp_extra=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/extra-sets" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $extra_op" \
          -d "{\"operation_key\":\"$extra_op\",\"expected_revision\":$workout_revision,\"exercise_id\":$set_catalog_json,\"exercise_name\":\"Air Squat\",\"exercise_category\":\"legs\",\"exercise_modality\":\"strength\",\"set_index\":1,\"actual_reps\":20,\"completed\":true}")
        body=$(cat /tmp/smoke.body)
        extra_body=$body
        workout_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$body" 2>/dev/null || true)
        complete_op="smoke-complete-incomplete-v1"
        resp_complete=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/complete" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $complete_op" \
          -d "{\"operation_key\":\"$complete_op\",\"expected_revision\":$workout_revision}")
        body=$(cat /tmp/smoke.body)
        complete_body=$body
        complete_revision=$workout_revision
        finalized_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$body" 2>/dev/null || true)
        status=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['status'])" "$body" 2>/dev/null || true)

        correction_op="smoke-correct-finalized-v1"
        correction_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/scheduled-workouts/$workout_id/sets/$correction_set_id" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $correction_op" \
          -d "{\"operation_key\":\"$correction_op\",\"expected_revision\":$finalized_revision,\"actual_reps\":8,\"completed\":true}")
        correction_body=$(cat /tmp/smoke.body)
        correction_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$correction_body" 2>/dev/null || true)
        target_op="smoke-target-finalized-v1"
        target_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PATCH "$API/api/v1/scheduled-workouts/$workout_id/sets/$correction_set_id/target" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $target_op" \
          -d "{\"operation_key\":\"$target_op\",\"expected_revision\":$correction_revision,\"target_reps\":9}")
        target_body=$(cat /tmp/smoke.body)
        finalized_extra_op="smoke-extra-finalized-v1"
        finalized_extra_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/extra-sets" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $finalized_extra_op" \
          -d "{\"operation_key\":\"$finalized_extra_op\",\"expected_revision\":$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$target_body" 2>/dev/null || true),\"exercise_id\":null,\"exercise_name\":\"Too late\",\"exercise_category\":\"legs\",\"exercise_modality\":\"strength\",\"set_index\":2,\"completed\":true}")

        required_replay_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/scheduled-workouts/$workout_id/sets/$set_id" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $set_op" \
          -d "{\"operation_key\":\"$set_op\",\"expected_revision\":$required_revision,\"actual_reps\":10,\"completed\":true}")
        required_replay_body=$(cat /tmp/smoke.body)
        required_conflict_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/scheduled-workouts/$workout_id/sets/$set_id" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $set_op" \
          -d "{\"operation_key\":\"$set_op\",\"expected_revision\":$required_revision,\"actual_reps\":9,\"completed\":true}")
        required_conflict_body=$(cat /tmp/smoke.body)
        complete_replay_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/complete" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $complete_op" \
          -d "{\"operation_key\":\"$complete_op\",\"expected_revision\":$complete_revision}")
        complete_replay_body=$(cat /tmp/smoke.body)
        resp_part=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/participation?from=$week_from&to=$week_to" "${auth[@]}")
        participation_body=$(cat /tmp/smoke.body)
        if [ "$required_status" = "200" ] && [ "$resp_extra" = "201" ] && [ "$resp_complete" = "200" ] && [ "$status" = "incomplete" ] && [ "$correction_status" = "200" ] && [ "$target_status" = "200" ] && [ "$finalized_extra_status" = "409" ] && [ "$required_replay_status" = "200" ] && [ "$required_conflict_status" = "409" ] && [ "$complete_replay_status" = "200" ] && [ "$resp_part" = "200" ] && \
          python3 -c "import json,sys; original,replay,extra,complete,correction,target,complete_replay,conflict,participation=map(json.loads,sys.argv[1:]); assert original==replay and complete==complete_replay and correction['finalized_at']==complete['finalized_at'] and correction['status']=='incomplete' and target['status']=='incomplete' and all(x.get('exercise_id') is None for x in extra['extra_sets']) and conflict['code']=='IDEMPOTENCY_CONFLICT' and any(x['participated'] for x in participation['participation'])" "$required_body" "$required_replay_body" "$extra_body" "$complete_body" "$correction_body" "$target_body" "$complete_replay_body" "$required_conflict_body" "$participation_body" 2>/dev/null; then
          ok "retries stayed exact; history corrections preserved finalization and participation"
        else
          bad "goal training lifecycle failed (set=$required_status/$required_replay_status/$required_conflict_status extra=$resp_extra/$finalized_extra_status complete=$resp_complete/$complete_replay_status correction=$correction_status/$target_status status=$status participation=$resp_part)" "$(echo "$participation_body" | head -c 280)"
        fi
        # Missed-workout resolution routes: an incomplete workout cannot be
        # skipped, unskipping an unresolved workout is a no-op, and the
        # Idempotency-Key contract applies.
        resolved_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$correction_body" 2>/dev/null || true)
        target_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$target_body" 2>/dev/null || echo "$resolved_revision")
        skip_op="smoke-skip-incomplete-v1"
        skip_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/skip" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $skip_op" \
          -d "{\"operation_key\":\"$skip_op\",\"expected_revision\":$target_revision}")
        skip_body=$(cat /tmp/smoke.body)
        unskip_op="smoke-unskip-unresolved-v1"
        unskip_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/unskip" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $unskip_op" \
          -d "{\"operation_key\":\"$unskip_op\",\"expected_revision\":$target_revision}")
        unskip_body=$(cat /tmp/smoke.body)
        mismatch_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/skip" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: other-key" \
          -d "{\"operation_key\":\"$skip_op\",\"expected_revision\":$target_revision}")
        missing_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$(python3 -c 'import uuid; print(uuid.uuid4())')/skip" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-skip-missing-v1" \
          -d "{\"operation_key\":\"smoke-skip-missing-v1\",\"expected_revision\":1}")
        if [ "$skip_status" = "409" ] && [ "$unskip_status" = "200" ] && [ "$mismatch_status" = "422" ] && [ "$missing_status" = "404" ] && \
          python3 -c "import json,sys; skip,unskip=map(json.loads,sys.argv[1:]); assert skip['code']=='RESOLUTION_CONFLICT' and skip['resource']['status']=='incomplete' and 'resolution' not in unskip" "$skip_body" "$unskip_body" 2>/dev/null; then
          ok "missed-workout resolution routes enforce outcome, idempotency key, and ownership"
        else
          bad "resolution routes failed (skip=$skip_status unskip=$unskip_status mismatch=$mismatch_status missing=$missing_status)" "$(echo "$skip_body" | head -c 240)"
        fi
      fi
    fi
  fi
fi

# ---------- 11. off-plan session keeps date identity separate ----------
step "11. POST two off-plan workout_sessions on one date returns distinct UUIDs"
session_date=$(python3 -c "from datetime import date; print(date.today().isoformat())")
session_ids=""
for suffix in a b; do
  session_op="smoke-off-plan-$suffix-v1"
  session_payload="{\"scheduled_workout_id\":null,\"date\":\"$session_date\",\"name\":\"Off-plan $suffix\",\"operation_key\":\"$session_op\",\"expected_revision\":0}"
  resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/workout-sessions" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $session_op" \
    -d "$session_payload")
  body=$(cat /tmp/smoke.body)
  session_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$body" 2>/dev/null || true)
  [ "$resp" = "201" ] && session_ids="$session_ids $session_id"
  if [ "$suffix" = "a" ]; then
    first_session_id=$session_id
    first_session_body=$body
    first_session_status=$resp
    first_session_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$body" 2>/dev/null || true)
    first_session_replay_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/workout-sessions" \
      "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $session_op" -d "$session_payload")
    first_session_replay_body=$(cat /tmp/smoke.body)
  fi
done
if [ "$(echo "$session_ids" | xargs -n1 | sort -u | wc -l | tr -d ' ')" = "2" ] && [ "$first_session_status" = "201" ] && [ "$first_session_replay_status" = "201" ] && \
  python3 -c "import json,sys; assert json.loads(sys.argv[1])==json.loads(sys.argv[2])" "$first_session_body" "$first_session_replay_body" 2>/dev/null; then
  ok "multiple off-plan session UUIDs coexist and create replay is exact"
else
  bad "expected two distinct off-plan workout session IDs" "$session_ids"
fi

session_complete_op="smoke-complete-session-v1"
session_complete_payload="{\"status\":\"completed\",\"operation_key\":\"$session_complete_op\",\"expected_revision\":$first_session_revision}"
session_complete_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PATCH "$API/api/v1/workout-sessions/$first_session_id" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $session_complete_op" -d "$session_complete_payload")
session_complete_body=$(cat /tmp/smoke.body)
session_complete_replay_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PATCH "$API/api/v1/workout-sessions/$first_session_id" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $session_complete_op" -d "$session_complete_payload")
session_complete_replay_body=$(cat /tmp/smoke.body)
session_complete_conflict_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PATCH "$API/api/v1/workout-sessions/$first_session_id" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $session_complete_op" \
  -d "{\"status\":\"active\",\"operation_key\":\"$session_complete_op\",\"expected_revision\":$first_session_revision}")
session_complete_conflict_body=$(cat /tmp/smoke.body)
session_participation_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/participation?from=$session_date&to=$session_date" "${auth[@]}")
session_participation_body=$(cat /tmp/smoke.body)
if [ "$session_complete_status" = "200" ] && [ "$session_complete_replay_status" = "200" ] && [ "$session_complete_conflict_status" = "409" ] && [ "$session_participation_status" = "200" ] && \
  python3 -c "import json,sys; a,r,c,p=map(json.loads,sys.argv[1:]); assert a==r and a['status']=='completed' and c['code']=='IDEMPOTENCY_CONFLICT' and any(x['date']==a['date'] and x['participated'] for x in p['participation'])" "$session_complete_body" "$session_complete_replay_body" "$session_complete_conflict_body" "$session_participation_body" 2>/dev/null; then
  ok "session completion replayed exactly and participation committed atomically"
else
  bad "session completion acceptance failed (complete=$session_complete_status replay=$session_complete_replay_status conflict=$session_complete_conflict_status participation=$session_participation_status)" "$(echo "$session_participation_body" | head -c 280)"
fi

# ---------- 12. lossless day-log partial updates ----------
step "12. Partial day-log edits preserve unrelated workout detail"
log_date="2026-01-15"
template_resp=$(curl -s -w $'\n%{http_code}' -X POST "$API/api/v1/templates" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d '{"name":"Smoke Lossless Log","type_id":"push","subtype_id":"hypertrophy","exercises":[{"name":"Smoke Bench Press","sort_order":1,"sets":3,"reps":8}]}')
template_status=${template_resp##*$'\n'}
template_body=${template_resp%$'\n'*}
template_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$template_body" 2>/dev/null || true)
exercise_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['exercises'][0]['id'])" "$template_body" 2>/dev/null || true)

create_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/logs" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d "{\"date\":\"$log_date\",\"type_id\":\"push\",\"subtype_id\":\"hypertrophy\",\"template_id\":\"$template_id\",\"session_notes\":\"original\",\"overrides\":[{\"exercise_id\":\"$exercise_id\",\"actual_sets\":3,\"actual_reps\":8,\"actual_weight\":135,\"notes\":\"original override\",\"skipped\":false}],\"set_logs\":[{\"exercise_id\":\"$exercise_id\",\"set_index\":1,\"target_reps\":8,\"target_weight\":135,\"actual_reps\":8,\"actual_weight\":135,\"duration_seconds\":null,\"completed\":true}]}")

notes_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/logs/$log_date" \
  "${auth[@]}" -H "Content-Type: application/json" -d '{"session_notes":"notes only"}')
notes_body=$(cat /tmp/smoke.body)
notes_preserved=false
if [ "$notes_status" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['session_notes']=='notes only' and len(d['overrides'])==1 and len(d['set_logs'])==1 and d['set_logs'][0]['actual_reps']==8" "$notes_body" 2>/dev/null; then
  notes_preserved=true
fi

overrides_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/logs/$log_date" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d "{\"overrides\":[{\"exercise_id\":\"$exercise_id\",\"actual_sets\":4,\"actual_reps\":6,\"actual_weight\":145,\"notes\":\"override only\",\"skipped\":false}]}")
overrides_body=$(cat /tmp/smoke.body)
overrides_preserved=false
if [ "$overrides_status" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['session_notes']=='notes only' and len(d['overrides'])==1 and d['overrides'][0]['actual_sets']==4 and len(d['set_logs'])==1 and d['set_logs'][0]['actual_reps']==8" "$overrides_body" 2>/dev/null; then
  overrides_preserved=true
fi

foreign_exercise_id=$(python3 -c "import uuid; print(uuid.uuid4())")
rollback_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/logs/$log_date" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d "{\"set_logs\":[{\"exercise_id\":\"$foreign_exercise_id\",\"set_index\":1,\"actual_reps\":5,\"completed\":true}]}")
readback_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/logs/$log_date" "${auth[@]}")
rollback_body=$(cat /tmp/smoke.body)
rollback_preserved=false
if [ "$rollback_status" = "422" ] && [ "$readback_status" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['session_notes']=='notes only' and len(d['overrides'])==1 and len(d['set_logs'])==1 and d['set_logs'][0]['actual_reps']==8" "$rollback_body" 2>/dev/null; then
  rollback_preserved=true
fi

clear_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/logs/$log_date" \
  "${auth[@]}" -H "Content-Type: application/json" -d '{"set_logs":[]}')
clear_body=$(cat /tmp/smoke.body)
if [ "$template_status" = "201" ] && [ "$create_status" = "201" ] && [ "$notes_preserved" = true ] && [ "$overrides_preserved" = true ] && [ "$rollback_preserved" = true ] && [ "$clear_status" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['session_notes']=='notes only' and len(d['overrides'])==1 and len(d.get('set_logs',[]))==0" "$clear_body" 2>/dev/null; then
  ok "partial edits preserved detail; failed replacement rolled back; explicit [] cleared only set_logs"
else
  bad "lossless day-log acceptance failed (template=$template_status create=$create_status notes=$notes_status overrides=$overrides_status rollback=$rollback_status/$readback_status clear=$clear_status)" "$(echo "$clear_body" | head -c 280)"
fi

# ---------- 13. completed sport activity ----------
step "13. POST /api/v1/sport-activities is idempotent and preserves participation"
sport_op="smoke-sport-v1"
sport_payload="{\"date\":\"$session_date\",\"sport_id\":\"basketball\",\"sport_name\":\"Basketball\",\"duration_minutes\":60,\"notes\":\"Smoke pickup game\",\"operation_key\":\"$sport_op\"}"
sport_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $sport_op" \
  -d "$sport_payload")
sport_body=$(cat /tmp/smoke.body)
sport_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$sport_body" 2>/dev/null || true)
sport_replay_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $sport_op" \
  -d "$sport_payload")
sport_replay_body=$(cat /tmp/smoke.body)
sport_list_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/sport-activities?from=$session_date&to=$session_date" "${auth[@]}")
sport_list_body=$(cat /tmp/smoke.body)
sport_part_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/participation?from=$session_date&to=$session_date" "${auth[@]}")
sport_part_body=$(cat /tmp/smoke.body)
if [ "$sport_status" = "201" ] && [ "$sport_replay_status" = "201" ] && [ "$sport_list_status" = "200" ] && [ "$sport_part_status" = "200" ] && [ -n "$sport_id" ] && \
  python3 -c "import json,sys; created,replay,listed,participation=sys.argv[1:]; c=json.loads(created); r=json.loads(replay); l=json.loads(listed); p=json.loads(participation); assert c['id']==r['id'] and len(l)==1 and l[0]['id']==c['id'] and any(x['date']==c['date'] and x['participated'] for x in p['participation'])" "$sport_body" "$sport_replay_body" "$sport_list_body" "$sport_part_body" 2>/dev/null; then
  ok "sport replay returned one activity and participation=true for its date"
else
  bad "sport activity acceptance failed (create=$sport_status replay=$sport_replay_status list=$sport_list_status participation=$sport_part_status)" "$(echo "$sport_list_body" | head -c 280)"
fi

# ---------- 13b. imported health workout ----------
step "13b. POST /api/v1/sport-activities imports a health workout once (201 then 200)"
hk_uuid="SMOKE-HK-9B3C2E1A"
hk_payload_a="{\"date\":\"$session_date\",\"sport_id\":\"running\",\"sport_name\":\"Running\",\"duration_minutes\":42,\"source\":\"apple_health\",\"external_id\":\"$hk_uuid\",\"distance_meters\":8120,\"energy_kcal\":512,\"operation_key\":\"smoke-hk-a\"}"
hk_payload_b="{\"date\":\"$session_date\",\"sport_id\":\"running\",\"sport_name\":\"Running\",\"duration_minutes\":42,\"source\":\"apple_health\",\"external_id\":\"$hk_uuid\",\"distance_meters\":8120,\"energy_kcal\":512,\"operation_key\":\"smoke-hk-b\"}"
hk_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-hk-a" -d "$hk_payload_a")
hk_body=$(cat /tmp/smoke.body)
hk_again_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-hk-b" -d "$hk_payload_b")
hk_again_body=$(cat /tmp/smoke.body)
hk_bad_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-hk-c" \
  -d "{\"date\":\"$session_date\",\"sport_id\":\"tennis\",\"sport_name\":\"Tennis\",\"duration_minutes\":30,\"external_id\":\"$hk_uuid\",\"operation_key\":\"smoke-hk-c\"}")
hk_bad_body=$(cat /tmp/smoke.body)
if [ "$hk_status" = "201" ] && [ "$hk_again_status" = "200" ] && [ "$hk_bad_status" = "422" ] && \
  python3 -c "import json,sys; a,b,bad,manual=[json.loads(x) for x in sys.argv[1:]]; assert a['id']==b['id'] and a['source']=='apple_health' and a['external_id']=='$hk_uuid' and a['distance_meters']==8120 and a['energy_kcal']==512 and 'avg_heart_rate_bpm' not in a; assert bad['code']=='VALIDATION_ERROR' and bad['details']['field']=='external_id'; assert manual['source']=='manual' and 'external_id' not in manual" "$hk_body" "$hk_again_body" "$hk_bad_body" "$sport_body" 2>/dev/null; then
  ok "imported workout created once, re-import returned 200 with the same id, manual rows report source=manual"
else
  bad "imported workout acceptance failed (create=$hk_status again=$hk_again_status manual_with_external_id=$hk_bad_status)" "$(echo "$hk_again_body" | head -c 280)"
fi

# ---------- 13c. health export ledger ----------
step "13c. PUT /api/v1/health-exports records a finished workout once (201, 201 replay, 200, 409, list)"
if [ -n "${workout_id:-}" ]; then
  hx_uuid="SMOKE-HK-EXPORT-1"
  hx_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/health-exports/scheduled_workout/$workout_id" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-hx-a" \
    -d "{\"source\":\"apple_health\",\"external_id\":\"$hx_uuid\",\"operation_key\":\"smoke-hx-a\"}")
  hx_body=$(cat /tmp/smoke.body)
  # Verbatim retry: same operation key, same body, exported_at still omitted.
  # The server defaults exported_at to now, so this only replays 201 if the
  # idempotency hash is computed from the client-sent request.
  sleep 1
  hx_replay_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/health-exports/scheduled_workout/$workout_id" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-hx-a" \
    -d "{\"source\":\"apple_health\",\"external_id\":\"$hx_uuid\",\"operation_key\":\"smoke-hx-a\"}")
  hx_replay_body=$(cat /tmp/smoke.body)
  hx_again_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/health-exports/scheduled_workout/$workout_id" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-hx-b" \
    -d "{\"source\":\"apple_health\",\"external_id\":\"$hx_uuid\",\"operation_key\":\"smoke-hx-b\"}")
  hx_again_body=$(cat /tmp/smoke.body)
  hx_conflict_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/health-exports/scheduled_workout/$workout_id" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-hx-c" \
    -d "{\"source\":\"apple_health\",\"external_id\":\"SMOKE-HK-EXPORT-OTHER\",\"operation_key\":\"smoke-hx-c\"}")
  hx_conflict_body=$(cat /tmp/smoke.body)
  hx_missing_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/health-exports/day_log/00000000-0000-0000-0000-00000000dead" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-hx-d" \
    -d "{\"source\":\"apple_health\",\"external_id\":\"$hx_uuid\",\"operation_key\":\"smoke-hx-d\"}")
  hx_date=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['date'])" "$hx_body" 2>/dev/null || echo "$session_date")
  hx_list_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/health-exports?from=$hx_date&to=$hx_date" "${auth[@]}")
  hx_list_body=$(cat /tmp/smoke.body)
  if [ "$hx_status" = "201" ] && [ "$hx_replay_status" = "201" ] && [ "$hx_again_status" = "200" ] && [ "$hx_conflict_status" = "409" ] && [ "$hx_missing_status" = "404" ] && [ "$hx_list_status" = "200" ] && \
    python3 -c "import json,sys; a,r,b,c,l=[json.loads(x) for x in sys.argv[1:]]; assert a['id']==r['id']==b['id'] and a['external_id']=='$hx_uuid' and a['workout_id']=='$workout_id'; assert r['exported_at']==a['exported_at']; assert c['code']=='HEALTH_EXPORT_CONFLICT' and c['resource']['external_id']=='$hx_uuid'; assert any(e['id']==a['id'] for e in l['health_exports'])" "$hx_body" "$hx_replay_body" "$hx_again_body" "$hx_conflict_body" "$hx_list_body" 2>/dev/null; then
    ok "export recorded once, verbatim retry replayed 201, same identity under a new key returned 200, conflicted on a different identity, listed by date"
  else
    bad "health export acceptance failed (record=$hx_status replay=$hx_replay_status again=$hx_again_status conflict=$hx_conflict_status missing=$hx_missing_status list=$hx_list_status)" "$(echo "$hx_replay_body" | head -c 280)"
  fi
else
  bad "health export acceptance skipped: no finalized scheduled workout id from step 10" ""
fi

# ---------- 13d. planned sport session ----------
step "13d. a sport unit schedules a session that one linked sport activity completes"
read -r sport_week_from sport_week_to <<EOF
$(python3 - <<'PY'
from datetime import date, timedelta
today=date.today(); monday=today+timedelta(days=(7-today.weekday())%7)+timedelta(days=14)
print(monday.isoformat(), (monday+timedelta(days=6)).isoformat())
PY
)
EOF
sport_program_op="smoke-sport-program-v1"
sport_program_payload="{\"name\":\"Smoke mixed week\",\"primary_goal\":\"strength\",\"roadmap\":{},\"operation_key\":\"$sport_program_op\",\"workouts\":[{\"name\":\"Smoke lift\",\"sequence_position\":1,\"exercises\":[{\"name\":\"Air Squat\",\"category\":\"legs\",\"modality\":\"strength\",\"exercise_order\":1,\"target_sets\":2,\"target_reps\":10}]},{\"kind\":\"sport\",\"name\":\"Smoke ride\",\"sequence_position\":2,\"sport\":{\"sport_id\":\"cycling\",\"sport_name\":\"Cycling\",\"target_duration_minutes\":90,\"target_distance_meters\":40000}}]}"
sport_program_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/programs" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $sport_program_op" -d "$sport_program_payload")
sport_program_body=$(cat /tmp/smoke.body)
sport_program_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$sport_program_body" 2>/dev/null || true)
sport_program_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$sport_program_body" 2>/dev/null || true)
sport_unit_ok=$(python3 -c "import json,sys; w=json.loads(sys.argv[1])['workouts']; assert w[0]['kind']=='exercises' and 'sport' not in w[0]; assert w[1]['kind']=='sport' and w[1]['sport']['sport_id']=='cycling' and w[1]['sport']['target_distance_meters']==40000 and w[1]['exercises']==[]; print('yes')" "$sport_program_body" 2>/dev/null || echo no)
sport_bad_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/programs" \
  "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-sport-program-bad" \
  -d "{\"name\":\"Bad\",\"primary_goal\":\"strength\",\"roadmap\":{},\"operation_key\":\"smoke-sport-program-bad\",\"workouts\":[{\"kind\":\"sport\",\"name\":\"Spike\",\"sequence_position\":1,\"sport\":{\"sport_id\":\"volleyball\",\"sport_name\":\"Volleyball\",\"target_duration_minutes\":60,\"target_distance_meters\":5000}}]}")
sport_bad_body=$(cat /tmp/smoke.body)
if [ "$sport_program_status" = "201" ] && [ "$sport_unit_ok" = "yes" ] && [ "$sport_bad_status" = "422" ] && [ -n "$sport_program_id" ]; then
  sport_materialize_op="smoke-sport-materialize-v1"
  sm_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/schedule/materialize" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $sport_materialize_op" \
    -d "{\"program_id\":\"$sport_program_id\",\"from\":\"$sport_week_from\",\"to\":\"$sport_week_to\",\"operation_key\":\"$sport_materialize_op\",\"expected_revision\":$sport_program_revision}")
  sm_body=$(cat /tmp/smoke.body)
  sport_session_id=$(python3 -c "import json,sys; rows=[w for w in json.loads(sys.argv[1])['scheduled_workouts'] if w['kind']=='sport']; assert rows and rows[0]['status']=='planned' and rows[0]['required_sets']==[] and rows[0]['sport']['sport_id']=='cycling' and 'sport_activity_id' not in rows[0]['sport']; print(rows[0]['id'])" "$sm_body" 2>/dev/null || true)
  sport_session_revision=$(python3 -c "import json,sys; print([w for w in json.loads(sys.argv[1])['scheduled_workouts'] if w['kind']=='sport'][0]['revision'])" "$sm_body" 2>/dev/null || true)
  if [ "$sm_status" != "201" ] || [ -z "$sport_session_id" ]; then
    bad "sport unit did not materialize as a planned sport session (status=$sm_status)" "$(echo "$sm_body" | head -c 280)"
  else
    # The session date is in the future, so record it as performed today (performed date may differ).
    record_op="smoke-sport-record-v1"
    record_payload="{\"date\":\"$session_date\",\"sport_id\":\"cycling\",\"sport_name\":\"Cycling\",\"duration_minutes\":95,\"distance_meters\":42000,\"perceived_effort\":7,\"operation_key\":\"$record_op\",\"scheduled_workout_id\":\"$sport_session_id\"}"
    record_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities" \
      "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $record_op" -d "$record_payload")
    record_body=$(cat /tmp/smoke.body)
    record_replay_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities" \
      "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $record_op" -d "$record_payload")
    record_replay_body=$(cat /tmp/smoke.body)
    record_again_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities" \
      "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-sport-record-v2" \
      -d "{\"date\":\"$session_date\",\"sport_id\":\"cycling\",\"sport_name\":\"Cycling\",\"duration_minutes\":60,\"operation_key\":\"smoke-sport-record-v2\",\"scheduled_workout_id\":\"$sport_session_id\"}")
    record_again_body=$(cat /tmp/smoke.body)
    sport_sched_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/schedule?from=$sport_week_from&to=$sport_week_to" "${auth[@]}")
    sport_sched_body=$(cat /tmp/smoke.body)
    sport_extra_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$sport_session_id/extra-sets" \
      "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-sport-extra" \
      -d "{\"operation_key\":\"smoke-sport-extra\",\"expected_revision\":$((sport_session_revision+1)),\"exercise_id\":null,\"exercise_name\":\"Push-Up\",\"exercise_category\":\"push\",\"exercise_modality\":\"strength\",\"set_index\":1,\"actual_reps\":10,\"completed\":true}")
    sport_extra_body=$(cat /tmp/smoke.body)
    if [ "$record_status" = "201" ] && [ "$record_replay_status" = "201" ] && [ "$record_again_status" = "409" ] && [ "$sport_sched_status" = "200" ] && [ "$sport_extra_status" = "422" ] && \
      python3 -c "import json,sys; a,r,c,s,e=[json.loads(x) for x in sys.argv[1:]]; sid='$sport_session_id'; assert a['id']==r['id'] and a['scheduled_workout_id']==sid and a['distance_meters']==42000 and a['perceived_effort']==7; assert c['code']=='SPORT_SESSION_CONFLICT' and c['details']['status']=='completed' and c['resource']['id']==sid; row=[w for w in s['scheduled_workouts'] if w['id']==sid][0]; assert row['status']=='completed' and row['sport']['sport_activity_id']==a['id'] and row['sport']['performed_date']==a['date'] and row['required_sets']==[]; assert e['code']=='VALIDATION_ERROR' and e['details']['field']=='kind'" "$record_body" "$record_replay_body" "$record_again_body" "$sport_sched_body" "$sport_extra_body" 2>/dev/null; then
      ok "sport session planned, completed once by a linked activity, replayed, conflicted on a second record, and refused an extra set"
      # 13e. contract 1.3.0: unlink reopens the future session; linking the same activity back completes it again.
      record_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$record_body" 2>/dev/null || true)
      done_revision=$(python3 -c "import json,sys; print([w for w in json.loads(sys.argv[1])['scheduled_workouts'] if w['id']=='$sport_session_id'][0]['revision'])" "$sport_sched_body" 2>/dev/null || true)
      unlink_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities/$record_id/unlink" \
        "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-sport-unlink" \
        -d "{\"expected_revision\":$done_revision,\"operation_key\":\"smoke-sport-unlink\"}")
      unlink_body=$(cat /tmp/smoke.body)
      reopened_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['scheduled_workout']['revision'])" "$unlink_body" 2>/dev/null || echo 0)
      unlink_again_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities/$record_id/unlink" \
        "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-sport-unlink-2" \
        -d "{\"expected_revision\":$reopened_revision,\"operation_key\":\"smoke-sport-unlink-2\"}")
      unlink_again_body=$(cat /tmp/smoke.body)
      relink_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/sport-activities/$record_id/link" \
        "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-sport-link" \
        -d "{\"scheduled_workout_id\":\"$sport_session_id\",\"expected_revision\":$reopened_revision,\"operation_key\":\"smoke-sport-link\"}")
      relink_body=$(cat /tmp/smoke.body)
      if [ "$unlink_status" = "200" ] && [ "$unlink_again_status" = "409" ] && [ "$relink_status" = "200" ] && \
        python3 -c "import json,sys; u,a,l=[json.loads(x) for x in sys.argv[1:]]; sid='$sport_session_id'; assert 'scheduled_workout_id' not in u['sport_activity'] and u['scheduled_workout']['status']=='planned' and u['scheduled_workout'].get('finalized_at') is None and 'sport_activity_id' not in u['scheduled_workout']['sport']; assert a['code']=='SPORT_SESSION_CONFLICT' and a['details']['reason']=='not_linked'; assert l['sport_activity']['scheduled_workout_id']==sid and l['scheduled_workout']['status']=='completed' and l['scheduled_workout']['sport']['sport_activity_id']==l['sport_activity']['id']" "$unlink_body" "$unlink_again_body" "$relink_body" 2>/dev/null; then
        ok "unlink reopened the session, a second unlink conflicted with not_linked, and link-existing completed it again"
      else
        bad "sport session link repair failed (unlink=$unlink_status again=$unlink_again_status relink=$relink_status)" "$(echo "$unlink_body $unlink_again_body $relink_body" | head -c 320)"
      fi
    else
      bad "planned sport session acceptance failed (record=$record_status replay=$record_replay_status again=$record_again_status schedule=$sport_sched_status extra=$sport_extra_status)" "$(echo "$record_again_body $sport_extra_body" | head -c 320)"
    fi
  fi
else
  bad "sport unit program acceptance failed (create=$sport_program_status units=$sport_unit_ok distance-on-volleyball=$sport_bad_status)" "$(echo "$sport_program_body $sport_bad_body" | head -c 320)"
fi

# ---------- 13f. nutrition days and target ----------
step "13f. Nutrition: target 201/200, day 201/replay/Health-ignored, Health day 201 → manual takeover → ignored, clear 204, range, future 422"
nu_yesterday=$(python3 -c "from datetime import datetime,timezone,timedelta; print((datetime.now(timezone.utc).date()-timedelta(days=1)).isoformat())")
nu_before=$(python3 -c "from datetime import datetime,timezone,timedelta; print((datetime.now(timezone.utc).date()-timedelta(days=2)).isoformat())")
nu_tomorrow=$(python3 -c "from datetime import datetime,timezone,timedelta; print((datetime.now(timezone.utc).date()+timedelta(days=1)).isoformat())")
nu_today=$(python3 -c "from datetime import datetime,timezone; print(datetime.now(timezone.utc).date().isoformat())")
nu_put() { curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API$1" "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $2" -d "$3"; }
nt_status=$(nu_put "/api/v1/nutrition/target" "smoke-nt-a" "{\"protein_g\":150,\"energy_kcal\":2200,\"expected_revision\":0,\"operation_key\":\"smoke-nt-a\"}")
nt_body=$(cat /tmp/smoke.body)
nt_rev=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$nt_body" 2>/dev/null || echo 0)
nt_same_status=$(nu_put "/api/v1/nutrition/target" "smoke-nt-b" "{\"protein_g\":150,\"energy_kcal\":2200,\"expected_revision\":$nt_rev,\"operation_key\":\"smoke-nt-b\"}")
nt_same_body=$(cat /tmp/smoke.body)
nt_get_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/nutrition/target" "${auth[@]}")
nt_get_body=$(cat /tmp/smoke.body)
nd_status=$(nu_put "/api/v1/nutrition/days/$nu_yesterday" "smoke-nd-a" "{\"protein_g\":162.5,\"energy_kcal\":2150,\"expected_revision\":0,\"operation_key\":\"smoke-nd-a\"}")
nd_body=$(cat /tmp/smoke.body)
nd_replay_status=$(nu_put "/api/v1/nutrition/days/$nu_yesterday" "smoke-nd-a" "{\"protein_g\":162.5,\"energy_kcal\":2150,\"expected_revision\":0,\"operation_key\":\"smoke-nd-a\"}")
nd_replay_body=$(cat /tmp/smoke.body)
nd_stale_status=$(nu_put "/api/v1/nutrition/days/$nu_yesterday" "smoke-nd-stale" "{\"protein_g\":1,\"expected_revision\":0,\"operation_key\":\"smoke-nd-stale\"}")
nd_stale_body=$(cat /tmp/smoke.body)
nd_hk_over_manual_status=$(nu_put "/api/v1/nutrition/days/$nu_yesterday" "smoke-nd-hk1" "{\"source\":\"apple_health\",\"protein_g\":99,\"operation_key\":\"smoke-nd-hk1\"}")
nd_hk_over_manual_body=$(cat /tmp/smoke.body)
nd_hk_status=$(nu_put "/api/v1/nutrition/days/$nu_before" "smoke-nd-hk2" "{\"source\":\"apple_health\",\"protein_g\":120,\"carbs_g\":200.5,\"operation_key\":\"smoke-nd-hk2\"}")
nd_hk_body=$(cat /tmp/smoke.body)
nd_hk_replace_status=$(nu_put "/api/v1/nutrition/days/$nu_before" "smoke-nd-hk2b" "{\"source\":\"apple_health\",\"protein_g\":125,\"operation_key\":\"smoke-nd-hk2b\"}")
nd_hk_replace_body=$(cat /tmp/smoke.body)
nd_takeover_status=$(nu_put "/api/v1/nutrition/days/$nu_before" "smoke-nd-m2" "{\"protein_g\":130,\"expected_revision\":2,\"operation_key\":\"smoke-nd-m2\"}")
nd_takeover_body=$(cat /tmp/smoke.body)
nd_hk_again_status=$(nu_put "/api/v1/nutrition/days/$nu_before" "smoke-nd-hk3" "{\"source\":\"apple_health\",\"protein_g\":121,\"operation_key\":\"smoke-nd-hk3\"}")
nd_hk_again_body=$(cat /tmp/smoke.body)
nd_clear_status=$(nu_put "/api/v1/nutrition/days/$nu_yesterday" "smoke-nd-clear" "{\"expected_revision\":1,\"operation_key\":\"smoke-nd-clear\"}")
nd_clear_replay_status=$(nu_put "/api/v1/nutrition/days/$nu_yesterday" "smoke-nd-clear" "{\"expected_revision\":1,\"operation_key\":\"smoke-nd-clear\"}")
nd_clear_none_status=$(nu_put "/api/v1/nutrition/days/$nu_yesterday" "smoke-nd-clear2" "{\"expected_revision\":0,\"operation_key\":\"smoke-nd-clear2\"}")
nd_gone_status=$(nu_put "/api/v1/nutrition/days/$nu_yesterday" "smoke-nd-gone" "{\"protein_g\":5,\"expected_revision\":1,\"operation_key\":\"smoke-nd-gone\"}")
nd_gone_body=$(cat /tmp/smoke.body)
# An older target is inserted while the revision is checked against the current (today's) target.
nt_old_status=$(nu_put "/api/v1/nutrition/target" "smoke-nt-old" "{\"effective_from\":\"$nu_before\",\"protein_g\":140,\"expected_revision\":1,\"operation_key\":\"smoke-nt-old\"}")
nt_old_body=$(cat /tmp/smoke.body)
nd_range_old_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/nutrition/days?from=$nu_yesterday&to=$nu_yesterday" "${auth[@]}")
nd_range_old_body=$(cat /tmp/smoke.body)
nd_range_today_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/nutrition/days?from=$nu_today&to=$nu_today" "${auth[@]}")
nd_range_today_body=$(cat /tmp/smoke.body)
nd_range_status=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/nutrition/days?from=$nu_before&to=$nu_today" "${auth[@]}")
nd_range_body=$(cat /tmp/smoke.body)
nd_future_status=$(nu_put "/api/v1/nutrition/days/$nu_tomorrow" "smoke-nd-future" "{\"protein_g\":1,\"expected_revision\":0,\"operation_key\":\"smoke-nd-future\"}")
if [ "$nt_status" = "201" ] && [ "$nt_same_status" = "200" ] && [ "$nt_get_status" = "200" ] && \
   [ "$nd_status" = "201" ] && [ "$nd_replay_status" = "201" ] && [ "$nd_stale_status" = "409" ] && [ "$nd_hk_over_manual_status" = "200" ] && \
   [ "$nd_hk_status" = "201" ] && [ "$nd_takeover_status" = "200" ] && [ "$nd_hk_again_status" = "200" ] && [ "$nd_clear_status" = "204" ] && \
   [ "$nd_range_status" = "200" ] && [ "$nd_future_status" = "422" ] && \
   [ "$nd_hk_replace_status" = "200" ] && [ "$nd_clear_replay_status" = "204" ] && [ "$nd_clear_none_status" = "204" ] && [ "$nd_gone_status" = "409" ] && \
   [ "$nt_old_status" = "201" ] && [ "$nd_range_old_status" = "200" ] && [ "$nd_range_today_status" = "200" ] && \
   python3 -c "
import json,sys
t,ts,tg,d,dr,st,hm,hk,tk,ha,rg,hr,gn,to,ro,rt=[json.loads(x) for x in sys.argv[1:]]
assert hr['id']==hk['id'] and hr['source']=='apple_health' and hr['protein_g']==125 and hr['revision']==2 and 'carbs_g' not in hr
assert gn['code']=='REVISION_CONFLICT' and gn['details']['actual_revision']==0 and 'resource' not in gn
assert to['effective_from']=='$nu_before' and to['protein_g']==140 and to['revision']==1 and to['id']!=t['id']
assert ro['days']==[] and [x['id'] for x in ro['targets']]==[to['id']]
assert [x['id'] for x in rt['targets']]==[t['id']]
assert t['protein_g']==150 and t['energy_kcal']==2200 and t['carbs_g'] is None and t['revision']==1
assert ts['id']==t['id'] and ts['revision']==1 and tg['id']==t['id']
assert d['protein_g']==162.5 and d['energy_kcal']==2150 and d['source']=='manual' and d['revision']==1 and 'carbs_g' not in d
assert dr['id']==d['id'] and dr['logged_at']==d['logged_at']
assert st['code']=='REVISION_CONFLICT' and st['details']['actual_revision']==1 and st['resource']['id']==d['id']
assert hm['id']==d['id'] and hm['protein_g']==162.5 and hm['source']=='manual'
assert hk['source']=='apple_health' and hk['protein_g']==120 and hk['carbs_g']==200.5 and hk['revision']==1
assert tk['id']==hk['id'] and tk['source']=='manual' and tk['protein_g']==130 and tk['revision']==3 and 'carbs_g' not in tk
assert ha['id']==hk['id'] and ha['protein_g']==130 and ha['source']=='manual'
assert [x['date'] for x in rg['days']]==['$nu_before'] and [x['id'] for x in rg['targets']]==[to['id'],t['id']]
" "$nt_body" "$nt_same_body" "$nt_get_body" "$nd_body" "$nd_replay_body" "$nd_stale_body" "$nd_hk_over_manual_body" "$nd_hk_body" "$nd_takeover_body" "$nd_hk_again_body" "$nd_range_body" "$nd_hk_replace_body" "$nd_gone_body" "$nt_old_body" "$nd_range_old_body" "$nd_range_today_body" 2>/dev/null; then
  ok "nutrition target saved, unchanged, read back, and an older target inserted under the current revision; day created, replayed, stale-conflicted, protected from a Health import; Health day replaced by Health, taken over by the athlete, then protected; cleared 204 (replay and no-row too), gone-row conflict omits resource; ranges carry the target in effect and never duplicate; future date rejected"
else
  bad "nutrition acceptance failed (target=$nt_status same=$nt_same_status get=$nt_get_status day=$nd_status replay=$nd_replay_status stale=$nd_stale_status hk_over_manual=$nd_hk_over_manual_status hk=$nd_hk_status hk_replace=$nd_hk_replace_status takeover=$nd_takeover_status hk_again=$nd_hk_again_status clear=$nd_clear_status clear_replay=$nd_clear_replay_status clear_none=$nd_clear_none_status gone=$nd_gone_status old_target=$nt_old_status range=$nd_range_status range_old=$nd_range_old_status range_today=$nd_range_today_status future=$nd_future_status)" "$(echo "$nd_range_old_body $nd_gone_body" | head -c 400)"
fi

# ---------- 13g. food logging ----------
step "13g. Food logging: private food 201 → barcode → entry 201 (replay) → day source foods → typed PUT 409 → Health ignored → update/stale → recents → delete → emptied day reads null"
fl_put() { curl -s -o /tmp/smoke.body -w "%{http_code}" -X "$1" "$API$2" "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $3" -d "$4"; }
fl_get() { curl -s -o /tmp/smoke.body -w "%{http_code}" "$API$1" "${auth[@]}"; }
fl_del() { curl -s -o /tmp/smoke.body -w "%{http_code}" -X DELETE "$API$1" "${auth[@]}" -H "Idempotency-Key: $2"; }
fl_today=$(python3 -c "from datetime import datetime,timezone; print(datetime.now(timezone.utc).date().isoformat())")
fl_food_status=$(fl_put POST "/api/v1/nutrition/foods" "smoke-uf-a" '{"name":"Smoke whey","brand":"Smoke","barcode":"096619123452","serving":{"label":"1 scoop","grams":30},"energy_kcal":120,"protein_g":24,"carbs_g":3,"fat_g":1.5,"operation_key":"smoke-uf-a"}')
fl_food_body=$(cat /tmp/smoke.body)
fl_food_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$fl_food_body" 2>/dev/null || echo none)
fl_bad_code_status=$(fl_get "/api/v1/nutrition/foods/barcode/096619123456")
fl_barcode_status=$(fl_get "/api/v1/nutrition/foods/barcode/0%2096619%2012345%202")
fl_barcode_body=$(cat /tmp/smoke.body)
fl_search_status=$(fl_get "/api/v1/nutrition/foods/search?q=smoke%20whey")
fl_search_body=$(cat /tmp/smoke.body)
fl_short_q_status=$(fl_get "/api/v1/nutrition/foods/search?q=s")
fl_entry_status=$(fl_put POST "/api/v1/nutrition/days/$fl_today/entries" "smoke-fe-a" "{\"food\":{\"id\":\"$fl_food_id\"},\"serving\":{\"label\":\"1 scoop\",\"grams\":30},\"amount\":2,\"operation_key\":\"smoke-fe-a\"}")
fl_entry_body=$(cat /tmp/smoke.body)
fl_entry_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['entry']['id'])" "$fl_entry_body" 2>/dev/null || echo none)
fl_replay_status=$(fl_put POST "/api/v1/nutrition/days/$fl_today/entries" "smoke-fe-a" "{\"food\":{\"id\":\"$fl_food_id\"},\"serving\":{\"label\":\"1 scoop\",\"grams\":30},\"amount\":2,\"operation_key\":\"smoke-fe-a\"}")
fl_replay_body=$(cat /tmp/smoke.body)
fl_mismatch_status=$(fl_put POST "/api/v1/nutrition/days/$fl_today/entries" "smoke-fe-a" "{\"food\":{\"id\":\"$fl_food_id\"},\"serving\":{\"label\":\"1 scoop\",\"grams\":30},\"amount\":3,\"operation_key\":\"smoke-fe-a\"}")
fl_quick_status=$(fl_put POST "/api/v1/nutrition/days/$fl_today/entries" "smoke-fe-q" '{"quick_add":{"protein_g":10.5,"energy_kcal":50},"operation_key":"smoke-fe-q"}')
fl_quick_body=$(cat /tmp/smoke.body)
fl_quick_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['entry']['id'])" "$fl_quick_body" 2>/dev/null || echo none)
fl_typed_status=$(fl_put PUT "/api/v1/nutrition/days/$fl_today" "smoke-fe-typed" '{"protein_g":1,"expected_revision":2,"operation_key":"smoke-fe-typed"}')
fl_typed_body=$(cat /tmp/smoke.body)
fl_health_status=$(fl_put PUT "/api/v1/nutrition/days/$fl_today" "smoke-fe-hk" '{"source":"apple_health","protein_g":99,"operation_key":"smoke-fe-hk"}')
fl_health_body=$(cat /tmp/smoke.body)
fl_update_status=$(fl_put PUT "/api/v1/nutrition/entries/$fl_entry_id" "smoke-fe-u" '{"serving":{"label":"1 scoop","grams":30},"amount":1,"expected_revision":1,"operation_key":"smoke-fe-u"}')
fl_update_body=$(cat /tmp/smoke.body)
fl_stale_status=$(fl_put PUT "/api/v1/nutrition/entries/$fl_entry_id" "smoke-fe-s" '{"serving":{"label":"1 scoop","grams":30},"amount":1,"expected_revision":1,"operation_key":"smoke-fe-s"}')
fl_stale_body=$(cat /tmp/smoke.body)
fl_list_status=$(fl_get "/api/v1/nutrition/days/$fl_today/entries")
fl_list_body=$(cat /tmp/smoke.body)
fl_recent_status=$(fl_get "/api/v1/nutrition/foods/recent")
fl_recent_body=$(cat /tmp/smoke.body)
fl_del_quick_status=$(fl_del "/api/v1/nutrition/entries/$fl_quick_id?expected_revision=1" "smoke-fe-dq")
fl_del_quick_body=$(cat /tmp/smoke.body)
fl_del_status=$(fl_del "/api/v1/nutrition/entries/$fl_entry_id?expected_revision=2" "smoke-fe-d")
fl_del_body=$(cat /tmp/smoke.body)
fl_archive_status=$(fl_del "/api/v1/nutrition/foods/$fl_food_id?expected_revision=1" "smoke-uf-d")
fl_after_archive_status=$(fl_get "/api/v1/nutrition/foods/barcode/096619123452")
fl_unknown_food_status=$(fl_put PUT "/api/v1/nutrition/foods/00000000-0000-0000-0000-000000000000" "smoke-uf-x" '{"name":"x","serving":{"label":"1","grams":1},"energy_kcal":1,"protein_g":1,"carbs_g":1,"fat_g":1,"expected_revision":1,"operation_key":"smoke-uf-x"}')
fl_after_delete_status=$(fl_get "/api/v1/nutrition/days/$fl_today/entries")
fl_after_delete_body=$(cat /tmp/smoke.body)
if [ "$fl_food_status" = "201" ] && [ "$fl_bad_code_status" = "422" ] && [ "$fl_barcode_status" = "200" ] && [ "$fl_search_status" = "200" ] && [ "$fl_short_q_status" = "422" ] && \
   [ "$fl_entry_status" = "201" ] && [ "$fl_replay_status" = "201" ] && [ "$fl_mismatch_status" = "409" ] && [ "$fl_quick_status" = "201" ] && \
   [ "$fl_typed_status" = "409" ] && [ "$fl_health_status" = "200" ] && [ "$fl_update_status" = "200" ] && [ "$fl_stale_status" = "409" ] && \
   [ "$fl_list_status" = "200" ] && [ "$fl_recent_status" = "200" ] && [ "$fl_del_quick_status" = "200" ] && [ "$fl_del_status" = "200" ] && \
   [ "$fl_archive_status" = "204" ] && [ "$fl_after_archive_status" = "404" ] && [ "$fl_unknown_food_status" = "404" ] && [ "$fl_after_delete_status" = "200" ] && \
   python3 -c "
import json,sys
food,bc,sr,en,rp,qk,ty,hk,up,st,ls,rc,dq,dl,ol=[json.loads(x) for x in sys.argv[1:]]
assert food['source']=='own' and food['barcode']=='0096619123452' and food['revision']==1 and food['per_100g']=={'energy_kcal':400,'protein_g':80,'carbs_g':10,'fat_g':5}
assert [s['grams'] for s in food['servings']]==[30,100]
assert bc['id']==food['id'], 'own food wins the barcode chain'
assert any(f['id']==food['id'] for f in sr['foods']) and sr['foods'][0]['id']==food['id']
assert en['converted']==[] and en['entry']['kind']=='food' and en['entry']['energy_kcal']==240 and en['entry']['protein_g']==48 and en['entry']['amount']==2
assert en['day']['source']=='foods' and en['day']['protein_g']==48 and en['day']['energy_kcal']==240 and en['day']['revision']==1
assert rp['entry']['id']==en['entry']['id'] and rp['day']['revision']==1, 'verbatim replay returns the original body'
assert qk['day']['protein_g']==58.5 and qk['day']['energy_kcal']==290 and qk['day']['revision']==2
assert ty['code']=='DAY_HAS_FOOD_ENTRIES' and ty['resource']['source']=='foods'
assert hk['source']=='foods' and hk['protein_g']==58.5, 'Health save ignored over a foods day'
assert up['entry']['revision']==2 and up['entry']['protein_g']==24 and up['day']['protein_g']==34.5 and up['day']['revision']==3
assert st['code']=='REVISION_CONFLICT' and st['details']['actual_revision']==2 and st['resource']['id']==en['entry']['id']
assert [e['kind'] for e in ls['entries']]==['food','quick_add'] and ls['day']['protein_g']==34.5
assert len(rc['foods'])==1 and rc['foods'][0]['food']['id']==food['id'] and rc['foods'][0]['last_amount']==1 and rc['foods'][0]['times_logged']==1
assert dq['day']['protein_g']==24 and dq['day']['revision']==4
assert dl['day'] is None, 'removing the last entry removes the day'
assert ol['day'] is None and ol['entries']==[], 'day and entries gone after the last delete'
" "$fl_food_body" "$fl_barcode_body" "$fl_search_body" "$fl_entry_body" "$fl_replay_body" "$fl_quick_body" "$fl_typed_body" "$fl_health_body" "$fl_update_body" "$fl_stale_body" "$fl_list_body" "$fl_recent_body" "$fl_del_quick_body" "$fl_del_body" "$fl_after_delete_body" 2>/dev/null; then
  ok "food logging: private food created and found first by barcode and search; bad check digit and short query rejected; entry created with snapshot and day source foods, replayed verbatim, mismatched key conflicted; quick add summed; typed PUT refused with DAY_HAS_FOOD_ENTRIES; Health save ignored; update recomputed and bumped the day; stale revision conflicted with the entry; recents listed the food; deletes recomputed and removed the day; archive hid the food; unknown food id is the shared 404; the emptied day reads back as null"
else
  bad "food logging acceptance failed (food=$fl_food_status bad_code=$fl_bad_code_status barcode=$fl_barcode_status search=$fl_search_status short_q=$fl_short_q_status entry=$fl_entry_status replay=$fl_replay_status mismatch=$fl_mismatch_status quick=$fl_quick_status typed=$fl_typed_status health=$fl_health_status update=$fl_update_status stale=$fl_stale_status list=$fl_list_status recent=$fl_recent_status del_quick=$fl_del_quick_status del=$fl_del_status archive=$fl_archive_status after_archive=$fl_after_archive_status unknown_food=$fl_unknown_food_status after_delete=$fl_after_delete_status)" "$(echo "$fl_entry_body $fl_typed_body" | head -c 500)"
fi

# ---------- 14. account deletion (LAST: wipes the smoke user) ----------
step "14. DELETE /api/v1/account returns 204 and clears the user's data"
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X DELETE "$API/api/v1/account" "${auth[@]}")
if [ "$resp" = "204" ]; then
  # After deletion the user's templates must be gone (empty list or null).
  resp2=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/templates" "${auth[@]}")
  body=$(cat /tmp/smoke.body)
  if [ "$resp2" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d in (None, [])" "$body" 2>/dev/null; then
    ok "account deleted; templates now empty (this also cleans up smoke data)"
  else
    bad "post-delete templates expected empty, got $resp2" "$(echo "$body" | head -c 200)"
  fi
else
  bad "DELETE /account expected 204, got $resp" "$(cat /tmp/smoke.body | head -c 200)"
fi

# ---------- summary ----------
printf "\n\033[1m%d passed, %d failed\033[0m\n" "$pass" "$fail"
[ "$fail" -eq 0 ] || exit 1
