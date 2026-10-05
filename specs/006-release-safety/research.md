# Research: Release-Safe Training Workflows

## Decision 1: Bound inclusive goal-training ranges at 366 days

**Decision**: Reject any inclusive range whose end is more than 365 calendar days after its start, producing at most 366 dates.

**Rationale**: This covers a full leap-year history while making generation, lazy finalization, and result loading finite. Calendar arithmetic uses parsed dates rather than elapsed-hour math, avoiding daylight-saving ambiguity.

**Alternatives considered**:

- 31 or 90 days: safer but would constrain legitimate annual history/export use.
- Endpoint-specific limits: more flexible but harder for clients to reason about and easier to miss on a new path.
- Pagination alone: appropriate for history lists later, but it does not bound materialization loops or lazy-finalization side effects.

## Decision 2: Use one short PostgreSQL transaction per mutation

**Decision**: Check idempotency, compare revisions, write all participating aggregates, load the authoritative response, insert its replay record, and commit in one transaction. No network work or unbounded iteration occurs inside it.

**Rationale**: This is the only way to prevent a finalized workout without participation (or the inverse) after a process crash. It follows the Supabase/PostgreSQL guidance to keep lock duration short and makes database constraints coordinate all load-balanced replicas.

**Alternatives considered**:

- Compensating writes: a crash can occur before compensation and still exposes inconsistent data.
- In-memory locks or replay caches: do not coordinate replicas and disappear on restart.
- A message queue/outbox: materially larger operational scope and unnecessary for synchronous state that fits one database transaction.

## Decision 3: Preserve consistent lock ordering

**Decision**: Transactions that need both domains acquire the program advisory lock before the schedule advisory lock. Schedule-only mutations take only the schedule lock.

**Rationale**: Consistent ordering prevents cycles between adoption, materialization, regeneration, plan transition, and workout logging. User-scoped transaction locks serialize conflicting workflows across replicas without holding a separate connection after commit.

**Alternatives considered**:

- Row locks only: aggregate creation has no row to lock and cross-table ordering remains inconsistent.
- One global lock: simple but destroys horizontal throughput between unrelated users.

## Decision 4: Treat adoption as a durable one-time import

**Decision**: Read owned non-rest weekly assignments ordered by ISO weekday; create one program workout per assigned weekday; map each referenced template's exercises; schedule the next Monday-through-Sunday block strictly after the athlete's local today; never update legacy rows.

**Rationale**: It reflects the existing recurring plan exactly, avoids rewriting current/historical activity, and gives deterministic behavior for repeat and concurrent calls.

**Alternatives considered**:

- Import every owned template: would copy templates the athlete does not currently use.
- Import the current week: could collide with historical or already-started activity.
- Keep the new program continuously linked to legacy rows: would violate immutable goal-training snapshots and make future legacy edits rewrite the new plan.

## Decision 5: Make replay evaluation precede resource/revision evaluation

**Decision**: Services perform a cheap typed replay lookup before reads needed to build a new mutation, while the transaction DAO repeats the check under the domain lock before writing.

**Rationale**: An exact retry necessarily carries the old expected revision. Evaluating the current revision first incorrectly turns a safe replay into a revision conflict. The second in-transaction check closes the concurrent duplicate race.

**Alternatives considered**:

- Skip the service pre-check: some services would perform or prepare secondary writes before reaching the transaction boundary.
- Trust only the service pre-check: two concurrent first attempts could both miss it.

## Decision 6: Reuse existing schema and dependencies

**Decision**: Add no migration and no package dependency.

**Rationale**: The existing `idempotency_records` and `legacy_adoptions` tables already encode the required user/scope/key uniqueness. Existing composite indexes cover owner/date and owner/operation access patterns; the imported week is bounded to seven rows.

**Alternatives considered**:

- Add adoption date columns: the authoritative dated resources are already linked through program IDs and persisted in the replay response.
- Add Redis: adds distributed state, failure modes, and cost while PostgreSQL remains authoritative.

## Decision 7: Preserve API-first compatibility for existing header-only calls

**Decision**: Training-profile PUT and custom-program POST/PUT normalize a
missing JSON `operation_key` from the required `Idempotency-Key` header. A
present JSON value must still match. Every other goal-training mutation remains
strict, and the dependent app release sends both values everywhere.

**Rationale**: The currently shipped client already supplies stable header
keys on these three calls but does not duplicate them in the body. The API must
deploy before the dependent mobile behavior, so strict body enforcement would
turn an otherwise additive API rollout into a breaking change.

**Alternatives considered**:

- Deploy the app first: impossible for legacy adoption because the current API
  does not expose the new route, and App Store rollout is not atomic.
- Require the body immediately: simpler, but breaks profile and custom-program
  writes from existing app versions.

## Decision 8: Detach catalog IDs submitted through the legacy exercise slot

**Decision**: Extra-set `exercise_id` preserves an owned legacy exercise UUID,
accepts a recognized global catalog UUID only by normalizing it to null, and
rejects every foreign or unknown UUID. Immutable exercise snapshots remain the
authoritative history.

**Rationale**: The current app sends `scheduled_set.catalog_id` through a field
whose database foreign key points to the owned legacy `exercises` table. There
is no catalog-provenance column on `set_logs`; inserting the UUID would fail.
Detaching a known catalog UUID repairs the existing path without schema churn,
while the dependent app stops sending the ambiguous value.

**Alternatives considered**:

- Add a `catalog_id` migration: clearer long-term provenance, but it expands a
  release-safety sprint that does not rely on mutable IDs for history.
- Accept any UUID as null: avoids the foreign-key error but weakens nested-ID
  ownership and typo detection.
