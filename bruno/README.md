# GymPulse API — Bruno collection

A [Bruno](https://www.usebruno.com/) collection with every route in `internal/router/router.go`,
plus Supabase sign-in requests that fetch and refresh access tokens.

## Open it

1. Install Bruno (`brew install --cask bruno`).
2. **Open Collection** → choose this `bruno/` folder.
3. Pick an environment (top right): `production` or `local`.

## Sign in (hosted API)

1. Environment settings → `production` → fill the **secret** values:
   `supabasePublishableKey` (Supabase → Project Settings → API Keys; public),
   `email`, `password`. Secret values stay on your machine; Bruno never writes them to these files.
2. Run **00 Auth / Sign in**. It stores `accessToken`, `refreshToken` and `userId`.
3. Every other request now sends `Authorization: Bearer {{accessToken}}`.
4. After about an hour you get `401`: run **00 Auth / Refresh token**.

The hosted API only accepts accounts listed in its `ALLOWED_USER_IDS`; anyone else gets
`403 ACCOUNT_NOT_ALLOWED`. **00 Auth / Current user** shows your id.

## Local API

Nothing to sign in to: on the `local` environment the collection signs the same HS256 test token as
`scripts/smoke.sh` for `localUserId` with `localJwtSecret` before the first request
(**00 Auth / Local test token** does the same on demand). Only development and test servers accept
it, and `localUserId` must exist in the local `auth.users`. The four Supabase requests skip
themselves when `supabasePublishableKey` is empty.

Run everything from the command line:

```bash
npx @usebruno/cli run --env local -r
```

## Destructive requests

Every delete lives in **99 Cleanup (destructive)**, so earlier folders can use what they create;
account deletion runs last. Those requests, and a few others named `DESTRUCTIVE …`, delete data or change it in ways you cannot undo (including
`DELETE /api/v1/account`, which deletes the whole account). They throw before sending unless the
environment sets `allowDestructive` to `true`. `production` ships with `false`; leave it that way
unless you mean it.

## Ids between requests

Create requests store the new id (`programId`, `templateId`, `foodId`, …) for the requests that
follow, so run a folder's create request before its get, update or delete requests.

## Requests that need existing data

On a brand-new account these answer 404, 409 or 422 by design; each request's Docs tab says what it
needs: Adopt legacy weekly plan, Recover missed workout, Skip missed workout, Link and Unlink sport
session, Advance stage, and Upload avatar (needs Supabase Storage; the local stack answers 503).

Note: renaming a scheduled workout (`PATCH /scheduled-workouts/{id}`) replaces its sets with new ids,
so that request re-captures `setId`.

## Rate limits

Most routes share 600 requests per minute per user. Tighter limits: program generation 5/min,
schedule rewrites 30/min, food search and barcode 60/min, export and imports 6/min, avatar upload
and account deletion 10/min.

## Keep it current

When a route is added or its body changes, update the matching `.bru` file in the same pull request
as `docs/CONTRACTS.md`.
