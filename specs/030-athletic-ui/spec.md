# Feature Specification: Athletic UI palettes (accept `irisViolet`, default to `voltLime`)

**Feature Branch**: `spec/030-athletic-ui`

**Feature ID**: `gympulse-athletic-ui`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/034-athletic-ui/spec.md) (app branch `spec/034-athletic-ui`)

**Created**: 2026-11-15

**Status**: Approved (owner approved the Athletic UI redesign on 2026-11-15; this is its API half)

**Input**: The app redesign offers four palettes: `voltLime` (default), `obsidianEmber`, `abyssCerulean`, and a new `irisViolet`. It stops offering `mossChartreuse`, `oliveCitrine`, and `slateMint`, and shows Volt when it reads one of them. The API rejects `irisViolet` today and gives accounts with no stored settings `obsidianEmber`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Choose Iris & Violet (Priority: P1)

An athlete picks the Iris & Violet palette in the app and the choice is saved to their account, so it follows them to another device.

**Independent Test**: `PUT /api/v1/settings` with `{"palette":"irisViolet"}` returns 200 with `palette: "irisViolet"`, and a following `GET /api/v1/settings` returns the same value.

**Acceptance Scenarios**:

1. **Given** any authenticated account, **When** it sends `PUT /api/v1/settings` with `palette: "irisViolet"`, **Then** the response is 200 and the palette is stored; weight unit and weekly goal are unchanged.
2. **Given** an older app build, **When** it sends any of `obsidianEmber`, `abyssCerulean`, `mossChartreuse`, `voltLime`, `oliveCitrine`, or `slateMint`, **Then** the response is still 200 and the value is stored and returned unchanged.
3. **Given** an unknown palette name, **When** it is sent, **Then** the response is still `422 VALIDATION_ERROR` with `details.field = "body"`.

### User Story 2 - New accounts start on Volt (Priority: P1)

An account that has never saved settings reads the app's new default palette.

**Independent Test**: `GET /api/v1/settings` for an account with no `user_settings` row returns `palette: "voltLime"`.

**Acceptance Scenarios**:

1. **Given** an account with no stored settings, **When** it reads settings, **Then** `palette` is `voltLime` (it was `obsidianEmber`), and `weight_unit` and `weekly_goal` are still `lb` and `5`.
2. **Given** an account with no stored settings, **When** its first write omits `palette` (for example only `weekly_goal`), **Then** the stored and returned palette is `voltLime`.
3. **Given** an account with a stored settings row, **When** this change is deployed, **Then** its stored palette is unchanged, including `obsidianEmber`.

### Edge Cases

- **Accounts that never saved settings will now read `voltLime` instead of `obsidianEmber`.** This is intended: it is the redesign's default. An older app build on such an account switches from Ember to Volt on its next settings read; that build already renders `voltLime`.
- An account whose first settings write came from an older build that omitted `palette` has a stored `obsidianEmber`. The service cannot tell that from a deliberate choice, so it is left alone. No backfill.
- An older app build that reads `irisViolet` (set from a newer build on another device) does not know the name. Its fallback is the app's concern and is covered by the app spec; the API does not rewrite stored values per client.
- Rolling the migration back moves `irisViolet` rows to `voltLime` before the constraint narrows, and restores the `obsidianEmber` column default.

## Non-Goals *(mandatory)*

- Removing or rejecting `mossChartreuse`, `oliveCitrine`, or `slateMint`. Older app builds keep working.
- Rewriting stored palettes (no backfill, no mapping of retired palettes to Volt on the server).
- New endpoints, fields, statuses, or error codes.
- Any app change, merge, or deployment.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `PUT /api/v1/settings` accepts `irisViolet` for `palette` in addition to the six existing values.
- **FR-002**: `GET /api/v1/settings` may return `irisViolet`.
- **FR-003**: The database check `user_settings_palette_check` allows the same seven values the validator allows.
- **FR-004**: An account with no stored settings reads `palette: "voltLime"`, and a first write that omits `palette` stores `voltLime`. The `user_settings.palette` column default is `voltLime`.
- **FR-005**: Stored rows are not changed by the forward migration.
- **FR-006**: The down migration restores the six-value check and the `obsidianEmber` column default, after moving `irisViolet` rows to `voltLime`.
- **FR-007**: `docs/CONTRACTS.md` (contract 1.9.0), the generated Swagger files, and `docs/DATABASE_DESIGN.md` describe the new value and default.

### Key Entities

- `user_settings.palette` (existing column; wider check and new default only).

## Design Notes

- **Contract**: additive enum value on an existing field, plus a changed default for accounts with no row. Statuses and error shape unchanged. Contract 1.9.0.
- **Ownership**: unchanged. The user ID still comes only from the verified token.
- **Placement**: the allowed list stays in the `validate` tags on `model.UserSettings` and `model.UpdateUserSettingsRequest`; the default stays in `model.DefaultUserSettings()`, which the DAO already uses for both the no-row read and the insert branch of the single-statement patch. No handler, service, or DAO code changes.
- **Migration** `042_add_settings_palette_iris.up.sql`: one `ALTER TABLE` that swaps the check and sets the column default. `user_settings` has one row per account, so the validation scan under the table lock is short, as in 023 and 024.
- **Mixed versions**: the release step migrates before the new binary serves. The previous binary against the migrated schema behaves as before (it still validates six values). The new binary needs the migrated schema to store `irisViolet`.
- **Rollout**: deploy this API change before the app change. Until then, choosing Iris & Violet in the new app fails validation and the app rolls the choice back.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A `PUT` with `palette: "irisViolet"` returns 200 in handler, service, DAO integration, and smoke tests.
- **SC-002**: All six earlier palette values still validate and store.
- **SC-003**: An account with no settings row reads `voltLime`.
- **SC-004**: `go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh`, the DAO integration suite, the smoke suite, and the Swagger drift check pass.

## Assumptions

- The app spec names the palette `irisViolet` exactly; the API stores the name as an opaque string.
