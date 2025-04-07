// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ctxrwmutex

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// CtxRWMutex is a context aware RWMutex
// It uses a semaphore to a) allow for multiple readers and b) allow for context cancellation (comes with the semaphore package)
// The semaphore is initialized with a weight of 100, which means that 100 readers can read the mutex at the same time
// If the semaphore is locked by a writer, no readers can read the mutex
// If the semaphore is locked by a reader, no writers can write the mutex but multiple (up to 100) readers can read the mutex at the same time
type CtxRWMutex struct {
	sem *semaphore.Weighted
}

func NewCtxRWMutex() *CtxRWMutex {
	return &CtxRWMutex{
		sem: semaphore.NewWeighted(100),
	}
}

// RLock locks the mutex for reading
func (m *CtxRWMutex) RLock(ctx context.Context) error {
	return m.sem.Acquire(ctx, 1)
}

// RUnlock unlocks the mutex for reading
func (m *CtxRWMutex) RUnlock() {
	m.sem.Release(1)
}

// Lock locks the mutex for writing
func (m *CtxRWMutex) Lock(ctx context.Context) error {
	return m.sem.Acquire(ctx, 100)
}

// Unlock unlocks the mutex for writing
func (m *CtxRWMutex) Unlock() {
	m.sem.Release(100)
}
