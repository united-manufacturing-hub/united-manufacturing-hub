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

package lockmanager

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type LockManager struct {
	tracker *lockOrderTracker
	enabled bool
}

type Lock struct {
	mu      sync.RWMutex
	name    string
	level   int
	manager *LockManager
}

type lockOrderTracker struct {
	mu    sync.RWMutex
	locks map[uint64][]heldLock
}

type heldLock struct {
	name  string
	level int
}

func NewLockManager() *LockManager {
	return &LockManager{
		tracker: &lockOrderTracker{
			locks: make(map[uint64][]heldLock),
		},
		enabled: os.Getenv("ENABLE_LOCK_ORDER_CHECKS") == "1",
	}
}

func (lm *LockManager) NewLock(name string, level int) *Lock {
	return &Lock{
		name:    name,
		level:   level,
		manager: lm,
	}
}

func (l *Lock) Lock() {
	l.manager.checkLockOrder(l.name, l.level)
	l.mu.Lock()
	l.manager.recordLockAcquired(l.name, l.level)
}

func (l *Lock) Unlock() {
	l.manager.recordLockReleased(l.name)
	l.mu.Unlock()
}

func (l *Lock) RLock() {
	l.manager.checkLockOrder(l.name, l.level)
	l.mu.RLock()
	l.manager.recordLockAcquired(l.name, l.level)
}

func (l *Lock) RUnlock() {
	l.manager.recordLockReleased(l.name)
	l.mu.RUnlock()
}

func (lm *LockManager) IsHeld(name string) bool {
	if !lm.enabled {
		return false
	}

	gid := getGoroutineID()

	lm.tracker.mu.RLock()
	defer lm.tracker.mu.RUnlock()

	heldLocks := lm.tracker.locks[gid]
	for _, held := range heldLocks {
		if held.name == name {
			return true
		}
	}

	return false
}

func (lm *LockManager) checkLockOrder(lockName string, lockLevel int) {
	if !lm.enabled {
		return
	}

	gid := getGoroutineID()

	lm.tracker.mu.RLock()
	heldLocks := lm.tracker.locks[gid]
	lm.tracker.mu.RUnlock()

	for _, held := range heldLocks {
		if held.level > lockLevel {
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

func (lm *LockManager) recordLockAcquired(lockName string, lockLevel int) {
	if !lm.enabled {
		return
	}

	gid := getGoroutineID()

	lm.tracker.mu.Lock()
	defer lm.tracker.mu.Unlock()

	lm.tracker.locks[gid] = append(lm.tracker.locks[gid], heldLock{
		name:  lockName,
		level: lockLevel,
	})
}

func (lm *LockManager) recordLockReleased(lockName string) {
	if !lm.enabled {
		return
	}

	gid := getGoroutineID()

	lm.tracker.mu.Lock()
	defer lm.tracker.mu.Unlock()

	locks, exists := lm.tracker.locks[gid]
	if !exists {
		return
	}

	for i := len(locks) - 1; i >= 0; i-- {
		if locks[i].name == lockName {
			lm.tracker.locks[gid] = append(locks[:i], locks[i+1:]...)
			break
		}
	}

	if len(lm.tracker.locks[gid]) == 0 {
		delete(lm.tracker.locks, gid)
	}
}

func getGoroutineID() uint64 {
	var buf [64]byte

	n := runtime.Stack(buf[:], false)
	idField := strings.Fields(string(buf[:n]))[1]
	id, _ := strconv.ParseUint(idField, 10, 64)

	return id
}
