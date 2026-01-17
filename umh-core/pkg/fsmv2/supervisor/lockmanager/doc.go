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

// Package lockmanager provides lock ordering enforcement for FSMv2.
//
// # Overview
//
// The LockManager ensures locks are always acquired in a consistent order to
// prevent deadlocks. It uses a level-based ordering system where locks with
// lower levels must be acquired before locks with higher levels.
//
// # Why Lock Ordering Matters
//
// Without consistent lock ordering, deadlocks can occur:
//
// Deadlock scenario (without ordering):
//
//	Goroutine A                Goroutine B
//	-----------                -----------
//	Lock(Supervisor.mu)        Lock(WorkerContext.mu)
//	...                        ...
//	Lock(WorkerContext.mu)     Lock(Supervisor.mu)
//	   ^ BLOCKED                   ^ BLOCKED
//	   = DEADLOCK                  = DEADLOCK
//
// With lock ordering (level system):
//
//	Goroutine A                Goroutine B
//	-----------                -----------
//	Lock(Supervisor.mu)        Lock(Supervisor.mu)     // Must wait
//	Lock(WorkerContext.mu)          ^ BLOCKED (waits for A)
//	Unlock(WorkerContext.mu)   Lock(WorkerContext.mu)  // Now allowed
//	Unlock(Supervisor.mu)      Unlock(WorkerContext.mu)
//	   = NO DEADLOCK              Unlock(Supervisor.mu)
//
// # Lock Level Hierarchy
//
// FSMv2 uses the following lock levels (lower = acquire first):
//
//	Level 0: Supervisor.mu       - Top-level supervisor state
//	Level 1: ctxMu               - Context map modifications
//	Level 2: WorkerContext.mu    - Per-worker state
//	Level 3: Collector.mu        - Per-worker collection
//	Level 4: ActionExecutor.mu   - Per-worker action execution
//
// Rule: Always acquire locks in ascending level order.
//
// # Mandatory Ordering: Supervisor.mu → WorkerContext.mu
//
// The most important ordering is Supervisor.mu before WorkerContext.mu because:
//
// 1. Supervisor iterates workers: The supervisor's tick loop iterates over
// worker contexts. It must hold Supervisor.mu to prevent concurrent modification.
//
// 2. Worker modifications: When modifying a specific worker, we need
// WorkerContext.mu. If we already hold Supervisor.mu from iteration, we must
// acquire WorkerContext.mu second (not first).
//
// 3. Deadlock potential: If WorkerContext.mu could be acquired before
// Supervisor.mu, we'd have deadlock scenarios between tick loop and worker
// operations.
//
// # Why Environment Variable Control?
//
// Lock order checking is controlled by ENABLE_LOCK_ORDER_CHECKS=1 because:
//
// 1. Production Performance: Checking lock order requires per-goroutine tracking
// and runtime stack introspection. This adds overhead (microseconds per lock).
//
// 2. Development Safety: During development and testing, enable checks to catch
// ordering violations early. Panics on violation produce full stack traces for debugging.
//
// 3. CI Enforcement: CI runs with checks enabled. Any ordering violation fails
// the build before reaching production.
//
// In production: ENABLE_LOCK_ORDER_CHECKS="" (disabled, no overhead)
// In development: ENABLE_LOCK_ORDER_CHECKS=1 (enabled, catches violations)
// In CI: ENABLE_LOCK_ORDER_CHECKS=1 (enabled, enforces ordering)
//
// # Why Panic on Violation?
//
// The LockManager panics on lock order violations because:
//
// 1. Immediate Feedback: Developers see the violation immediately in testing,
// with a full stack trace showing exactly where the bad ordering occurred.
//
// 2. No Silent Failures: A log message could be missed. A panic cannot be.
//
// 3. Fail-Fast: Better to crash during testing than deadlock in production.
//
// 4. Debugging Context: The panic message includes:
//   - Which locks are involved
//   - Their respective levels
//   - The expected ordering
//   - Full stack trace
//
// # Detection Mechanism
//
// The LockManager tracks per-goroutine lock acquisitions:
//
// 1. Each goroutine has a list of currently held locks (name + level)
// 2. On Lock(), check if any held lock has a HIGHER level
// 3. If yes, panic (would violate ordering)
// 4. If no, record the new lock and proceed
// 5. On Unlock(), remove the lock from the held list
//
// # Usage
//
//	manager := lockmanager.NewLockManager()
//
//	// Create locks with levels
//	supervisorMu := manager.NewLock("Supervisor.mu", 0)
//	workerMu := manager.NewLock("WorkerContext.mu", 2)
//
//	// Correct ordering (level 0 before level 2)
//	supervisorMu.Lock()
//	workerMu.Lock()
//	// ... do work ...
//	workerMu.Unlock()
//	supervisorMu.Unlock()
//
//	// Incorrect ordering (level 2 before level 0) - panics if checks enabled
//	workerMu.Lock()
//	supervisorMu.Lock()  // PANIC: lock order violation
//
// # Testing Lock Order
//
// Run tests with lock order checks enabled:
//
//	ENABLE_LOCK_ORDER_CHECKS=1 ginkgo -r ./pkg/fsmv2/supervisor
//
// Any lock order violation will cause a panic with:
//   - The violating lock acquisition
//   - The higher-level lock already held
//   - Full stack trace
//
// # Integration with Supervisor
//
// The supervisor creates locks at initialization:
//
//	s := &Supervisor{
//	    lockManager: lockmanager.NewLockManager(),
//	}
//	s.mu = s.lockManager.NewLock("Supervisor.mu", 0)
//
// All lock operations go through the LockManager for ordering validation.
package lockmanager
