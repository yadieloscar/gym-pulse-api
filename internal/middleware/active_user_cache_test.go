package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeClock is a settable time source for the cache.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func newTestCache(checker ActiveUserChecker, ttl time.Duration) (*ActiveUserCache, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	cache := NewActiveUserCache(checker, ttl)
	cache.now = clock.Now
	return cache, clock
}

func TestActiveUserCacheTrustsAConfirmedIdentityForTheTTL(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	checker := &activeUserCheckerStub{exists: true}
	cache, clock := newTestCache(checker, DefaultActiveUserCacheTTL)

	for range 9 {
		exists, err := cache.Exists(ctx, userID)
		if err != nil || !exists {
			t.Fatalf("Exists() = %v, %v; want true, nil", exists, err)
		}
	}
	if checker.calls != 1 {
		t.Fatalf("a burst of 9 requests made %d lookups, want 1", checker.calls)
	}

	clock.now = clock.now.Add(DefaultActiveUserCacheTTL - time.Millisecond)
	if _, err := cache.Exists(ctx, userID); err != nil || checker.calls != 1 {
		t.Fatalf("inside the TTL: lookups = %d, err = %v; want 1, nil", checker.calls, err)
	}

	// Deleted outside the API: the next lookup after the TTL sees it.
	checker.exists = false
	clock.now = clock.now.Add(time.Millisecond)
	exists, err := cache.Exists(ctx, userID)
	if err != nil || exists {
		t.Fatalf("after the TTL Exists() = %v, %v; want false, nil", exists, err)
	}
	if checker.calls != 2 {
		t.Fatalf("lookups = %d, want 2", checker.calls)
	}
}

func TestActiveUserCacheNeverStoresAMissingIdentityOrAnError(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	checker := &activeUserCheckerStub{exists: false}
	cache, _ := newTestCache(checker, DefaultActiveUserCacheTTL)

	for range 3 {
		if exists, _ := cache.Exists(ctx, userID); exists {
			t.Fatal("a missing identity was reported as existing")
		}
	}
	if checker.calls != 3 {
		t.Fatalf("missing identity: lookups = %d, want 3", checker.calls)
	}

	checker.exists, checker.err = true, errors.New("database unavailable")
	for range 2 {
		if _, err := cache.Exists(ctx, userID); err == nil {
			t.Fatal("a failed lookup returned no error")
		}
	}
	if checker.calls != 5 {
		t.Fatalf("failed lookups: lookups = %d, want 5", checker.calls)
	}
}

func TestActiveUserCacheForgetAsksAgain(t *testing.T) {
	ctx, userID, other := context.Background(), uuid.New(), uuid.New()
	checker := &activeUserCheckerStub{exists: true}
	cache, _ := newTestCache(checker, DefaultActiveUserCacheTTL)

	_, _ = cache.Exists(ctx, userID)
	_, _ = cache.Exists(ctx, other)
	cache.Forget(userID)
	checker.exists = false

	if exists, _ := cache.Exists(ctx, userID); exists {
		t.Fatal("a forgotten identity was still trusted")
	}
	if exists, _ := cache.Exists(ctx, other); !exists {
		t.Fatal("forgetting one user dropped another")
	}
	if checker.calls != 3 {
		t.Fatalf("lookups = %d, want 3", checker.calls)
	}
}

// forgettingChecker forgets the user while its lookup is in flight, like an
// account deletion that runs during another request's lookup.
type forgettingChecker struct {
	cache *ActiveUserCache
	calls int
}

func (c *forgettingChecker) Exists(_ context.Context, userID uuid.UUID) (bool, error) {
	c.calls++
	if c.calls == 1 {
		c.cache.Forget(userID)
	}
	return true, nil
}

func TestActiveUserCacheDropsALookupThatSpansAForget(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	checker := &forgettingChecker{}
	cache, _ := newTestCache(checker, DefaultActiveUserCacheTTL)
	checker.cache = cache

	_, _ = cache.Exists(ctx, userID)
	_, _ = cache.Exists(ctx, userID)
	if checker.calls != 2 {
		t.Fatalf("lookups = %d, want 2: an answer from before the Forget was stored", checker.calls)
	}
	_, _ = cache.Exists(ctx, userID)
	if checker.calls != 2 {
		t.Fatalf("lookups = %d, want 2: the answer after the Forget was not stored", checker.calls)
	}
}

func TestActiveUserCacheWithoutTTLAlwaysAsks(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	checker := &activeUserCheckerStub{exists: true}
	cache, _ := newTestCache(checker, 0)

	_, _ = cache.Exists(ctx, userID)
	_, _ = cache.Exists(ctx, userID)
	if checker.calls != 2 {
		t.Fatalf("lookups = %d, want 2", checker.calls)
	}
}

func TestActiveUserCacheStaysBounded(t *testing.T) {
	ctx := context.Background()
	checker := &activeUserCheckerStub{exists: true}
	cache, clock := newTestCache(checker, DefaultActiveUserCacheTTL)

	for range maxActiveUserCacheEntries {
		_, _ = cache.Exists(ctx, uuid.New())
	}
	full := uuid.New()
	_, _ = cache.Exists(ctx, full)
	_, _ = cache.Exists(ctx, full)
	if len(cache.expires) != maxActiveUserCacheEntries {
		t.Fatalf("entries = %d, want %d", len(cache.expires), maxActiveUserCacheEntries)
	}
	if checker.calls != maxActiveUserCacheEntries+2 {
		t.Fatalf("a full cache stored a new answer: lookups = %d", checker.calls)
	}

	// Once the old answers expire, they make room.
	clock.now = clock.now.Add(DefaultActiveUserCacheTTL)
	_, _ = cache.Exists(ctx, full)
	if len(cache.expires) != 1 {
		t.Fatalf("entries after expiry = %d, want 1", len(cache.expires))
	}
}

func TestRequireActiveUserWithCacheRejectsADeletedAccountAtOnce(t *testing.T) {
	userID := uuid.New()
	checker := &activeUserCheckerStub{exists: true}
	cache, _ := newTestCache(checker, DefaultActiveUserCacheTTL)
	request := func(method, path string) *http.Request {
		req := httptest.NewRequest(method, path, nil)
		return req.WithContext(context.WithValue(req.Context(), UserIDKey, userID.String()))
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	guarded := RequireActiveUser(cache)(ok)
	deleteAccount := ForgetActiveUser(cache)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		checker.exists = false // the handler removes the identity
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, request(http.MethodGet, "/api/v1/settings"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("before deletion: status = %d", rec.Code)
	}
	deleteAccount.ServeHTTP(httptest.NewRecorder(), request(http.MethodDelete, "/api/v1/account"))

	rec = httptest.NewRecorder()
	guarded.ServeHTTP(rec, request(http.MethodGet, "/api/v1/settings"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("after deletion: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestForgetActiveUserWithoutAForgetterPassesThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	ForgetActiveUser(nil)(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/v1/account", nil))
	if !called {
		t.Fatal("the route did not run")
	}
}
