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

package supervisor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Phase 2.2: Runtime Lock Order Assertions (TDD Approach)
//
// These tests verify that runtime lock order violations are detected at development time.
// They follow the RED-GREEN-REFACTOR cycle:
//   1. RED: Write failing tests (tests expect panic on violations)
//   2. GREEN: Implement checkLockOrder() to make tests pass
//   3. REFACTOR: Optimize and clean up
//
// Environment Variable: ENABLE_LOCK_ORDER_CHECKS=1 (development only)
//
// Why internal package:
// These tests need direct access to Supervisor and WorkerContext types
// to test lock acquisition patterns. They cannot be in supervisor_test
// package because those types are not exported.
var _ = Describe("Runtime Lock Order Assertions", func() {
	Describe("Correct Lock Order (should NOT panic)", func() {
		It("should allow Supervisor.mu alone", func() {
			// This is always safe - no ordering concerns
			Expect(func() {
				s := &Supervisor{}
				s.lockMu()
				defer s.unlockMu()
				// Safe: Only one lock held
			}).ToNot(Panic())
		})

		It("should allow Supervisor.ctxMu alone", func() {
			// ctxMu is independent and can be acquired alone
			Expect(func() {
				s := &Supervisor{}
				s.lockCtxMu()
				defer s.unlockCtxMu()
				// Safe: Independent lock
			}).ToNot(Panic())
		})

		It("should allow WorkerContext.mu alone", func() {
			// Worker locks are independent per worker
			Expect(func() {
				wc := &WorkerContext{}
				wc.lockMu()
				defer wc.unlockMu()
				// Safe: Per-worker lock
			}).ToNot(Panic())
		})

		It("should allow Supervisor.mu → WorkerContext.mu (correct order)", func() {
			// This is the CORRECT order as documented
			Expect(func() {
				s := &Supervisor{}
				wc := &WorkerContext{}

				s.lockMu()
				// Correct: Supervisor lock first
				wc.lockMu()
				wc.unlockMu()
				s.unlockMu()
			}).ToNot(Panic())
		})

		It("should allow Supervisor.mu → Supervisor.ctxMu (advisory order)", func() {
			// Advisory order (both rarely needed together)
			Expect(func() {
				s := &Supervisor{}

				s.lockMu()
				s.lockCtxMu()
				s.unlockCtxMu()
				s.unlockMu()
			}).ToNot(Panic())
		})

		It("should allow acquiring and releasing locks in sequence", func() {
			// Safe: Locks released before acquiring others
			Expect(func() {
				s := &Supervisor{}
				wc := &WorkerContext{}

				s.lockMu()
				s.unlockMu()

				// Safe: First lock released before second acquired
				wc.lockMu()
				wc.unlockMu()
			}).ToNot(Panic())
		})

		It("should allow multiple workers acquiring their own locks concurrently", func() {
			// Worker locks are independent - no ordering between different workers
			Expect(func() {
				wc1 := &WorkerContext{}
				wc2 := &WorkerContext{}

				done := make(chan struct{}, 2)

				go func() {
					wc1.lockMu()
					defer wc1.unlockMu()
					done <- struct{}{}
				}()

				go func() {
					wc2.lockMu()
					defer wc2.unlockMu()
					done <- struct{}{}
				}()

				<-done
				<-done
			}).ToNot(Panic())
		})
	})

	Describe("Lock Order Violations (SHOULD panic)", func() {
		It("should panic when WorkerContext.mu acquired before Supervisor.mu", func() {
			// This violates Rule 1: Supervisor.mu must be acquired first
			Expect(func() {
				s := &Supervisor{}
				wc := &WorkerContext{}

				wc.lockMu()
				// WRONG ORDER: WorkerContext lock held, trying to acquire Supervisor lock
				s.lockMu()
				s.unlockMu()
				wc.unlockMu()
			}).To(PanicWith(ContainSubstring("lock order violation")))
		})

		It("should panic with clear error message showing which locks", func() {
			// Verify panic message is helpful for debugging
			Expect(func() {
				s := &Supervisor{}
				wc := &WorkerContext{}

				wc.lockMu()
				s.lockMu() // Wrong order
				s.unlockMu()
				wc.unlockMu()
			}).To(PanicWith(And(
				ContainSubstring("lock order violation"),
				ContainSubstring("Supervisor.mu"),
				ContainSubstring("WorkerContext.mu"),
			)))
		})

		It("should include goroutine ID in panic message", func() {
			// Helps identify which goroutine caused the violation
			Expect(func() {
				s := &Supervisor{}
				wc := &WorkerContext{}

				wc.lockMu()
				s.lockMu() // Wrong order
				s.unlockMu()
				wc.unlockMu()
			}).To(PanicWith(ContainSubstring("goroutine")))
		})
	})

	Describe("Edge Cases", func() {
		It("should handle read locks (RLock) same as write locks for ordering", func() {
			// Read locks still participate in lock ordering
			Expect(func() {
				s := &Supervisor{}
				wc := &WorkerContext{}

				s.rLockMu()
				wc.rLockMu()
				wc.rUnlockMu()
				s.rUnlockMu()
			}).ToNot(Panic())
		})

		It("should panic even with read locks in wrong order", func() {
			Expect(func() {
				s := &Supervisor{}
				wc := &WorkerContext{}

				wc.rLockMu()
				s.rLockMu() // Still wrong order
				s.rUnlockMu()
				wc.rUnlockMu()
			}).To(PanicWith(ContainSubstring("lock order violation")))
		})

		It("should allow re-acquiring same lock if properly released", func() {
			// Locks can be acquired multiple times if properly released each time
			Expect(func() {
				s := &Supervisor{}

				s.lockMu()
				s.unlockMu()

				s.lockMu()
				s.unlockMu()
			}).ToNot(Panic())
		})
	})

	Describe("Environment Variable Gating", func() {
		It("should be disabled by default (no env var set)", func() {
			// Without ENABLE_LOCK_ORDER_CHECKS, assertions should be no-op
			// This test documents that behavior but can't easily test it
			// since we can't un-set env vars in running process
			Skip("Cannot test env var behavior in same process")
		})
	})
})
