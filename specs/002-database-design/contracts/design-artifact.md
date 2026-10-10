# Database Design Artifact Contract

This feature introduces no new HTTP endpoint, request field, response field, status, or stable error
code. `docs/CONTRACTS.md` remains unchanged.

The delivered design is conformant only when it:

1. Labels current facts as **Current** and unimplemented recommendations as **Target**.
2. Includes exactly every table created by migrations 001–019, including `auth.users`.
3. Includes every current column's type, nullability, default, purpose, and sensitivity.
4. Includes all current keys, checks, uniqueness, FK delete actions, and named indexes.
5. Traces public persistence concepts to `docs/CONTRACTS.md` and calls out derived/external data.
6. Documents query access patterns and both current and proposed supporting indexes.
7. Documents transaction, lock-order, revision, idempotency, and retry rules.
8. Documents account deletion, operational-record expiry, backup retention, and recovery targets.
9. Documents checked-in RLS/grant hardening separately from unverified hosted exposure and role configuration.
10. Gives every target gap a priority, forward migration stage, compatibility rule, and validation
    requirement.
11. Includes a non-mutating validation command that reports pass, drift, or an unavailable optional
    environment.

No target recommendation may be presented as deployed behavior until a later migration and its tests
land.
