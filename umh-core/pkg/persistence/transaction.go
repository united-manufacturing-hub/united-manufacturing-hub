package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// WithTransaction runs fn within a transaction.
// Automatically commits on success, rolls back on error or panic.
//
// DESIGN DECISION: Panic recovery with rollback
// WHY: Panics in transaction function should not leave transaction uncommitted.
// Recover from panic, rollback transaction, re-panic to preserve stack trace.
//
// TRADE-OFF: Hides panic briefly during rollback, but ensures cleanup.
// Alternative would be to let panic propagate without cleanup, risking leaked transactions.
//
// INSPIRED BY: database/sql.Tx defer pattern, GORM's Transaction helper.
//
// DESIGN DECISION: Context cancellation checking before commit
// WHY: Respect cancellation signals - don't commit work that should be abandoned.
// Check ctx.Err() before Commit to handle cancellation between fn completion and commit.
//
// TRADE-OFF: Adds overhead of context check, but prevents committing cancelled work.
//
// INSPIRED BY: Go context best practices, SQL transaction timeout patterns.
//
// Example:
//
//	err := basic.WithTransaction(ctx, store, func(tx basic.Tx) error {
//	    tx.Insert(ctx, "users", user)
//	    tx.Insert(ctx, "audit_log", audit)
//	    return nil  // Commits automatically
//	})
//
// Example with error handling:
//
//	err := basic.WithTransaction(ctx, store, func(tx basic.Tx) error {
//	    if err := tx.Insert(ctx, "users", user); err != nil {
//	        return err  // Rolls back automatically
//	    }
//	    return nil
//	})
func WithTransaction(ctx context.Context, store Store, fn func(tx Tx) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	tx, err := store.BeginTx(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()

			panic(r)
		}
	}()

	err = fn(tx)
	if err != nil {
		_ = tx.Rollback()

		return err
	}

	if ctx.Err() != nil {
		_ = tx.Rollback()

		return ctx.Err()
	}

	return tx.Commit()
}

// WithRetry wraps WithTransaction with retry logic for optimistic locking conflicts.
//
// DESIGN DECISION: Exponential backoff for ErrConflict retries
// WHY: Optimistic locking conflicts (version mismatches) are transient.
// Retry with backoff gives other transactions time to complete.
// Linear backoff would be simpler but doesn't adapt to contention levels.
//
// TRADE-OFF: Adds latency on conflicts, but increases success rate.
// Alternative would be immediate retry, but that wastes CPU on high contention.
//
// INSPIRED BY: Linear's sync conflict resolution, Postgres retry strategies,
// TCP exponential backoff algorithm.
//
// DESIGN DECISION: Retry only ErrConflict, fail immediately on other errors
// WHY: Only version conflicts are worth retrying - they indicate another transaction
// succeeded and we should try again with new data. Other errors (network, validation,
// constraint violations) won't resolve with retry.
//
// TRADE-OFF: Network errors could also benefit from retry, but that adds complexity.
// Future enhancement: make retryable errors configurable.
//
// INSPIRED BY: HTTP 409 Conflict retry patterns, database deadlock retry strategies.
//
// Backoff strategy:
//   - Attempt 1: immediate
//   - Attempt 2: 10ms delay
//   - Attempt 3: 20ms delay
//   - Attempt 4: 40ms delay
//   - Attempt 5: 80ms delay
//   - Max: 5 retries (configurable via maxRetries parameter)
//
// Example:
//
//	err := basic.WithRetry(ctx, store, 5, func(tx basic.Tx) error {
//	    // Read-modify-write with version check
//	    doc, _ := tx.Get(ctx, "users", id)
//	    doc["count"] = doc["count"].(float64) + 1
//	    return tx.Update(ctx, "users", id, doc)  // May fail with ErrConflict
//	})
//
// Example with FSM state update:
//
//	err := basic.WithRetry(ctx, store, 5, func(tx basic.Tx) error {
//	    snapshot, _ := tx.Get(ctx, "container", workerID)
//	    snapshot["observed"] = newObservedState
//	    return tx.Update(ctx, "container", workerID, snapshot)
//	})
func WithRetry(ctx context.Context, store Store, maxRetries int, fn func(tx Tx) error) error {
	if maxRetries < 0 {
		return fmt.Errorf("maxRetries must be >= 0, got %d", maxRetries)
	}

	var lastErr error

	backoff := 10 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			backoff *= 2
		}

		lastErr = WithTransaction(ctx, store, fn)
		if lastErr == nil {
			return nil
		}

		if !errors.Is(lastErr, ErrConflict) {
			return lastErr
		}
	}

	return fmt.Errorf("transaction failed after %d retries: %w", maxRetries, lastErr)
}
