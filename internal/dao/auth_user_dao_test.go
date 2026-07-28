package dao

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type authUserQueryRow struct {
	exists bool
	err    error
}

func (r authUserQueryRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("unexpected scan target count")
	}
	target, ok := dest[0].(*bool)
	if !ok {
		return errors.New("unexpected scan target type")
	}
	*target = r.exists
	return nil
}

type authUserQuerierStub struct {
	row   pgx.Row
	query string
	args  []any
}

func (q *authUserQuerierStub) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	q.query = query
	q.args = args
	return q.row
}

func TestAuthUserDAOExists(t *testing.T) {
	userID := uuid.New()

	t.Run("returns authoritative existence", func(t *testing.T) {
		for _, exists := range []bool{true, false} {
			querier := &authUserQuerierStub{row: authUserQueryRow{exists: exists}}
			repo := &AuthUserDAO{database: querier}

			got, err := repo.Exists(context.Background(), userID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != exists {
				t.Fatalf("exists = %t, want %t", got, exists)
			}
			if !strings.Contains(querier.query, "FROM auth.users WHERE id = $1") {
				t.Fatalf("query is not scoped to the auth identity root: %s", querier.query)
			}
			if len(querier.args) != 1 || querier.args[0] != userID {
				t.Fatalf("query args = %v, want [%s]", querier.args, userID)
			}
		}
	})

	t.Run("wraps database failures", func(t *testing.T) {
		queryErr := errors.New("database unavailable")
		repo := &AuthUserDAO{database: &authUserQuerierStub{row: authUserQueryRow{err: queryErr}}}

		exists, err := repo.Exists(context.Background(), userID)
		if exists || !errors.Is(err, queryErr) {
			t.Fatalf("exists = %t, err = %v; want false and wrapped database error", exists, err)
		}
	})
}
