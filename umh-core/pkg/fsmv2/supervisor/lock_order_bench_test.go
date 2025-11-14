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
)

// BenchmarkLockAcquisitionWithoutChecks measures baseline lock performance
// without lock order checking (ENABLE_LOCK_ORDER_CHECKS not set).
func BenchmarkLockAcquisitionWithoutChecks(b *testing.B) {
	s := &Supervisor{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.lockMu()
		s.unlockMu()
	}
}

// BenchmarkLockAcquisitionConcurrentWithoutChecks measures concurrent lock
// performance without checking.
func BenchmarkLockAcquisitionConcurrentWithoutChecks(b *testing.B) {
	s := &Supervisor{}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.lockMu()
			s.unlockMu()
		}
	})
}

// BenchmarkWorkerContextLockWithoutChecks measures WorkerContext lock performance.
func BenchmarkWorkerContextLockWithoutChecks(b *testing.B) {
	wc := &WorkerContext{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wc.lockMu()
		wc.unlockMu()
	}
}

// BenchmarkReadLockWithoutChecks measures RLock performance.
func BenchmarkReadLockWithoutChecks(b *testing.B) {
	s := &Supervisor{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.rLockMu()
		s.rUnlockMu()
	}
}

// BenchmarkHierarchicalLockWithoutChecks measures performance of acquiring
// Supervisor.mu then WorkerContext.mu in correct order.
func BenchmarkHierarchicalLockWithoutChecks(b *testing.B) {
	s := &Supervisor{}
	wc := &WorkerContext{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.lockMu()
		wc.lockMu()
		wc.unlockMu()
		s.unlockMu()
	}
}
