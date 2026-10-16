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
// # Lock ordering
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
//	Lock(Supervisor.mu)        // Must wait
//	Lock(WorkerContext.mu)          ^ BLOCKED (waits for A)
//	Unlock(WorkerContext.mu)   Lock(WorkerContext.mu)  // Now allowed
//	Unlock(Supervisor.mu)      Unlock(WorkerContext.mu)
//	   = NO DEADLOCK              Unlock(Supervisor.mu)
//
// # Lock level hierarchy
//
// FSMv2 uses the following lock levels (lower = acquire first):
//
//	Level 1: Supervisor.mu       - Workers map, children and global variables
//	Level 2: Supervisor.ctxMu    - Supervisor context and its cancel function
//	Level 3: WorkerContext.mu    - Per-worker current state
//
// The levels are the lockLevel constants in supervisor/supervisor.go.
//
// Rule: Always acquire locks in ascending level order.
//
// # Supervisor.mu before WorkerContext.mu
//
// The supervisor's tick loop iterates over worker contexts while holding
// Supervisor.mu to prevent concurrent modification. When modifying a specific
// worker, WorkerContext.mu must be acquired second. If WorkerContext.mu could
// be acquired first, deadlocks would occur between the tick loop and worker
// operations.
//
// # Environment variable control
//
// Lock order checking is controlled by ENABLE_LOCK_ORDER_CHECKS=1.
//
// Checking lock order requires per-goroutine tracking and runtime stack
// introspection, adding overhead (microseconds per lock). Checks are disabled
// unless the variable is set, so production runs without the overhead.
// No Makefile target or CI workflow sets it; only the lockmanager unit tests
// enable it. Set it yourself to check the supervisor's lock order.
//
// # Panic on violation
//
// The LockManager panics on lock order violations to provide immediate feedback
// with a full stack trace. A log message could be missed; a panic cannot.
// Crashing during testing is preferable to deadlocking in production.
//
// The panic message includes:
//   - Which locks are involved
//   - Their respective levels
//   - The expected ordering
//   - Full stack trace
//
// # Detection mechanism
//
// The LockManager tracks per-goroutine lock acquisitions:
//
// 1. Each goroutine has a list of held locks (name + level)
// 2. On Lock(), check if any held lock has a HIGHER level
// 3. If yes, panic (ordering violation)
// 4. If no, record the lock and proceed
// 5. On Unlock(), remove the lock from the held list
//
// # Usage
//
//	manager := lockmanager.NewLockManager()
//
//	// Create locks with levels
//	supervisorMu := manager.NewLock("Supervisor.mu", 1)
//	workerMu := manager.NewLock("WorkerContext.mu", 3)
//
//	// Correct ordering (level 1 before level 3)
//	supervisorMu.Lock()
//	workerMu.Lock()
//	// ... do work ...
//	workerMu.Unlock()
//	supervisorMu.Unlock()
//
//	// Incorrect ordering (level 3 before level 1) - panics if checks enabled
//	workerMu.Lock()
//	supervisorMu.Lock()  // PANIC: lock order violation
//
// # Testing lock order
//
// Run tests with lock order checks enabled:
//
//	ENABLE_LOCK_ORDER_CHECKS=1 ginkgo -r ./pkg/fsmv2/supervisor
//
// Any lock order violation causes a panic with:
//   - The violating lock acquisition
//   - The higher-level lock already held
//   - Full stack trace
//
// # Supervisor integration
//
// The supervisor creates locks at initialization:
//
//	lm := lockmanager.NewLockManager()
//	s := &Supervisor{
//	    lockManager: lm,
//	    mu:          lm.NewLock(lockNameSupervisorMu, lockLevelSupervisorMu),
//	    ctxMu:       lm.NewLock(lockNameSupervisorCtxMu, lockLevelSupervisorCtxMu),
//	}
//
// Each WorkerContext.mu is created in newWorkerContext with lockLevelWorkerContextMu.
//
// All lock operations go through the LockManager for ordering validation.
package lockmanager
