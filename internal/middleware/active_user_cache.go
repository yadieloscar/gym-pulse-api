package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DefaultActiveUserCacheTTL is how long a confirmed identity is trusted
// without asking the database again. An identity deleted outside this API
// (for example in the Supabase dashboard) keeps access for at most this long
// on each server process; deletion through DELETE /api/v1/account takes
// effect at once because that route forgets the user.
const DefaultActiveUserCacheTTL = 30 * time.Second

// maxActiveUserCacheEntries bounds the cache's memory. Past it, expired
// entries are dropped; if none have expired, the answer is not stored.
const maxActiveUserCacheEntries = 10000

// ActiveUserCache remembers positive answers of an ActiveUserChecker for a
// short time, so a burst of requests from one athlete costs one identity
// lookup instead of one per request. Negative answers and errors are never
// stored: a missing identity or a failing check is asked again each time.
type ActiveUserCache struct {
	checker ActiveUserChecker
	ttl     time.Duration
	now     func() time.Time

	mu      sync.Mutex
	expires map[uuid.UUID]time.Time
	// generation changes on every Forget. A lookup that began before a Forget
	// does not store its answer, so a deletion cannot be undone by a request
	// that was already in flight.
	generation uint64
}

// NewActiveUserCache wraps checker. A non-positive ttl disables caching.
func NewActiveUserCache(checker ActiveUserChecker, ttl time.Duration) *ActiveUserCache {
	return &ActiveUserCache{checker: checker, ttl: ttl, now: time.Now, expires: map[uuid.UUID]time.Time{}}
}

// Exists implements ActiveUserChecker.
func (c *ActiveUserCache) Exists(ctx context.Context, userID uuid.UUID) (bool, error) {
	c.mu.Lock()
	now := c.now()
	if until, ok := c.expires[userID]; ok && now.Before(until) {
		c.mu.Unlock()
		return true, nil
	}
	generation := c.generation
	c.mu.Unlock()

	exists, err := c.checker.Exists(ctx, userID)
	if err != nil || !exists || c.ttl <= 0 {
		return exists, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return true, nil
	}
	if len(c.expires) >= maxActiveUserCacheEntries {
		for id, until := range c.expires {
			if !now.Before(until) {
				delete(c.expires, id)
			}
		}
		if len(c.expires) >= maxActiveUserCacheEntries {
			return true, nil
		}
	}
	c.expires[userID] = now.Add(c.ttl)
	return true, nil
}

// Forget drops userID's stored answer and discards any lookup in flight, so
// the next request asks the database.
func (c *ActiveUserCache) Forget(userID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.expires, userID)
	c.generation++
}

// ActiveUserForgetter is implemented by a checker that caches answers.
type ActiveUserForgetter interface {
	Forget(userID uuid.UUID)
}

// ForgetActiveUser makes the wrapped route forget the caller's cached
// identity before and after it runs. It guards account deletion: no request
// may ride on an answer stored before the identity was removed.
func ForgetActiveUser(forgetter ActiveUserForgetter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if forgetter == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := activeUserID(r.Context())
			if ok {
				forgetter.Forget(userID)
				defer forgetter.Forget(userID)
			}
			next.ServeHTTP(w, r)
		})
	}
}
