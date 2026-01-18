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

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

func GetGoroutineCount() int {
	return runtime.NumGoroutine()
}

func WaitForGoroutineCount(expected int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current := runtime.NumGoroutine()
		if current <= expected+5 {
			return nil
		}

		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for goroutine count to reach %d (within 5), current: %d", expected, runtime.NumGoroutine())
}

type BlockingAction struct {
	blockForever bool
	blockFor     time.Duration
	executed     bool
	mu           sync.Mutex
}

func NewBlockingAction(forever bool) *BlockingAction {
	return &BlockingAction{blockForever: forever}
}

func NewBlockingActionWithDuration(duration time.Duration) *BlockingAction {
	return &BlockingAction{blockFor: duration}
}

func (a *BlockingAction) Execute(ctx context.Context, deps any) error {
	a.mu.Lock()
	a.executed = true
	a.mu.Unlock()

	if a.blockForever {
		<-ctx.Done()

		return ctx.Err()
	}

	if a.blockFor > 0 {
		select {
		case <-time.After(a.blockFor):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (a *BlockingAction) String() string {
	return "BlockingAction"
}

func (a *BlockingAction) Name() string {
	return "BlockingAction"
}

func (a *BlockingAction) WasExecuted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.executed
}

type PanicAction struct {
	executed bool
	mu       sync.Mutex
}

func (a *PanicAction) Execute(ctx context.Context, deps any) error {
	a.mu.Lock()
	a.executed = true
	a.mu.Unlock()
	panic("intentional panic for testing")
}

func (a *PanicAction) String() string {
	return "PanicAction"
}

func (a *PanicAction) Name() string {
	return "PanicAction"
}

func (a *PanicAction) WasExecuted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.executed
}

type SlowAction struct {
	duration time.Duration
	executed bool
	mu       sync.Mutex
}

func NewSlowAction(duration time.Duration) *SlowAction {
	return &SlowAction{duration: duration}
}

func (a *SlowAction) Execute(ctx context.Context, deps any) error {
	a.mu.Lock()
	a.executed = true
	a.mu.Unlock()

	select {
	case <-time.After(a.duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *SlowAction) String() string {
	return fmt.Sprintf("SlowAction(%s)", a.duration)
}

func (a *SlowAction) Name() string {
	return "SlowAction"
}

func (a *SlowAction) WasExecuted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.executed
}

type MockObservedState struct {
	timestamp    time.Time
	desiredState fsmv2.DesiredState
}

func (m *MockObservedState) GetTimestamp() time.Time {
	return m.timestamp
}

func (m *MockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	if m.desiredState != nil {
		return m.desiredState
	}

	return &MockDesiredState{}
}

type MockDesiredState struct {
	shutdownRequested bool
}

func (m *MockDesiredState) IsShutdownRequested() bool {
	return m.shutdownRequested
}

func (m *MockDesiredState) GetState() string {
	return "running"
}

func (m *MockDesiredState) SetShutdownRequested(requested bool) {
	m.shutdownRequested = requested
}

func ExpectNoGoroutineLeaks(before int) {
	Eventually(func() int {
		runtime.GC()

		return runtime.NumGoroutine()
	}, "3s", "100ms").Should(BeNumerically("<=", before+5))
}

func GetWorkerStateName[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](sup *supervisor.Supervisor[TObserved, TDesired], workerID string) string {
	stateName, _, err := sup.GetWorkerState(workerID)
	if err != nil {
		return ""
	}

	return stateName
}

func GetChildSupervisor[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](parentSup *supervisor.Supervisor[TObserved, TDesired], childName string) *supervisor.Supervisor[TObserved, TDesired] {
	children := parentSup.GetChildren()
	for name, child := range children {
		if name == childName {
			if typedChild, ok := child.(*supervisor.Supervisor[TObserved, TDesired]); ok {
				return typedChild
			}
		}
	}

	return nil
}

func RunSupervisorWithTimeout[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](ctx context.Context, sup *supervisor.Supervisor[TObserved, TDesired], interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := sup.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

type MockConnectionPool struct {
	failureMode string
	failCount   int
	mu          sync.Mutex
}

func NewConnectionPool() *MockConnectionPool {
	return &MockConnectionPool{failureMode: "none"}
}

func (m *MockConnectionPool) WithFailures(count int) *MockConnectionPool {
	m.failureMode = "transient"
	m.failCount = count

	return m
}

func (m *MockConnectionPool) AlwaysFails() *MockConnectionPool {
	m.failureMode = "always"

	return m
}

func (m *MockConnectionPool) Build() *MockConnectionPool {
	return m
}

func (m *MockConnectionPool) Acquire() (child.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failureMode == "always" {
		return nil, errors.New("connection pool exhausted")
	}

	if m.failureMode == "transient" && m.failCount > 0 {
		m.failCount--

		return nil, errors.New("transient connection error")
	}

	return &MockConnection{}, nil
}

func (m *MockConnectionPool) Release(conn child.Connection) error {
	return nil
}

func (m *MockConnectionPool) HealthCheck(conn child.Connection) error {
	return nil
}

type MockConnection struct{}

func (m *MockConnection) IsHealthy() bool {
	return true
}

func NewTestApplicationSupervisor(yamlConfig string, logger *zap.SugaredLogger) (*supervisor.Supervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState], error) {
	ctx := context.Background()

	basicStore := memory.NewInMemoryStore()

	applicationWorkerType, err := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
	if err != nil {
		return nil, fmt.Errorf("failed to derive worker type: %w", err)
	}

	_ = basicStore.CreateCollection(ctx, applicationWorkerType+"_identity", nil)
	_ = basicStore.CreateCollection(ctx, applicationWorkerType+"_desired", nil)
	_ = basicStore.CreateCollection(ctx, applicationWorkerType+"_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, logger)

	sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:           "test-app-001",
		Name:         "Test Application Supervisor",
		Store:        triangularStore,
		Logger:       logger,
		TickInterval: 100 * time.Millisecond,
		YAMLConfig:   yamlConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create application supervisor: %w", err)
	}

	return sup, nil
}
