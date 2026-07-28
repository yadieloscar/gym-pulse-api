package dao

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type advisoryLockPoolStub struct {
	conn     advisoryLockConnection
	err      error
	acquires int
}

func (p *advisoryLockPoolStub) Acquire(context.Context) (advisoryLockConnection, error) {
	p.acquires++
	return p.conn, p.err
}

type advisoryLockConnectionStub struct {
	lockErr, unlockErr, discardErr error
	unlocked                       bool
	lockKey, unlockKey             int64
	released, discarded            bool
}

func (c *advisoryLockConnectionStub) Lock(_ context.Context, key int64) error {
	c.lockKey = key
	return c.lockErr
}

func (c *advisoryLockConnectionStub) Unlock(_ context.Context, key int64) (bool, error) {
	c.unlockKey = key
	return c.unlocked, c.unlockErr
}

func (c *advisoryLockConnectionStub) Release() {
	c.released = true
}

func (c *advisoryLockConnectionStub) Discard(context.Context) error {
	c.discarded = true
	return c.discardErr
}

func TestUserOperationLockerWithUserLock(t *testing.T) {
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")

	t.Run("runs operation and releases only after confirmed unlock", func(t *testing.T) {
		conn := &advisoryLockConnectionStub{unlocked: true}
		pool := &advisoryLockPoolStub{conn: conn}
		locker := newUserOperationLocker(pool)
		called := false

		err := locker.WithUserLock(context.Background(), userID, func(context.Context) error {
			called = true
			if conn.released {
				t.Fatal("connection released before operation completed")
			}
			return nil
		})

		if err != nil {
			t.Fatal(err)
		}
		if !called || pool.acquires != 1 || !conn.released || conn.discarded {
			t.Fatalf("called=%t acquires=%d released=%t discarded=%t", called, pool.acquires, conn.released, conn.discarded)
		}
		if conn.lockKey != userOperationLockKey(userID) || conn.unlockKey != conn.lockKey {
			t.Fatalf("lock keys differ: acquire=%d release=%d", conn.lockKey, conn.unlockKey)
		}
	})

	t.Run("preserves operation failure after a successful unlock", func(t *testing.T) {
		operationErr := errors.New("operation failed")
		conn := &advisoryLockConnectionStub{unlocked: true}
		locker := newUserOperationLocker(&advisoryLockPoolStub{conn: conn})

		err := locker.WithUserLock(context.Background(), userID, func(context.Context) error {
			return operationErr
		})

		if !errors.Is(err, operationErr) || !conn.released || conn.discarded {
			t.Fatalf("err=%v released=%t discarded=%t", err, conn.released, conn.discarded)
		}
	})

	t.Run("acquisition failure does not invoke operation", func(t *testing.T) {
		acquireErr := errors.New("pool exhausted")
		pool := &advisoryLockPoolStub{err: acquireErr}
		locker := newUserOperationLocker(pool)
		called := false

		err := locker.WithUserLock(context.Background(), userID, func(context.Context) error {
			called = true
			return nil
		})

		if !errors.Is(err, acquireErr) || called || pool.acquires != 1 {
			t.Fatalf("err=%v called=%t acquires=%d", err, called, pool.acquires)
		}
	})

	t.Run("lock failure discards ambiguous session", func(t *testing.T) {
		lockErr := errors.New("lock query canceled")
		conn := &advisoryLockConnectionStub{lockErr: lockErr}
		locker := newUserOperationLocker(&advisoryLockPoolStub{conn: conn})

		err := locker.WithUserLock(context.Background(), userID, func(context.Context) error {
			t.Fatal("operation ran without a lock")
			return nil
		})

		if !errors.Is(err, lockErr) || !conn.discarded || conn.released {
			t.Fatalf("err=%v released=%t discarded=%t", err, conn.released, conn.discarded)
		}
	})

	t.Run("false unlock result is an invariant failure and discards session", func(t *testing.T) {
		conn := &advisoryLockConnectionStub{unlocked: false}
		locker := newUserOperationLocker(&advisoryLockPoolStub{conn: conn})

		err := locker.WithUserLock(context.Background(), userID, func(context.Context) error {
			return nil
		})

		if err == nil || !strings.Contains(err.Error(), "advisory lock was not held") {
			t.Fatalf("err=%v", err)
		}
		if !conn.discarded || conn.released {
			t.Fatalf("released=%t discarded=%t", conn.released, conn.discarded)
		}
	})

	t.Run("unlock and discard failures join the operation error", func(t *testing.T) {
		operationErr := errors.New("operation failed")
		unlockErr := errors.New("unlock failed")
		discardErr := errors.New("close failed")
		conn := &advisoryLockConnectionStub{
			unlockErr:  unlockErr,
			discardErr: discardErr,
		}
		locker := newUserOperationLocker(&advisoryLockPoolStub{conn: conn})

		err := locker.WithUserLock(context.Background(), userID, func(context.Context) error {
			return operationErr
		})

		if !errors.Is(err, operationErr) || !errors.Is(err, unlockErr) || !errors.Is(err, discardErr) {
			t.Fatalf("joined error = %v", err)
		}
		if !conn.discarded || conn.released {
			t.Fatalf("released=%t discarded=%t", conn.released, conn.discarded)
		}
	})
}
