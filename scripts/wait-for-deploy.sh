#!/usr/bin/env bash
# Waits until a deployed API serves the expected build and is ready.
#
# Usage: scripts/wait-for-deploy.sh <base-url> <expected-version> [timeout-seconds]
#
# Succeeds once GET /health reports "version":"<expected-version>" and GET
# /ready returns 200. Fails after the timeout (default 900s), which is what
# happens when the pre-deploy migration fails and the previous build keeps
# serving.
set -euo pipefail

base_url="${1:?usage: wait-for-deploy.sh <base-url> <expected-version> [timeout-seconds]}"
expected="${2:?usage: wait-for-deploy.sh <base-url> <expected-version> [timeout-seconds]}"
timeout="${3:-900}"
interval="${WAIT_INTERVAL_SECONDS:-10}"
base_url="${base_url%/}"

deadline=$((SECONDS + timeout))
serving=""
while ((SECONDS < deadline)); do
  body=$(curl -fsS --max-time 10 "$base_url/health" 2>/dev/null || true)
  serving=$(printf '%s' "$body" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
  if [[ "$serving" == "$expected" ]]; then
    ready=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$base_url/ready" || true)
    if [[ "$ready" == "200" ]]; then
      echo "$base_url serves $expected and is ready"
      exit 0
    fi
    echo "serving $expected, /ready returned $ready; waiting"
  else
    echo "serving '${serving:-unreachable}', want $expected; waiting"
  fi
  sleep "$interval"
done

echo "::error::$base_url did not serve $expected within ${timeout}s (last seen: '${serving:-unreachable}'). Check the Railway pre-deploy (migration) and deploy logs." >&2
exit 1
