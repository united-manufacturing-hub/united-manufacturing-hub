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
