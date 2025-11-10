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
	"fmt"
	"runtime"
	"sync"
	"time"

	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
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

func (a *BlockingAction) Execute(ctx context.Context) error {
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

func (a *PanicAction) Execute(ctx context.Context) error {
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

func (a *SlowAction) Execute(ctx context.Context) error {
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

type MockState struct {
	name       string
	nextState  fsmv2.State
	signal     fsmv2.Signal
	action     fsmv2.Action
	callCount  int
	mu         sync.Mutex
}

func NewMockState(name string) *MockState {
	return &MockState{
		name:   name,
		signal: fsmv2.SignalNone,
	}
}

func (m *MockState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	if m.nextState == nil {
		return m, m.signal, m.action
	}
	return m.nextState, m.signal, m.action
}

func (m *MockState) String() string {
	return m.name
}

func (m *MockState) Reason() string {
	return fmt.Sprintf("MockState: %s", m.name)
}

func (m *MockState) SetTransition(nextState fsmv2.State, signal fsmv2.Signal, action fsmv2.Action) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextState = nextState
	m.signal = signal
	m.action = action
}

func (m *MockState) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

type MockWorker struct {
	identity           fsmv2.Identity
	initialState       fsmv2.State
	observedState      fsmv2.ObservedState
	collectErr         error
	collectBlockFor    time.Duration
	collectCallCount   int
	collectPanic       bool
	mu                 sync.RWMutex
}

func NewMockWorker(id string) *MockWorker {
	return &MockWorker{
		identity: fsmv2.Identity{
			ID:         id,
			WorkerType: "mock",
		},
		initialState: NewMockState("Initial"),
	}
}

func (m *MockWorker) GetIdentity() fsmv2.Identity {
	return m.identity
}

func (m *MockWorker) GetInitialState() fsmv2.State {
	return m.initialState
}

func (m *MockWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	m.mu.Lock()
	m.collectCallCount++
	shouldPanic := m.collectPanic
	collectErr := m.collectErr
	blockFor := m.collectBlockFor
	observedState := m.observedState
	m.mu.Unlock()

	if shouldPanic {
		panic("intentional panic in CollectObservedState")
	}

	if blockFor > 0 {
		select {
		case <-time.After(blockFor):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if collectErr != nil {
		return nil, collectErr
	}

	if observedState != nil {
		return observedState, nil
	}

	return &MockObservedState{
		timestamp: time.Now(),
	}, nil
}

func (m *MockWorker) SetCollectError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collectErr = err
}

func (m *MockWorker) SetCollectBlockFor(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collectBlockFor = duration
}

func (m *MockWorker) SetCollectPanic(shouldPanic bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collectPanic = shouldPanic
}

func (m *MockWorker) GetCollectCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.collectCallCount
}

type MockObservedState struct {
	timestamp     time.Time
	desiredState  fsmv2.DesiredState
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

func (m *MockDesiredState) ShutdownRequested() bool {
	return m.shutdownRequested
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
