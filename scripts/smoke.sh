#!/usr/bin/env bash
# scripts/smoke.sh — 5-second contract smoke test for the local API.
#
# What it asserts:
#   1. /health is 200
#   2. PUT /api/v1/settings rejects partial body with 400 "invalid settings"
#   3. PUT /api/v1/settings accepts weekly_goal=1 (regression for the
#      bug where validator required min=3 and broke onboarding)
#   4. GET /api/v1/stats/distribution returns either an array or `null`
#      (client must coerce; this asserts the documented contract)
#
# Requirements:
#   - API container running on :8080 (see gym-pulse-dev-loop skill)
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

# ---------- 2. settings: partial body must 400 ----------
step "2. PUT /api/v1/settings with partial body should 400"
resp=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT "$API/api/v1/settings" \
  "${auth[@]}" -H "Content-Type: application/json" -d '{"weekly_goal":5}')
body=$(cat /tmp/smoke.body)
case "$resp" in
  422) ok "partial body rejected with 422 VALIDATION_ERROR (body: $body)" ;;
  400) ok "partial body rejected with 400 (body: $body)" ;;
  401) bad "got 401 — JWKS is still enabled in the container; see header comment" "$body" ;;
  200) bad "partial body was ACCEPTED — validator no longer requires weight_unit?" "$body" ;;
  *)   bad "unexpected status $resp" "$body" ;;
esac

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

# ---------- summary ----------
printf "\n\033[1m%d passed, %d failed\033[0m\n" "$pass" "$fail"
[ "$fail" -eq 0 ] || exit 1
