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
#   5. goal profile → starter copy → dated scheduled workout → required and
#      extra sets → incomplete outcome → participation remains separate
#   6. notes-only and overrides-only day-log edits preserve performed sets
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
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/plan/weekly" \
  "${auth[@]}" -H "Content-Type: application/json" \
  -d '{"days":[{"weekday":1,"rest":true},{"weekday":3,"rest":true}]}')
body=$(cat /tmp/smoke.body)
if [ "$resp" != "200" ]; then
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
  -d "{\"primary_goal\":\"strength\",\"available_days\":[1,4],\"usual_activity\":\"moderate\",\"experience\":\"beginner\",\"equipment\":[\"bodyweight\"],\"session_duration_minutes\":45,\"timezone\":\"UTC\",\"preferences\":{},\"expected_revision\":0}")
body=$(cat /tmp/smoke.body)
if [ "$resp" != "200" ] || ! python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['primary_goal']=='strength' and d['revision']>=1" "$body" 2>/dev/null; then
  bad "training profile expected 200 with primary_goal=strength, got $resp" "$(echo "$body" | head -c 240)"
else
  ok "training profile persisted primary_goal=strength"

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
      workout_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['scheduled_workouts'][0]['revision'])" "$body" 2>/dev/null || true)
      if [ "$resp" != "201" ] || [ -z "$workout_id" ] || [ -z "$set_id" ]; then
        bad "schedule materialize expected dated scheduled_workout, got $resp" "$(echo "$body" | head -c 280)"
      else
        set_op="smoke-check-required-v1"
        resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/scheduled-workouts/$workout_id/sets/$set_id" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $set_op" \
          -d "{\"operation_key\":\"$set_op\",\"expected_revision\":$workout_revision,\"actual_reps\":10,\"completed\":true}")
        body=$(cat /tmp/smoke.body)
        workout_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$body" 2>/dev/null || true)
        extra_op="smoke-extra-set-v1"
        resp_extra=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/extra-sets" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $extra_op" \
          -d "{\"operation_key\":\"$extra_op\",\"expected_revision\":$workout_revision,\"exercise_name\":\"Air Squat\",\"exercise_category\":\"legs\",\"exercise_modality\":\"strength\",\"set_index\":1,\"actual_reps\":20,\"completed\":true}")
        body=$(cat /tmp/smoke.body)
        workout_revision=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['revision'])" "$body" 2>/dev/null || true)
        complete_op="smoke-complete-incomplete-v1"
        resp_complete=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/scheduled-workouts/$workout_id/complete" \
          "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $complete_op" \
          -d "{\"operation_key\":\"$complete_op\",\"expected_revision\":$workout_revision}")
        body=$(cat /tmp/smoke.body)
        status=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['status'])" "$body" 2>/dev/null || true)
        resp_part=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$API/api/v1/participation?from=$week_from&to=$week_to" "${auth[@]}")
        participation_body=$(cat /tmp/smoke.body)
        if [ "$resp" = "200" ] && [ "$resp_extra" = "201" ] && [ "$resp_complete" = "200" ] && [ "$status" = "incomplete" ] && [ "$resp_part" = "200" ] && python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert any(x['participated'] for x in d['participation'])" "$participation_body" 2>/dev/null; then
          ok "required + extra sets produced incomplete scheduled workout and separate participation=true"
        else
          bad "goal training lifecycle failed (set=$resp extra=$resp_extra complete=$resp_complete status=$status participation=$resp_part)" "$(echo "$participation_body" | head -c 280)"
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
  resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST "$API/api/v1/workout-sessions" \
    "${auth[@]}" -H "Content-Type: application/json" -H "Idempotency-Key: $session_op" \
    -d "{\"scheduled_workout_id\":null,\"date\":\"$session_date\",\"name\":\"Off-plan $suffix\",\"operation_key\":\"$session_op\",\"expected_revision\":0}")
  body=$(cat /tmp/smoke.body)
  [ "$resp" = "201" ] && session_ids="$session_ids $(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$body" 2>/dev/null || true)"
done
if [ "$(echo "$session_ids" | xargs -n1 | sort -u | wc -l | tr -d ' ')" = "2" ]; then
  ok "multiple off-plan session UUIDs coexist on one date"
else
  bad "expected two distinct off-plan workout session IDs" "$session_ids"
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

# ---------- 13. account deletion (LAST: wipes the smoke user) ----------
step "13. DELETE /api/v1/account returns 204 and clears the user's data"
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
