#!/usr/bin/env bash
# Definition of done for goal-based-training (API). Exit 0 = built.
set -euo pipefail
cd "$(dirname "$0")/../.."

fail() { echo "FAIL: $*" >&2; exit 1; }

# C1-C11: the public contract must name the new source-of-truth concepts.
grep -qi "primary_goal" docs/CONTRACTS.md \
  || fail "C1 — primary_goal is absent from docs/CONTRACTS.md"
grep -qiE "scheduled_workout|scheduled workout" docs/CONTRACTS.md \
  || fail "C3-C7 — dated scheduled-workout contract is absent"
grep -qi "incomplete" docs/CONTRACTS.md \
  || fail "C7 — incomplete workout status is absent"
grep -qi "participation" docs/CONTRACTS.md \
  || fail "C8-C9 — day participation is not separated from workout status"
grep -qi "idempot" docs/CONTRACTS.md \
  || fail "C11 — offline replay/idempotency behavior is undocumented"

# C12: observable API behavior is covered and the full Go suite is green.
go test ./... || fail "C12 — Go test suite failing"
grep -qi "primary_goal" scripts/smoke.sh \
  || fail "C12 — smoke.sh does not exercise the training profile"
grep -qiE "scheduled_workout|scheduled workout" scripts/smoke.sh \
  || fail "C12 — smoke.sh does not exercise a dated scheduled workout"
grep -qi "participation" scripts/smoke.sh \
  || fail "C12 — smoke.sh does not verify day participation"

echo "goal-based-training API: all criteria met"
