package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// FoodSource is one outside publisher of food facts. Implementations never
// return implausible items.
type FoodSource interface {
	// Search returns up to limit foods matching free text, source order.
	Search(ctx context.Context, query string, limit int) ([]model.Food, error)
	// LookupBarcode returns the product for a 14-digit GTIN; found is false
	// when the source has no usable product for it.
	LookupBarcode(ctx context.Context, gtin14 string) (food *model.Food, found bool, err error)
}

// ErrSourceUnavailable says the source could not be asked (no key, network,
// timeout, 429, 5xx). Callers degrade rather than fail.
var ErrSourceUnavailable = errors.New("food source unavailable")

// BudgetSpentError says the per-minute outbound budget for a source is used
// up; RetryAfter is how long until a slot frees.
type BudgetSpentError struct{ RetryAfter time.Duration }

func (e *BudgetSpentError) Error() string {
	return fmt.Sprintf("food source budget spent; retry in %s", e.RetryAfter.Round(time.Second))
}

// foodSourceTimeout bounds every outbound call so a slow source never holds a
// request past the client's patience.
const foodSourceTimeout = 3 * time.Second

func newFoodSourceClient() *http.Client {
	return &http.Client{Timeout: foodSourceTimeout}
}

// requestBudget is a mutex-guarded sliding window: at most limit requests in
// any window. It is in-process by design (see research.md); the API runs as
// one instance per environment.
type requestBudget struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	starts []time.Time
}

func newRequestBudget(limit int, window time.Duration, now func() time.Time) *requestBudget {
	return &requestBudget{limit: limit, window: window, now: now}
}

// take reserves one slot or returns how long until the oldest slot expires.
func (b *requestBudget) take() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	kept := b.starts[:0]
	for _, t := range b.starts {
		if now.Sub(t) < b.window {
			kept = append(kept, t)
		}
	}
	b.starts = kept
	if len(b.starts) >= b.limit {
		return &BudgetSpentError{RetryAfter: b.window - now.Sub(b.starts[0])}
	}
	b.starts = append(b.starts, now)
	return nil
}

// sourceStatusError maps an upstream status to ErrSourceUnavailable without
// echoing the body, which may be large or contain the key in an error echo.
func sourceStatusError(source string, status int) error {
	return fmt.Errorf("%s returned %d: %w", source, status, ErrSourceUnavailable)
}
