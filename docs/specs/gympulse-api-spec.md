# GymPulse API — Backend Spec

## What This Is

This is the backend API for GymPulse, a mobile fitness tracking app. The API handles workout template management, daily workout logging, and stats/streak calculations. It's a Go REST API deployed on Railway, connecting to a Supabase-hosted Postgres database with Supabase Auth for authentication.

**Repo:** `gym-pulse-api`
**Language:** Go
**Deploys to:** Railway (auto-deploy on push to main)
**Database:** Supabase Postgres (external, connected via DATABASE_URL)
**Auth:** Supabase JWT validation (no auth logic in the API — just verify tokens)

---

## Architecture

```
React Native App
    │
    │  Authorization: Bearer <supabase_jwt>
    ▼
Go API (this repo, hosted on Railway)
    │
    │  pgxpool connection
    ▼
Supabase Postgres (external managed DB)
```

The mobile app authenticates with Supabase Auth directly and receives a JWT. Every request to this API includes that JWT. This API validates the JWT, extracts the `user_id`, and uses it for all DB operations. This API never creates users or manages auth — Supabase handles that entirely.

---

## Tech Stack

| Layer | Choice | Notes |
|-------|--------|-------|
| Language | Go 1.23+ | |
| Router | github.com/go-chi/chi/v5 | Lightweight, idiomatic, great middleware |
| DB Driver | github.com/jackc/pgx/v5/pgxpool | Connection pooling, best Go PG driver |
| Migrations | github.com/golang-migrate/migrate/v4 | SQL file based migrations |
| JWT | github.com/golang-jwt/jwt/v5 | Validate Supabase JWTs |
| Validation | github.com/go-playground/validator/v10 | Struct tag validation |
| Config | Environment variables (os.Getenv) | 12-factor, Railway sets these |
| Logging | log/slog (stdlib) | Structured logging, zero deps |
| UUID | github.com/google/uuid | Generate UUIDs for entities |
| Testing | stdlib testing + testify | |

---

## Project Structure

```
gym-pulse-api/
├── cmd/
│   └── server/
│       └── main.go                  # Entry point: load config, connect DB, start server
├── internal/
│   ├── config/
│   │   └── config.go                # Load env vars into Config struct
│   ├── middleware/
│   │   ├── auth.go                  # JWT validation, extract user_id into context
│   │   ├── cors.go                  # CORS configuration
│   │   └── logging.go               # Request/response logging with slog
│   ├── handler/
│   │   ├── templates.go             # HTTP handlers for template CRUD
│   │   ├── logs.go                  # HTTP handlers for day log CRUD
│   │   ├── stats.go                 # HTTP handlers for stats endpoints
│   │   └── health.go                # GET /health
│   ├── service/
│   │   ├── template_svc.go          # Template business logic, validation
│   │   ├── log_svc.go               # Day log business logic, validation
│   │   └── stats_svc.go             # Streak calculation, distribution aggregation
│   ├── repository/
│   │   ├── template_repo.go         # Template SQL queries
│   │   ├── log_repo.go              # Day log SQL queries
│   │   └── stats_repo.go            # Stats/aggregation SQL queries
│   ├── model/
│   │   ├── template.go              # WorkoutTemplate, Exercise structs
│   │   ├── log.go                   # DayLog, ExerciseOverride structs
│   │   ├── stats.go                 # StatsSummary, Distribution structs
│   │   └── types.go                 # WorkoutTypeId, SubtypeId validation
│   └── router/
│       └── router.go                # Wire up routes + middleware
├── migrations/
│   ├── 001_create_workout_templates.up.sql
│   ├── 001_create_workout_templates.down.sql
│   ├── 002_create_exercises.up.sql
│   ├── 002_create_exercises.down.sql
│   ├── 003_create_day_logs.up.sql
│   ├── 003_create_day_logs.down.sql
│   ├── 004_create_exercise_overrides.up.sql
│   ├── 004_create_exercise_overrides.down.sql
│   ├── 005_create_user_settings.up.sql
│   └── 005_create_user_settings.down.sql
├── Dockerfile
├── go.mod
├── go.sum
├── .env.example
└── README.md
```

