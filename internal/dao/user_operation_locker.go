package dao

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errAdvisoryLockNotHeld = errors.New("advisory lock was not held by lock connection")

// UserOperationLocker serializes privacy-sensitive operations for one user
// across every API replica. A PostgreSQL session advisory lock is held across
// the storage and auth-provider calls as well as application database writes.
type UserOperationLocker struct {
	lockPool advisoryLockPool
}

type advisoryLockPool interface {
	Acquire(context.Context) (advisoryLockConnection, error)
}

type advisoryLockConnection interface {
	Lock(context.Context, int64) error
	Unlock(context.Context, int64) (bool, error)
	Release()
	Discard(context.Context) error
}

type pgxAdvisoryLockPool struct {
	pool *pgxpool.Pool
}

func (p pgxAdvisoryLockPool) Acquire(ctx context.Context) (advisoryLockConnection, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return &pgxAdvisoryLockConnection{conn: conn}, nil
}

type pgxAdvisoryLockConnection struct {
	conn *pgxpool.Conn
}

func (c *pgxAdvisoryLockConnection) Lock(ctx context.Context, key int64) error {
	_, err := c.conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, key)
	return err
}

func (c *pgxAdvisoryLockConnection) Unlock(ctx context.Context, key int64) (bool, error) {
	var unlocked bool
	if err := c.conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, key).Scan(&unlocked); err != nil {
		return false, err
	}
	return unlocked, nil
}

func (c *pgxAdvisoryLockConnection) Release() {
	c.conn.Release()
}

func (c *pgxAdvisoryLockConnection) Discard(ctx context.Context) error {
	return c.conn.Hijack().Close(ctx)
}

// NewUserOperationLocker requires a pool dedicated to advisory locks. Locked
// operations use the application's separate query pool, so sharing that pool
// here would allow concurrent operations to exhaust it and deadlock.
func NewUserOperationLocker(lockPool *pgxpool.Pool) *UserOperationLocker {
	return newUserOperationLocker(pgxAdvisoryLockPool{pool: lockPool})
}

func newUserOperationLocker(lockPool advisoryLockPool) *UserOperationLocker {
	return &UserOperationLocker{lockPool: lockPool}
}

func userOperationLockKey(userID uuid.UUID) int64 {
	return int64(binary.BigEndian.Uint64(userID[:8]))
}

func (l *UserOperationLocker) WithUserLock(
	ctx context.Context,
	userID uuid.UUID,
	operation func(context.Context) error,
) (resultErr error) {
	conn, err := l.lockPool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring user operation connection: %w", err)
	}

	key := userOperationLockKey(userID)
	if err := conn.Lock(ctx, key); err != nil {
		return errors.Join(
			fmt.Errorf("locking user operation: %w", err),
			discardAdvisoryLockConnection(ctx, conn),
		)
	}

	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		unlocked, unlockErr := conn.Unlock(unlockCtx, key)
		if unlockErr != nil || !unlocked {
			// Never return a pooled session that may still own an advisory lock.
			if unlockErr == nil {
				unlockErr = errAdvisoryLockNotHeld
			}
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("unlocking user operation: %w", unlockErr),
				conn.Discard(unlockCtx),
			)
			return
		}
		conn.Release()
	}()

	return operation(ctx)
}

func discardAdvisoryLockConnection(ctx context.Context, conn advisoryLockConnection) error {
	discardCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := conn.Discard(discardCtx); err != nil {
		return fmt.Errorf("discarding user operation connection: %w", err)
	}
	return nil
}
