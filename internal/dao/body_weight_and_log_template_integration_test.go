//go:build integration

package dao

import (
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestBodyWeightUpsertAndListReturnTheDate(t *testing.T) {
	ctx, pool, _, userID := newLogOnceFixture(t)
	weights := NewBodyWeightDAO(pool)

	saved, err := weights.Upsert(ctx, userID, &model.BodyWeight{Date: "2026-11-15", Weight: 81.4, Unit: "kg"})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if saved.Date != "2026-11-15" || saved.Weight != 81.4 || saved.Unit != "kg" {
		t.Fatalf("Upsert() = %+v", saved)
	}
	if _, err := weights.Upsert(ctx, userID, &model.BodyWeight{Date: "2026-11-15", Weight: 81.0, Unit: "kg"}); err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}

	listed, err := weights.List(ctx, userID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].Date != "2026-11-15" || listed[0].Weight != 81.0 {
		t.Fatalf("List() = %+v, want one 2026-11-15 entry at 81.0", listed)
	}
}

func TestDayLogDoesNotLoadAnotherUsersTemplate(t *testing.T) {
	ctx, pool, logs, userID := newLogOnceFixture(t)
	_, _, _, otherUserID := newLogOnceFixture(t)

	insertTemplate := func(owner uuid.UUID, name string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO workout_templates (user_id, name, type_id) VALUES ($1, $2, 'push') RETURNING id`, owner, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	ownTemplate := insertTemplate(userID, "Own push")
	foreignTemplate := insertTemplate(otherUserID, "Someone else's push")

	insertLog := func(date string, templateID uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO day_logs (user_id, date, type_id, subtype_id, template_id) VALUES ($1, $2, 'push', 'hypertrophy', $3)`, userID, date, templateID); err != nil {
			t.Fatal(err)
		}
	}
	insertLog("2026-11-14", ownTemplate)
	insertLog("2026-11-15", foreignTemplate)

	own, err := logs.GetByDate(ctx, userID, "2026-11-14")
	if err != nil {
		t.Fatal(err)
	}
	if own.Template == nil || own.TemplateName == nil || *own.TemplateName != "Own push" {
		t.Fatalf("own template not loaded: %+v", own)
	}

	foreign, err := logs.GetByDate(ctx, userID, "2026-11-15")
	if err != nil {
		t.Fatal(err)
	}
	if foreign.Template != nil || foreign.TemplateName != nil {
		t.Fatalf("another user's template was loaded: template=%+v name=%v", foreign.Template, foreign.TemplateName)
	}
}
