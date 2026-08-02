# GymPulse API

Go REST API backend for GymPulse, a mobile fitness tracking app. Handles workout template management, daily workout logging, and stats/streak calculations.

## Tech Stack

- **Go 1.26+** with [chi](https://github.com/go-chi/chi) router
- **PostgreSQL** (Supabase-hosted) via [pgx](https://github.com/jackc/pgx)
- **Supabase Auth** — JWT validation only (no auth logic in the API)
- **Deployed on** [Railway](https://railway.app)

## Architecture

```
handler → service → DAO → database
```

- **handler/** — HTTP concerns: parse request, call service, write response
- **service/** — Business logic and validation
- **dao/** — SQL queries and transaction boundaries, accepts/returns model structs
- **model/** — Shared data structures across layers
- **middleware/** — Auth (JWT), CORS, request logging

## Getting Started

### Prerequisites

- Go 1.26+
- PostgreSQL (or a Supabase project)

### Setup

```bash
cp .env.example .env
# Keep ENVIRONMENT explicit and fill in DATABASE_URL and SUPABASE_JWT_SECRET
```

### Run

```bash
go run ./cmd/server
```

Server starts on `http://localhost:8080`. Migrations run automatically on startup.

### Verify

```bash
curl http://localhost:8080/health
# → {"status":"ok"}
```

## API Surface

The API covers health/readiness, account/profile/settings, templates and the exercise catalog,
legacy logs and plans, body weight and statistics, training profiles and programs, scheduled
workouts and sessions, participation, and plan transitions.

`docs/CONTRACTS.md` is the client-facing source of truth for routes, payloads, validation, statuses,
stable errors, idempotency, and revisions. Generated Swagger is available from `/docs/` while the
server is running. Route registration lives in `internal/router/router.go`; avoid duplicating a
manually maintained endpoint inventory here.

All application endpoints require `Authorization: Bearer <supabase_jwt>`. `/health`, `/ready`, and
generated documentation are operational surfaces outside the authenticated application API.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `DATABASE_LOCK_URL` | Staging/production | `DATABASE_URL` in development/test | Direct or session-mode PostgreSQL connection for session advisory locks; Supavisor transaction mode (`:6543`) is rejected |
| `SUPABASE_URL` | Staging/production | — | Canonical Supabase project URL used to derive the production JWT issuer and JWKS endpoint |
| `SUPABASE_SERVICE_ROLE_KEY` | Staging/production | — | Server-only credential for avatar and auth-user deletion |
| `SUPABASE_JWT_SECRET` | Development/test | — | Legacy local HMAC token validation; production uses JWKS derived from `SUPABASE_URL` |
| `SUPABASE_JWT_AUDIENCE` | Staging/production | — | Expected access-token audience, normally `authenticated` |
| `PORT` | No | `8080` | Server port |
| `ALLOWED_ORIGINS` | Staging/production | `*` in development/test | Explicit HTTPS CORS origins (comma-separated) |
| `ENVIRONMENT` | Yes | Binary: —; production image: `production` | Explicitly select `development`, `test`, `staging`, or `production`; blank values fail startup |
| `LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, `error` |
