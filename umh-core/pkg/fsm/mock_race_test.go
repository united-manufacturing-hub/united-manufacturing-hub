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

package fsm

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("MockFSMManager", func() {
	Describe("thread safety", func() {
		// These tests verify that concurrent writes to ReconcileDelay/ReconcileError
		// and reads in Reconcile() are thread-safe.
		//
		// Tests should PASS with the race detector enabled (-race flag) because:
		// - Reconcile() acquires mutex and reads ReconcileDelay and ReconcileError
		// - SetReconcileDelay/SetReconcileError and WithReconcileDelay/WithReconcileError
		//   all acquire the mutex before writing
		//
		// This is the GREEN phase of TDD - proving thread-safety is implemented.

		It("should handle concurrent Reconcile calls and field modifications", func() {
			mock := NewMockFSMManager()

			// Use a context with timeout to ensure goroutines stay alive during writes
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var wg sync.WaitGroup

			// Start multiple goroutines calling Reconcile() concurrently
			// These goroutines will read ReconcileDelay and ReconcileError inside the mutex
			numReaders := 10
			for range numReaders {
				wg.Add(1)

				go func() {
					defer wg.Done()

					for {
						select {
						case <-ctx.Done():
							return
						default:
							// Reconcile() locks mutex and reads ReconcileDelay and ReconcileError
							_, _ = mock.Reconcile(ctx, SystemSnapshot{}, nil)
						}
					}
				}()
			}

			// Main goroutine writes to fields using thread-safe setter methods
			// This should NOT create a race condition with the reads inside Reconcile()
			numWrites := 1000
			for i := range numWrites {
				// Use thread-safe setter methods
				mock.SetReconcileDelay(time.Duration(i) * time.Microsecond)
				mock.SetReconcileError(errors.New("test error"))

				// Also test the builder methods which now acquire mutex
				mock.WithReconcileDelay(time.Duration(i) * time.Microsecond)
				mock.WithReconcileError(errors.New("another error"))

				// Small sleep to ensure overlap with Reconcile() calls
				time.Sleep(10 * time.Microsecond)
			}

			// Cancel context to stop reader goroutines
			cancel()
			wg.Wait()
		})

		It("should handle concurrent modification from multiple goroutines", func() {
			mock := NewMockFSMManager()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			var wg sync.WaitGroup

			// Goroutine 1: Continuously calls Reconcile()
			wg.Add(1)

			go func() {
				defer wg.Done()

				for {
					select {
					case <-ctx.Done():
						return
					default:
						_, _ = mock.Reconcile(ctx, SystemSnapshot{}, nil)
					}
				}
			}()

			// Goroutine 2: Continuously modifies ReconcileDelay using thread-safe setter
			wg.Add(1)

			go func() {
				defer wg.Done()

				for i := 0; ; i++ {
					select {
					case <-ctx.Done():
						return
					default:
						// Use thread-safe setter method
						mock.SetReconcileDelay(time.Duration(i%100) * time.Millisecond)
					}
				}
			}()

			// Goroutine 3: Continuously modifies ReconcileError using thread-safe setter
			wg.Add(1)

			go func() {
				defer wg.Done()

				for i := 0; ; i++ {
					select {
					case <-ctx.Done():
						return
					default:
						// Use thread-safe setter method
						if i%2 == 0 {
							mock.SetReconcileError(errors.New("error"))
						} else {
							mock.SetReconcileError(nil)
						}
					}
				}
			}()

			// Wait for context timeout
			<-ctx.Done()
			wg.Wait()
		})
	})
})