### Layer Responsibilities

- **handler/** — HTTP concerns only: parse request, call service, write response. No business logic, no SQL.
- **service/** — Business logic and validation: enforce rules like "can't log future dates", "type_id must be valid", streak calculation. Calls repository.
- **repository/** — SQL queries only. Accepts and returns model structs. No HTTP awareness.
- **model/** — Data structures shared across layers. JSON tags for API responses, db tags or scan methods for pgx.
- **middleware/** — Cross-cutting: auth, CORS, logging. Applied in router.

---

## Environment Variables

```env
# Required
PORT=8080                                            # Railway provides this
DATABASE_URL=postgresql://postgres:pw@db.xxx.supabase.co:5432/postgres
SUPABASE_JWT_SECRET=your-supabase-jwt-secret         # Supabase dashboard → Settings → API → JWT Secret

# Optional
ALLOWED_ORIGINS=http://localhost:8081,https://your-app.com   # CORS origins, comma-separated
ENVIRONMENT=development                                       # development | production
LOG_LEVEL=info                                                # debug | info | warn | error
```

Create `.env.example` with these. Never commit actual secrets.

---

## Database Schema

All tables live in the `public` schema of Supabase Postgres. Users are managed by Supabase Auth in `auth.users`.

### Migration 001: workout_templates

```sql
-- UP
CREATE TABLE workout_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    name        TEXT NOT NULL,
    type_id     TEXT NOT NULL,
    subtype_id  TEXT NOT NULL DEFAULT 'general',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_templates_user ON workout_templates(user_id);
CREATE INDEX idx_templates_user_type ON workout_templates(user_id, type_id);

-- DOWN
DROP TABLE IF EXISTS workout_templates;
```

### Migration 002: exercises

```sql
-- UP
CREATE TABLE exercises (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id     UUID NOT NULL REFERENCES workout_templates(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    sets            INTEGER,
    reps            INTEGER,
    weight          NUMERIC(7,2),
    rest_seconds    INTEGER,
    notes           TEXT
);

CREATE INDEX idx_exercises_template ON exercises(template_id);

-- DOWN
DROP TABLE IF EXISTS exercises;
```

### Migration 003: day_logs

```sql
-- UP
CREATE TABLE day_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL,
    date            DATE NOT NULL,
    type_id         TEXT NOT NULL,
    subtype_id      TEXT NOT NULL DEFAULT 'general',
    template_id     UUID REFERENCES workout_templates(id) ON DELETE SET NULL,
    session_notes   TEXT,
    logged_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    UNIQUE(user_id, date)
);

CREATE INDEX idx_logs_user_date ON day_logs(user_id, date);

-- DOWN
DROP TABLE IF EXISTS day_logs;
```

### Migration 004: exercise_overrides

```sql
-- UP
CREATE TABLE exercise_overrides (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    day_log_id      UUID NOT NULL REFERENCES day_logs(id) ON DELETE CASCADE,
    exercise_id     UUID NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
    actual_sets     INTEGER,
    actual_reps     INTEGER,
    actual_weight   NUMERIC(7,2),
    notes           TEXT,
    skipped         BOOLEAN DEFAULT false
);

CREATE INDEX idx_overrides_log ON exercise_overrides(day_log_id);

-- DOWN
DROP TABLE IF EXISTS exercise_overrides;
```

### Migration 005: user_settings

```sql
-- UP
CREATE TABLE user_settings (
    user_id         UUID PRIMARY KEY,
    weight_unit     TEXT NOT NULL DEFAULT 'lb',
    weekly_goal     INTEGER NOT NULL DEFAULT 5,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- DOWN
DROP TABLE IF EXISTS user_settings;
```

### Schema Design Decisions

- **No FK to auth.users** — Supabase manages auth.users in a separate schema. We store user_id as UUID and validate via JWT. Avoids cross-schema FK complexity.
- **Exercises are their own table** — not JSONB. Enables per-exercise history, progression queries, and efficient updates.
- **`ON DELETE SET NULL` on day_logs.template_id** — deleting a template doesn't delete logs. Logs keep type/subtype.
- **`ON DELETE CASCADE` on exercises** — deleting a template cleans up its exercises.
- **`UNIQUE(user_id, date)` on day_logs** — one workout per user per day, enforced at DB level.
- **type_id/subtype_id are TEXT** — validated at the API layer against hardcoded valid lists. No extra tables needed.
- **weight is NUMERIC(7,2)** — supports up to 99999.99 in either lb or kg.

---

## API Endpoints

All endpoints except `/health` require `Authorization: Bearer <supabase_jwt>` header.

### Health

```
GET /health
→ 200 { "status": "ok" }
```

### Templates

```
GET /api/v1/templates
  Query params: ?type=pull&subtype=hypertrophy (both optional)
  → 200 [{ id, name, type_id, subtype_id, exercise_count, exercises_preview, created_at, updated_at }]

GET /api/v1/templates/:id
  → 200 { id, name, type_id, subtype_id, exercises: [...], created_at, updated_at }
  → 404 if not found or not owned by user

POST /api/v1/templates
  Body: { name, type_id, subtype_id, exercises: [{ name, sets?, reps?, weight?, rest_seconds?, notes? }] }
  → 201 { id, name, type_id, subtype_id, exercises: [...], created_at, updated_at }
  → 422 if validation fails

PUT /api/v1/templates/:id
  Body: { name, type_id, subtype_id, exercises: [{ id? (existing), name, sets?, ... }] }
  Strategy: delete all existing exercises, re-insert with new data (simpler than diffing)
  → 200 { updated template }
  → 404 if not found or not owned

DELETE /api/v1/templates/:id
  → 204 No Content
  → 404 if not found or not owned
```

### Day Logs

```
GET /api/v1/logs?week=2026-02-16
  Returns all logs for the week containing that date (Monday through Sunday)
  → 200 [{ id, date, type_id, subtype_id, template_id, template_name, session_notes, logged_at }]

GET /api/v1/logs/:date
  Date format: 2026-02-16
  Returns single log with full exercise overrides
  → 200 { id, date, type_id, subtype_id, template_id, template: { name, exercises }, overrides, session_notes }
  → 404 if no log for that date

POST /api/v1/logs
  Body: { date, type_id, subtype_id, template_id?, overrides?[], session_notes? }
  → 201 { created log }
  → 409 if log already exists for that date (unique constraint)
  → 422 if date is in the future, or type_id/subtype_id invalid

PUT /api/v1/logs/:date
  Body: { overrides?[], session_notes? }
  Only updates overrides and notes — cannot change type/subtype/template after logging
  → 200 { updated log }
  → 404 if no log for that date

DELETE /api/v1/logs/:date
  → 204 No Content
  → 404 if no log for that date
```

### Stats

```
GET /api/v1/stats/summary
  → 200 {
      this_week: { completed: 3, goal: 5 },
      streak_weeks: 4,
      total_workouts: 47
  }

GET /api/v1/stats/distribution
  → 200 {
      types: [
        {
          type_id: "pull", count: 15,
          subtypes: [
            { subtype_id: "hypertrophy", count: 10 },
            { subtype_id: "strength", count: 5 }
          ]
        },
        { type_id: "push", count: 12, subtypes: [...] }
      ]
  }
```

### Settings

```
GET /api/v1/settings
  → 200 { weight_unit: "lb", weekly_goal: 5 }
  → 200 { weight_unit: "lb", weekly_goal: 5 } (returns defaults if no row exists)

PUT /api/v1/settings
  Body: { weight_unit?: "lb"|"kg", weekly_goal?: 3-7 }
  Uses UPSERT — creates row if not exists
  → 200 { weight_unit, weekly_goal }
```

---

## Models

```go
// --- Valid Types & Subtypes ---

var ValidTypeIDs = []string{
    "push", "pull", "legs", "cardio",
    "upper", "lower", "full", "core", "other",
}

var ValidSubtypeIDs = []string{
    "hypertrophy", "strength", "power", "endurance",
    "mobility", "conditioning", "skills", "general",
}

// --- Templates ---

type Exercise struct {
    ID          uuid.UUID  `json:"id"`
    TemplateID  uuid.UUID  `json:"-"`
    Name        string     `json:"name" validate:"required,min=1,max=200"`
    SortOrder   int        `json:"sort_order"`
    Sets        *int       `json:"sets,omitempty"`
    Reps        *int       `json:"reps,omitempty"`
    Weight      *float64   `json:"weight,omitempty"`
    RestSeconds *int       `json:"rest_seconds,omitempty"`
    Notes       *string    `json:"notes,omitempty"`
}

type WorkoutTemplate struct {
    ID        uuid.UUID  `json:"id"`
    UserID    uuid.UUID  `json:"-"`
    Name      string     `json:"name" validate:"required,min=1,max=100"`
    TypeID    string     `json:"type_id" validate:"required"`
    SubtypeID string     `json:"subtype_id" validate:"required"`
    Exercises []Exercise `json:"exercises"`
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}

// For list endpoint (no full exercises)
type TemplateSummary struct {
    ID               uuid.UUID `json:"id"`
    Name             string    `json:"name"`
    TypeID           string    `json:"type_id"`
    SubtypeID        string    `json:"subtype_id"`
    ExerciseCount    int       `json:"exercise_count"`
    ExercisesPreview []string  `json:"exercises_preview"` // First 3 exercise names
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
}

// --- Day Logs ---

type ExerciseOverride struct {
    ID           uuid.UUID `json:"id"`
    DayLogID     uuid.UUID `json:"-"`
    ExerciseID   uuid.UUID `json:"exercise_id" validate:"required"`
    ActualSets   *int      `json:"actual_sets,omitempty"`
    ActualReps   *int      `json:"actual_reps,omitempty"`
    ActualWeight *float64  `json:"actual_weight,omitempty"`
    Notes        *string   `json:"notes,omitempty"`
    Skipped      bool      `json:"skipped"`
}

type DayLog struct {
    ID           uuid.UUID          `json:"id"`
    UserID       uuid.UUID          `json:"-"`
    Date         string             `json:"date"`               // "2026-02-16"
    TypeID       string             `json:"type_id" validate:"required"`
    SubtypeID    string             `json:"subtype_id" validate:"required"`
    TemplateID   *uuid.UUID         `json:"template_id,omitempty"`
    Overrides    []ExerciseOverride `json:"overrides,omitempty"`
    SessionNotes *string            `json:"session_notes,omitempty"`
    LoggedAt     time.Time          `json:"logged_at"`
}

// --- Stats ---

type StatsSummary struct {
    ThisWeek      WeekProgress `json:"this_week"`
    StreakWeeks   int          `json:"streak_weeks"`
    TotalWorkouts int          `json:"total_workouts"`
}

type WeekProgress struct {
    Completed int `json:"completed"`
    Goal      int `json:"goal"`
}

type TypeDistribution struct {
    TypeID   string                `json:"type_id"`
    Count    int                   `json:"count"`
    Subtypes []SubtypeDistribution `json:"subtypes"`
}

type SubtypeDistribution struct {
    SubtypeID string `json:"subtype_id"`
    Count     int    `json:"count"`
}

// --- Settings ---

type UserSettings struct {
    WeightUnit string `json:"weight_unit" validate:"oneof=lb kg"`
    WeeklyGoal int    `json:"weekly_goal" validate:"min=3,max=7"`
}
```

---

## Auth Middleware

```go
type contextKey string
const UserIDKey contextKey = "user_id"

func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
                return
            }

            tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
            if tokenStr == authHeader {
                http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
                return
            }

            token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
                if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                    return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
                }
                return []byte(jwtSecret), nil
            })
            if err != nil || !token.Valid {
                http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
                return
            }

            claims, ok := token.Claims.(jwt.MapClaims)
            if !ok {
                http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
                return
            }

            userID, ok := claims["sub"].(string)
            if !ok || userID == "" {
                http.Error(w, `{"error":"missing user id in token"}`, http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), UserIDKey, userID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// Helper to extract user ID from context
func GetUserID(ctx context.Context) (uuid.UUID, error) {
    userIDStr, ok := ctx.Value(UserIDKey).(string)
    if !ok {
        return uuid.Nil, fmt.Errorf("user_id not found in context")
    }
    return uuid.Parse(userIDStr)
}
```

---

## Key Business Rules

### Validation Rules
- `type_id` must be one of: push, pull, legs, cardio, upper, lower, full, core, other
- `subtype_id` must be one of: hypertrophy, strength, power, endurance, mobility, conditioning, skills, general
- `date` on day logs cannot be in the future
- `date` on day logs must be a valid ISO date (YYYY-MM-DD)
- Template `name` is required, 1–100 characters
- Exercise `name` is required, 1–200 characters
- `weekly_goal` must be 3–7
- `weight_unit` must be "lb" or "kg"

### Ownership Rules
- All queries MUST filter by `user_id` from the JWT. Never trust client-provided user_id.
- A user can only read/modify their own templates, logs, and settings.
- Template endpoints: verify `workout_templates.user_id = jwt_user_id`
- Log endpoints: verify `day_logs.user_id = jwt_user_id`
- Return 404 (not 403) when a resource exists but isn't owned by the user — don't leak existence.

### Streak Calculation (stats_svc.go)
```
Input: user_id, weekly_goal
Output: number of consecutive weeks meeting the goal

streak = 0
checkWeek = current Monday

loop:
  count = COUNT logs WHERE user_id AND date BETWEEN checkWeek Monday..Sunday

  if checkWeek == current week:
    if count > 0 → streak++, move to previous week
    else → move to previous week (grace for in-progress week)
      if prev count >= goal → streak++, continue
      else → break
  else:
    if count >= goal → streak++, move to previous week
    else → break

return streak
```

---

## Dockerfile

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/server .
COPY migrations/ ./migrations/
EXPOSE 8080
CMD ["./server"]
```

---

## Railway Deployment

### Setup Steps
1. Push this repo to GitHub
2. Create Railway project → "Deploy from GitHub repo" → select this repo
3. Railway auto-detects Dockerfile, builds and deploys
4. Set environment variables in Railway dashboard:
   - `DATABASE_URL` — from Supabase dashboard → Settings → Database → Connection string (use "connection pooling" URI with port 6543 for best results)
   - `SUPABASE_JWT_SECRET` — from Supabase dashboard → Settings → API → JWT Secret
   - `ALLOWED_ORIGINS` — comma-separated allowed CORS origins
   - `ENVIRONMENT` — `production`
5. Set health check path to `/health` in Railway service settings
6. Railway provides a URL like `https://gym-pulse-api-production.up.railway.app`

### Deploy Workflow
- Push to `main` → Railway auto-builds → zero-downtime deploy
- Preview branches possible on Railway Pro plan

### Running Migrations
```bash
# Option 1: Auto-migrate on server startup (recommended for v1)
# In main.go, run migrations before starting the HTTP server

# Option 2: Manual via Railway CLI
railway run go run ./cmd/migrate up
```

---

## Local Development

```bash
# 1. Clone repo
git clone https://github.com/you/gym-pulse-api.git
cd gym-pulse-api

# 2. Copy env
cp .env.example .env
# Fill in DATABASE_URL (can use Supabase project or local Postgres)
# Fill in SUPABASE_JWT_SECRET

# 3. Run migrations
go run ./cmd/server migrate

# 4. Run server
go run ./cmd/server
# Server starts on http://localhost:8080

# 5. Test
curl http://localhost:8080/health
```

---

## Error Response Format

All errors return JSON:

```json
{
  "error": "human-readable message",
  "code": "VALIDATION_ERROR",
  "details": { "field": "type_id", "message": "invalid workout type" }
}
```

HTTP status codes:
- `200` — Success
- `201` — Created
- `204` — Deleted (no body)
- `400` — Bad request (malformed JSON, missing params)
- `401` — Unauthorized (missing/invalid JWT)
- `404` — Not found (or not owned by user)
- `409` — Conflict (duplicate day log date)
- `422` — Validation error (future date, invalid type_id, etc.)
- `500` — Internal server error
