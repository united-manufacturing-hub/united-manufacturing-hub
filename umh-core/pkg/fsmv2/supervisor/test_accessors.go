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
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// TestTick exposes tick() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestTick(ctx context.Context) error {
	return s.tick(ctx)
}

// TestRequestShutdown exposes requestShutdown() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestRequestShutdown(ctx context.Context, workerID string, reason string) error {
	return s.requestShutdown(ctx, workerID, reason)
}

// TestGetRestartCount returns collectorHealth.restartCount for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestGetRestartCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.collectorHealth.restartCount
}

// TestSetRestartCount sets collectorHealth.restartCount for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetRestartCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collectorHealth.restartCount = count
}

// TestTickAll exposes tickAll() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestTickAll(ctx context.Context) error {
	return s.tickAll(ctx)
}

// TestUpdateUserSpec exposes updateUserSpec() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestUpdateUserSpec(spec config.UserSpec) {
	s.updateUserSpec(spec)
}

// TestSetPendingRestart marks a worker as pending restart for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetPendingRestart(workerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingRestart[workerID] = true
}

// TestSetRestartRequestedAt sets the restart requested timestamp for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetRestartRequestedAt(workerID string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartRequestedAt[workerID] = t
}

// TestIsPendingRestart checks if worker is in pendingRestart map. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestIsPendingRestart(workerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingRestart[workerID]
}

// TestGetUserSpec returns the current userSpec for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestGetUserSpec() config.UserSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userSpec
}
