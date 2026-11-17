# Implementation Plan: Timed exercises and chosen stretching routines in saved workouts

**Feature ID**: `gympulse-workout-stretches` | **Spec**: [spec.md](spec.md) | **Date**: 2026-11-17

## Summary

Three nullable columns and the code that carries them: `exercises.duration_seconds` (seconds per set) and `workout_templates.warmup_routine_id` / `cooldown_routine_id` (app routine slugs). The template service gains a third exercise shape and the routine checks; every path that copies template exercises carries the new column. Contract 1.13.0, additive.

## Workflow and Specialist Scope

API engineering and the PostgreSQL practices skill apply (schema, constraints, migration lint). Cross-repository delivery applies: the app sibling is spec 039, API first. One primary agent; no delegation beyond two read-only research passes.

## Technical Context

Go, pgx, PostgreSQL. Layers: `internal/model` → `internal/service` → `internal/dao`. Migration 049 follows the strict lint from 043 (named constraints, `IF NOT EXISTS`, drop-then-add).

## Constitution Check

- Ownership: templates are read and written by owner only; no new route, no new table, so RLS and grants are unchanged.
- Contracts: `docs/CONTRACTS.md` updated in the same change; swagger regenerated.
- Tests: unit tests for the shape rule, routine semantics, the optional field, and legacy adoption; an integration test for the round trip, the export, the day-log template, and the checks.
- Compatibility: additive; the absent-key rule protects edits from an older app.

## Design

### Schema (migration 049)

| Column | Type | Rule |
| --- | --- | --- |
| `exercises.duration_seconds` | `INTEGER` | `NULL` or 1..3600 (`exercises_duration_seconds_check`) |
| `workout_templates.warmup_routine_id` | `TEXT` | `NULL` or a 1..64 slug (`workout_templates_warmup_routine_id_check`) |
| `workout_templates.cooldown_routine_id` | `TEXT` | same (`workout_templates_cooldown_routine_id_check`) |

No index: both are read only with their row. No backfill.

### Model

- `Exercise.DurationSeconds`, `CreateExerciseRequest.DurationSeconds` (`min=1,max=3600`).
- `WorkoutTemplate.WarmupRoutineID`, `CooldownRoutineID` (`*string`, always serialized).
- `CreateTemplateRequest` routine fields are `OptionalText`, which records whether the key was in the body.

### Service

`validateExerciseShape`: strength, timed, or cardio, exactly one. `validateRequest`: routine slug and length. `Update`: an absent routine key keeps the stored value.

### Paths that copy template exercises

| Path | Change |
| --- | --- |
| `templateDAO` get, insert, update | new columns |
| `logDAO.getTemplate` | selects `duration_seconds` and the routine fields |
| `readExportTemplates`, `ExportTemplate`, `ExportTemplateExercise` | new fields |
| `loadLegacyWeeklyAssignments`, `ProgramFromLegacyWeeklyPlan` | timed stays strength with `target_duration_seconds` |
| `logDAO.insertSetLogs` | none: modality is derived from `duration_minutes`, so a timed exercise is `strength` |

### Compatibility and rollout

Additive. API first; the app detects 1.13.0 from the routine keys on a template read.

## Complexity Tracking

`OptionalText` is one small type. The alternative, treating every PUT as a full replacement, would let the released app clear a routine choice on any edit.
