# Research: Sport Activity Tracking API

## Decisions

### Separate completed-activity resource

- **Decision**: Add `sport_activities`; do not add sports to `workout_sessions` or `day_logs`.
- **Rationale**: Sports need duration and a sport snapshot, not performed sets or template semantics.
- **Rejected**: A workout subtype would pollute workout totals and weaken structured-workout meaning.

### Existing idempotency ledger in one transaction

- **Decision**: Lock the athlete sport domain, replay from `idempotency_records`, insert the activity,
  upsert `day_participation`, store the replay response, and commit once.
- **Rationale**: It reuses stable conflicts and makes activity and participation one operation.
- **Rejected**: A table-only unique key cannot replay the response through the platform contract.

### Unique performed dates

- **Decision**: Add sport dates to `GetWeeklyCount` with `UNION`; leave total workouts, workout
  distribution, lifted volume, and planned quality unchanged.
- **Rationale**: Consistency grants one unit per local date and existing metric names retain meaning.
- **Rejected**: Counting minutes or activities double-counts busy dates.

### Profile-timezone date validation

- **Decision**: Load the owned training profile, default omitted date to local today, and reject future dates.
- **Rationale**: Participation is a civil-date concept and the profile timezone is existing truth.
- **Rejected**: UTC shifts dates for athletes outside UTC.

### Go API is the sole data path

- **Decision**: Enable RLS and revoke direct `anon`/`authenticated` grants.
- **Rationale**: This matches the app boundary and avoids a second authorization contract.
- **Rejected**: Direct Data API policies duplicate application access.

## Resolved Technical Questions

- Inclusive `from`/`to`, maximum 366 days, `from <= to`.
- Order: `date DESC, created_at DESC, id DESC`.
- Sport ID: lowercase slug up to 64; display name: trimmed 1–80.
- Duration: whole minutes 1–1,440. Notes: trimmed, optional, max 2,000.
- Operation key: trimmed, required, max 128.
- Missing/foreign IDs: identical `404 NOT_FOUND`.
- Identical replay returns original with `201`; mismatched payload returns `409`.
- No data backfill. Rollback drops the new table and retains participation evidence.
