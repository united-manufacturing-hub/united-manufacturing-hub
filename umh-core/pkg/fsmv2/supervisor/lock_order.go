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
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Lock names for tracking
const (
	lockNameSupervisorMu    = "Supervisor.mu"
	lockNameSupervisorCtxMu = "Supervisor.ctxMu"
	lockNameWorkerContextMu = "WorkerContext.mu"
)

// Lock levels for ordering (lower number = must be acquired first)
const (
	lockLevelSupervisorMu    = 1
	lockLevelSupervisorCtxMu = 2 // Can be acquired alone OR after Supervisor.mu
	lockLevelWorkerContextMu = 3 // Must be acquired AFTER Supervisor.mu
)

// lockOrderTracker tracks which locks each goroutine holds.
// Only active when ENABLE_LOCK_ORDER_CHECKS=1.
type lockOrderTracker struct {
	mu    sync.RWMutex
	locks map[uint64][]heldLock // goroutine ID → held locks
}

// heldLock represents a lock held by a goroutine
type heldLock struct {
	name  string
	level int
}

// Global lock order tracker (development only)
var globalTracker = &lockOrderTracker{
	locks: make(map[uint64][]heldLock),
}

// lockOrderChecksEnabled returns true if lock order checking is enabled.
// Only enabled in development with ENABLE_LOCK_ORDER_CHECKS=1.
func lockOrderChecksEnabled() bool {
	return os.Getenv("ENABLE_LOCK_ORDER_CHECKS") == "1"
}

// getGoroutineID returns the ID of the current goroutine.
// This is a bit of a hack but is acceptable for development-only debugging.
func getGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Stack trace format: "goroutine 123 [running]:\n..."
	idField := strings.Fields(string(buf[:n]))[1]
	id, _ := strconv.ParseUint(idField, 10, 64)
	return id
}

// checkLockOrder verifies that acquiring lockName at lockLevel does not
// violate the documented lock ordering rules.
//
// Rules enforced:
//  1. Supervisor.mu (level 1) before WorkerContext.mu (level 3)
//  2. Supervisor.mu (level 1) before Supervisor.ctxMu (level 2) - advisory
//  3. WorkerContext.mu locks are independent per worker
//
// Panics if a violation is detected with details about which locks were held
// and the goroutine ID for debugging.
func checkLockOrder(lockName string, lockLevel int) {
	if !lockOrderChecksEnabled() {
		return
	}

	gid := getGoroutineID()
	globalTracker.mu.RLock()
	heldLocks := globalTracker.locks[gid]
	globalTracker.mu.RUnlock()

	// Check if any higher-level lock is held (violation)
	for _, held := range heldLocks {
		if held.level > lockLevel {
			// VIOLATION: Trying to acquire lower-level lock while holding higher-level lock
			stack := make([]byte, 4096)
			n := runtime.Stack(stack, false)

			panic(fmt.Sprintf(
				"lock order violation: goroutine %d attempting to acquire %s (level %d) while holding %s (level %d)\n"+
					"Expected order: %s before %s\n"+
					"Stack trace:\n%s",
				gid,
				lockName, lockLevel,
				held.name, held.level,
				lockName, held.name,
				string(stack[:n]),
			))
		}
	}
}

// recordLockAcquired records that the current goroutine has acquired a lock.
func recordLockAcquired(lockName string, lockLevel int) {
	if !lockOrderChecksEnabled() {
		return
	}

	gid := getGoroutineID()
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()

	globalTracker.locks[gid] = append(globalTracker.locks[gid], heldLock{
		name:  lockName,
		level: lockLevel,
	})
}

// recordLockReleased records that the current goroutine has released a lock.
func recordLockReleased(lockName string) {
	if !lockOrderChecksEnabled() {
		return
	}

	gid := getGoroutineID()
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()

	locks := globalTracker.locks[gid]
	// Remove the most recent occurrence of this lock
	for i := len(locks) - 1; i >= 0; i-- {
		if locks[i].name == lockName {
			globalTracker.locks[gid] = append(locks[:i], locks[i+1:]...)
			break
		}
	}

	// Clean up empty entries
	if len(globalTracker.locks[gid]) == 0 {
		delete(globalTracker.locks, gid)
	}
}
