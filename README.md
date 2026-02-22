# GymPulse API

Go REST API backend for GymPulse, a mobile fitness tracking app. Handles workout template management, daily workout logging, and stats/streak calculations.

## Tech Stack

- **Go 1.23+** with [chi](https://github.com/go-chi/chi) router
- **PostgreSQL** (Supabase-hosted) via [pgx](https://github.com/jackc/pgx)
- **Supabase Auth** — JWT validation only (no auth logic in the API)
- **Deployed on** [Railway](https://railway.app)

## Architecture

```
handler → service → repository → database
```

- **handler/** — HTTP concerns: parse request, call service, write response
- **service/** — Business logic and validation
- **repository/** — SQL queries, accepts/returns model structs
- **model/** — Shared data structures across layers
- **middleware/** — Auth (JWT), CORS, request logging

## Getting Started

### Prerequisites

- Go 1.23+
- PostgreSQL (or a Supabase project)

### Setup

```bash
cp .env.example .env
# Fill in DATABASE_URL and SUPABASE_JWT_SECRET
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

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/api/v1/templates` | List templates (optional `?type=&subtype=`) |
| GET | `/api/v1/templates/:id` | Get template with exercises |
| POST | `/api/v1/templates` | Create template |
| PUT | `/api/v1/templates/:id` | Update template |
| DELETE | `/api/v1/templates/:id` | Delete template |
| GET | `/api/v1/logs?week=YYYY-MM-DD` | List logs for week |
| GET | `/api/v1/logs/:date` | Get log with overrides |
| POST | `/api/v1/logs` | Create day log |
| PUT | `/api/v1/logs/:date` | Update overrides/notes |
| DELETE | `/api/v1/logs/:date` | Delete day log |
| GET | `/api/v1/stats/summary` | Weekly progress, streak, total |
| GET | `/api/v1/stats/distribution` | Workout type distribution |
| GET | `/api/v1/settings` | Get user settings |
| PUT | `/api/v1/settings` | Update user settings |

All endpoints except `/health` require `Authorization: Bearer <supabase_jwt>`.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `SUPABASE_JWT_SECRET` | Yes | — | Supabase JWT secret for token validation |
| `PORT` | No | `8080` | Server port |
| `ALLOWED_ORIGINS` | No | `*` | CORS origins (comma-separated) |
| `ENVIRONMENT` | No | `development` | `development` or `production` |
| `LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, `error` |
