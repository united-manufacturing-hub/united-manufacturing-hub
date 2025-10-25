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

package communicator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
	csesync "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/sync"
)

func TestMockOrchestrator_ImplementsInterface(t *testing.T) {
	mock := NewMockOrchestrator()

	var _ csesync.OrchestratorInterface = mock
}

func TestMockOrchestrator_StartSync(t *testing.T) {
	ctx := context.Background()
	mock := NewMockOrchestrator()

	subscriptions := []csesync.Subscription{
		{Model: "*", Fields: []string{}},
	}

	err := mock.StartSync(ctx, protocol.TierEdge, subscriptions)
	require.NoError(t, err, "StartSync should succeed by default")

	assert.Equal(t, 1, mock.StartSyncCallCount(), "StartSync should be called once")
}

func TestMockOrchestrator_StartSyncError(t *testing.T) {
	ctx := context.Background()
	mock := NewMockOrchestrator()

	expectedErr := assert.AnError
	mock.SetStartSyncError(expectedErr)

	err := mock.StartSync(ctx, protocol.TierEdge, nil)
	require.ErrorIs(t, err, expectedErr, "StartSync should return configured error")
}

func TestMockOrchestrator_ProcessIncoming(t *testing.T) {
	ctx := context.Background()
	mock := NewMockOrchestrator()

	msg := protocol.RawMessage{
		From:    "frontend",
		Payload: []byte("test"),
	}

	err := mock.ProcessIncoming(ctx, msg)
	require.NoError(t, err, "ProcessIncoming should succeed by default")

	assert.Equal(t, 1, mock.ProcessIncomingCallCount(), "ProcessIncoming should be called once")
}

func TestMockOrchestrator_ProcessIncomingError(t *testing.T) {
	ctx := context.Background()
	mock := NewMockOrchestrator()

	expectedErr := assert.AnError
	mock.SetProcessIncomingError(expectedErr)

	err := mock.ProcessIncoming(ctx, protocol.RawMessage{})
	require.ErrorIs(t, err, expectedErr, "ProcessIncoming should return configured error")
}

func TestMockOrchestrator_Tick(t *testing.T) {
	ctx := context.Background()
	mock := NewMockOrchestrator()

	err := mock.Tick(ctx)
	require.NoError(t, err, "Tick should succeed by default")

	assert.Equal(t, 1, mock.TickCallCount(), "Tick should be called once")
}

func TestMockOrchestrator_TickError(t *testing.T) {
	ctx := context.Background()
	mock := NewMockOrchestrator()

	expectedErr := assert.AnError
	mock.SetTickError(expectedErr)

	err := mock.Tick(ctx)
	require.ErrorIs(t, err, expectedErr, "Tick should return configured error")
}

func TestMockOrchestrator_GetStatus(t *testing.T) {
	mock := NewMockOrchestrator()

	status := mock.GetStatus()

	assert.True(t, status.Healthy, "Mock should be healthy by default")
	assert.Equal(t, 0, status.PendingChangeCount, "Should have no pending changes by default")
}

func TestMockOrchestrator_SetStatus(t *testing.T) {
	mock := NewMockOrchestrator()

	expectedStatus := csesync.SyncStatus{
		Healthy:            false,
		LastSyncTime:       time.Now(),
		PendingChangeCount: 5,
		EdgeSyncID:         100,
		FrontendSyncID:     200,
	}

	mock.SetStatus(expectedStatus)

	status := mock.GetStatus()
	assert.Equal(t, expectedStatus.Healthy, status.Healthy)
	assert.Equal(t, expectedStatus.PendingChangeCount, status.PendingChangeCount)
	assert.Equal(t, expectedStatus.EdgeSyncID, status.EdgeSyncID)
	assert.Equal(t, expectedStatus.FrontendSyncID, status.FrontendSyncID)
}

func TestMockOrchestrator_ConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	mock := NewMockOrchestrator()

	var wg sync.WaitGroup
	callCount := 10

	for i := 0; i < callCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mock.StartSync(ctx, protocol.TierEdge, nil)
			_ = mock.ProcessIncoming(ctx, protocol.RawMessage{})
			_ = mock.Tick(ctx)
			_ = mock.GetStatus()
		}()
	}

	wg.Wait()

	assert.Equal(t, callCount, mock.StartSyncCallCount())
	assert.Equal(t, callCount, mock.ProcessIncomingCallCount())
	assert.Equal(t, callCount, mock.TickCallCount())
}

type MockOrchestrator struct {
	mu sync.Mutex

	startSyncCalls       int
	processIncomingCalls int
	tickCalls            int

	startSyncErr       error
	processIncomingErr error
	tickErr            error
	injectedErr        error

	status csesync.SyncStatus
}

func NewMockOrchestrator() *MockOrchestrator {
	return &MockOrchestrator{
		status: csesync.SyncStatus{
			Healthy:            true,
			PendingChangeCount: 0,
		},
	}
}

func (m *MockOrchestrator) StartSync(ctx context.Context, tier protocol.Tier, subscriptions []csesync.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.startSyncCalls++

	return m.startSyncErr
}

func (m *MockOrchestrator) ProcessIncoming(ctx context.Context, msg protocol.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.processIncomingCalls++

	return m.processIncomingErr
}

func (m *MockOrchestrator) Tick(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tickCalls++

	if m.injectedErr != nil {
		return m.injectedErr
	}

	return m.tickErr
}

func (m *MockOrchestrator) GetStatus() csesync.SyncStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.status
}

func (m *MockOrchestrator) SetStartSyncError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.startSyncErr = err
}

func (m *MockOrchestrator) SetProcessIncomingError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.processIncomingErr = err
}

func (m *MockOrchestrator) SetTickError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tickErr = err
}

func (m *MockOrchestrator) SetStatus(status csesync.SyncStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.status = status
}

func (m *MockOrchestrator) StartSyncCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.startSyncCalls
}

func (m *MockOrchestrator) ProcessIncomingCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.processIncomingCalls
}

func (m *MockOrchestrator) TickCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.tickCalls
}

func (m *MockOrchestrator) InjectError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.injectedErr = err
}

func (m *MockOrchestrator) ClearErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.injectedErr = nil
}
