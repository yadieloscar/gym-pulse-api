#!/usr/bin/env bash
# scripts/smoke-toggle.sh — run smoke.sh with JWKS temporarily disabled.
#
# This is the one-command path for local contract testing. It:
#   1. Reads the current SUPABASE_JWKS_URL from the running container
#   2. Recreates the API container with JWKS unset (HMAC fallback path)
#   3. Runs scripts/smoke.sh
#   4. Restores the original JWKS env regardless of pass/fail
#
# Takes ~10s end-to-end because of the container recreate.

set -uo pipefail
here="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
compose_dir="$(dirname "$(dirname "$here")")"   # gym-pulse/

jwks_was=$(docker exec gym-pulse-api-1 printenv SUPABASE_JWKS_URL 2>/dev/null || echo "")

restore() {
  echo
  echo "→ restoring container without smoke override"
  ( cd "$compose_dir" && docker compose up -d --no-build --force-recreate api >/dev/null )
}
trap restore EXIT

echo "→ disabling JWKS via docker-compose.smoke.yml override"
( cd "$compose_dir" && docker compose -f docker-compose.yml -f docker-compose.smoke.yml up -d --no-build --force-recreate api >/dev/null )

# wait for healthy
code=""
for _ in $(seq 1 30); do
  code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health || true)
  [ "$code" = "200" ] && break
  sleep 1
done
if [ "$code" != "200" ]; then
  echo "api did not come back healthy; recent logs follow" >&2
  ( cd "$compose_dir" && docker compose logs --tail=120 api ) >&2
  exit 1
fi

# Mirror the CI fixture so local writes that reference auth.users exercise the
# real foreign-key path instead of failing because the development database is
# empty. smoke.sh deletes this user and all owned data in its final step.
( cd "$compose_dir" && docker compose exec -T db \
  psql -U gympulse -d gympulse -v ON_ERROR_STOP=1 \
  -c "INSERT INTO auth.users (id) VALUES ('00000000-0000-0000-0000-000000000001') ON CONFLICT (id) DO NOTHING;" >/dev/null )

"$here/smoke.sh"
