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
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// MockFSMManager is a mock implementation of FSMManager for testing
type MockFSMManager struct {
	ReconcileCalled bool
	ReconcileError  error
	ReconcileDelay  time.Duration
	mutex           sync.Mutex
}

// NewMockFSMManager creates a new MockFSMManager instance
func NewMockFSMManager() *MockFSMManager {
	return &MockFSMManager{}
}

// Reconcile implements the FSMManager interface
// The filesystemService parameter is not used in this mock implementation,
// but is included to match the interface signature.
func (m *MockFSMManager) Reconcile(ctx context.Context, snapshot SystemSnapshot, services serviceregistry.Provider) (error, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ReconcileCalled = true

	if m.ReconcileDelay > 0 {
		select {
		case <-time.After(m.ReconcileDelay):
			// Delay completed
		case <-ctx.Done():
			return ctx.Err(), false
		}
	}

	return m.ReconcileError, false
}

// WithReconcileError configures the mock to return the given error
func (m *MockFSMManager) WithReconcileError(err error) *MockFSMManager {
	m.ReconcileError = err
	return m
}

// WithReconcileDelay configures the mock to delay for the given duration
func (m *MockFSMManager) WithReconcileDelay(delay time.Duration) *MockFSMManager {
	m.ReconcileDelay = delay
	return m
}

// ResetCalls clears the called flags for testing multiple calls
func (m *MockFSMManager) ResetCalls() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ReconcileCalled = false
}

// GetManagerName returns the name of the manager
func (m *MockFSMManager) GetManagerName() string {
	return "MockFSMManager"
}

func (m *MockFSMManager) GetInstances() map[string]FSMInstance {
	return map[string]FSMInstance{}
}

func (m *MockFSMManager) GetInstance(name string) (FSMInstance, bool) {
	return nil, false
}
