package ctxmutex

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// CtxMutex is a context aware Mutex
// It uses a semaphore to allow for context cancellation (comes with the semaphore package)
// The semaphore is initialized with a weight of 1, which means that only it acts as a mutex
type CtxMutex struct {
	sem *semaphore.Weighted
}

func NewCtxMutex() *CtxMutex {
	return &CtxMutex{
		sem: semaphore.NewWeighted(1),
	}
}

// Lock locks the mutex
func (m *CtxMutex) Lock(ctx context.Context) error {
	return m.sem.Acquire(ctx, 1)
}

// Unlock unlocks the mutex
func (m *CtxMutex) Unlock() {
	m.sem.Release(1)
}
