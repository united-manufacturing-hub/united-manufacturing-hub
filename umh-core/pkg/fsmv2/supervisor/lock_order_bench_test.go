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
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/lockmanager"
)

// BenchmarkLockAcquisitionWithoutChecks measures baseline lock performance
// without lock order checking (ENABLE_LOCK_ORDER_CHECKS not set).
func BenchmarkLockAcquisitionWithoutChecks(b *testing.B) {
	lm := lockmanager.NewLockManager()
	lock := lm.NewLock("BenchLock", 1)

	b.ResetTimer()
	for range b.N {
		lock.Lock()
		lock.Unlock()
	}
}

// BenchmarkLockAcquisitionConcurrentWithoutChecks measures concurrent lock
// performance without checking.
func BenchmarkLockAcquisitionConcurrentWithoutChecks(b *testing.B) {
	lm := lockmanager.NewLockManager()
	lock := lm.NewLock("BenchLock", 1)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lock.Lock()
			lock.Unlock()
		}
	})
}

// BenchmarkWorkerContextLockWithoutChecks measures WorkerContext lock performance.
func BenchmarkWorkerContextLockWithoutChecks(b *testing.B) {
	lm := lockmanager.NewLockManager()
	lock := lm.NewLock("WorkerLock", 1)

	b.ResetTimer()
	for range b.N {
		lock.Lock()
		lock.Unlock()
	}
}

// BenchmarkReadLockWithoutChecks measures RLock performance.
func BenchmarkReadLockWithoutChecks(b *testing.B) {
	lm := lockmanager.NewLockManager()
	lock := lm.NewLock("ReadLock", 1)

	b.ResetTimer()
	for range b.N {
		lock.RLock()
		lock.RUnlock()
	}
}

// BenchmarkHierarchicalLockWithoutChecks measures performance of acquiring
// Supervisor.mu then WorkerContext.mu in correct order.
func BenchmarkHierarchicalLockWithoutChecks(b *testing.B) {
	lm := lockmanager.NewLockManager()
	supervisorLock := lm.NewLock("SupervisorLock", 1)
	workerLock := lm.NewLock("WorkerLock", 2)

	b.ResetTimer()
	for range b.N {
		supervisorLock.Lock()
		workerLock.Lock()
		workerLock.Unlock()
		supervisorLock.Unlock()
	}
}
