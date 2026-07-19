# Engineering Hardening

## Outcome

GymPulse preserves user workout data under partial edits, retries, concurrent requests, and failed
database operations. Authentication and HTTP boundaries resist abusive input, and API behavior stays
compatible with released mobile clients.

This specification is coordinated with the app repository's
`specs/001-engineering-hardening/spec.md`.

## User journeys

### Safe workout edits

When a user changes notes, overrides, or performed sets, fields they did not edit remain unchanged.
Explicit empty collections clear only the named collection. Replacing the workout clears detail tied
to the previous workout unless replacement detail is supplied.

### Safe schedule changes

When schedule regeneration fails, the prior schedule remains intact. Concurrent session start and
regeneration cannot delete an active workout.

### Safe retries

Repeating an idempotent operation with the same key and payload returns the original result without
duplicating state. Reusing the key with another payload returns the documented conflict.

### Reliable program creation

Creating or cloning a program makes it active and atomically deactivates the prior active program.

### Secure requests

Authentication does not repeatedly fetch JWKS for arbitrary unknown keys, accepts only configured
algorithms/issuer/audience, and rejects malformed subjects. Oversized or trailing JSON is rejected
with a stable public error.

## Acceptance scenarios

1. A notes-only log update preserves overrides and set logs.
2. An overrides-only update preserves notes and set logs.
3. An explicit empty collection clears only that collection.
4. A failed regeneration leaves all prior eligible schedule rows unchanged.
5. Concurrent identical idempotent requests produce one mutation and the same response.
6. Materialize replay returns the original resources after unrelated schedule changes.
7. Two created programs leave exactly one active program according to the newest request.
8. Concurrent unknown JWT key IDs cause at most one bounded JWKS refresh per cooldown.
9. Over-limit and trailing JSON requests return documented errors.

## Non-goals

- Redesigning workout planning or onboarding.
- Replacing Supabase authentication.
- Combining performance optimization with transactional integrity changes.
- Removing compatibility behavior for released app versions.

## Product decisions

- Program creation activates the new program and deactivates the previous one atomically.
- Omitted log fields preserve existing values; explicit arrays replace or clear them.
- An empty string clears session notes; omission preserves them.
- Request bodies are limited to 1 MiB unless measured legitimate payloads require adjustment.
- Production requires configured Supabase issuer and audience.

