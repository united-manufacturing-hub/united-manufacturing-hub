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
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

func registerTestWorkerFactories() {
	workerTypes := []string{
		"child",
		"parent",
		"mqtt_client",
		"mqtt_broker",
		"opcua_client",
		"opcua_server",
		"s7comm_client",
		"modbus_client",
		"http_client",
		"child1",
		"child2",
		"grandchild",
		"valid_child",
		"another_child",
		"working-child",
		"failing-child",
		"working",
		"failing",
		"container",
		"s6_service",
		"type_a",
		"type_b",
	}

	for _, workerType := range workerTypes {
		wt := workerType
		// Register worker factory
		_ = factory.RegisterFactoryByType(wt, func(identity fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
			return &TestWorkerWithType{
				WorkerType: wt,
			}
		})

		// Register supervisor factory for hierarchical composition
		_ = factory.RegisterSupervisorFactoryByType(wt, func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(Config)

			return NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)
		})
	}
}

// TestSupervisorUsesTriangularStore verifies that Supervisor uses TriangularStore instead of basic Store.
// This test ensures the supervisor properly integrates with the triangular persistence model.
func TestSupervisorUsesTriangularStore(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	// Create in-memory store for testing
	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	// Create collections in database
	var err error

	err = basicStore.CreateCollection(ctx, "test_identity", nil)
	if err != nil {
		t.Fatalf("Failed to create identity collection: %v", err)
	}

	err = basicStore.CreateCollection(ctx, "test_desired", nil)
	if err != nil {
		t.Fatalf("Failed to create desired collection: %v", err)
	}

	err = basicStore.CreateCollection(ctx, "test_observed", nil)
	if err != nil {
		t.Fatalf("Failed to create observed collection: %v", err)
	}

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	if supervisor == nil {
		t.Fatal("Expected supervisor to be created")
	}
}

// TestActionBehaviorDuringCircuitBreaker verifies that in-progress actions continue execution
// when the circuit breaker opens due to infrastructure inconsistency.
//
// Acceptance Criteria (Task 3.2):
// 1. Circuit opens mid-action (child inconsistency detected)
// 2. Action continues execution (not cancelled)
// 3. Metrics recorded:
//   - supervisor_actions_during_circuit_total incremented
//   - supervisor_action_post_circuit_duration_seconds observes duration
//
// Expected Behavior:
// - Action started at T+0s, circuit opens at T+10s
// - Action runs to completion (~30s total)
// - Metrics show action continued for ~20s after circuit opened
//
// This is a TEST SPEC - not yet implemented.
// Implementation will follow TDD: RED → GREEN → REFACTOR.
func TestActionBehaviorDuringCircuitBreaker(t *testing.T) {
	t.Skip("Task 3.2: Test spec for action-child lifecycle behavior")

	// RED: This test will fail because:
	// 1. Supervisor.circuitOpen field doesn't exist yet
	// 2. ActionExecutor doesn't track circuit open timestamp
	// 3. Metrics not implemented (supervisor_actions_during_circuit_total, supervisor_action_post_circuit_duration_seconds)
	// 4. InfrastructureHealthChecker.CheckChildConsistency() not implemented

	// Test outline:
	// 1. Setup supervisor with infrastructure health checker
	// 2. Start long-running action (30s simulated delay)
	// 3. Trigger circuit open at T+10s (simulate child inconsistency)
	// 4. Wait for action to complete
	// 5. Verify:
	//    - Action completed successfully (not cancelled)
	//    - supervisor_actions_during_circuit_total == 1
	//    - supervisor_action_post_circuit_duration_seconds observed ~20s
}

// TestCollectorContinuesDuringCircuitOpen verifies that collector goroutines continue writing
// observations to TriangularStore even when the circuit breaker is open.
//
// Acceptance Criteria (Task 3.4):
// 1. Circuit opens (infrastructure failure detected)
// 2. Collector continues calling worker.CollectObservedState() every second
// 3. TriangularStore receives continuous updates (verified by timestamps)
// 4. When circuit closes, fresh observations available immediately (no staleness)
//
// Expected Behavior:
// - Circuit opens at T+0s
// - Collector writes at T+1s, T+2s, T+3s...T+10s (~10 updates during circuit open)
// - TriangularStore timestamps show continuous progression
// - Circuit closes at T+10s
// - Next LoadSnapshot() returns T+10s data (fresh, not T+0s)
//
// This is a TEST SPEC - not yet implemented.
// Implementation will follow TDD: RED → GREEN → REFACTOR.
func TestCollectorContinuesDuringCircuitOpen(t *testing.T) {
	t.Skip("Task 3.4: Test spec for collector-circuit independence")

	// RED: This test will fail because:
	// 1. Supervisor.circuitOpen field doesn't exist yet
	// 2. Collector goroutines not started in supervisor.Start()
	// 3. InfrastructureHealthChecker.CheckChildConsistency() not implemented
	// 4. No mechanism to track TriangularStore write timestamps

	// Test outline:
	// 1. Setup supervisor with mock worker that tracks CollectObservedState() calls
	// 2. Start supervisor (spawns collector goroutine)
	// 3. Trigger circuit open at T+0s (simulate infrastructure failure)
	// 4. Wait 10 seconds
	// 5. Verify:
	//    - CollectObservedState() called ~10 times (T+1s, T+2s, T+3s...T+10s)
	//    - TriangularStore write timestamps show progression: T+1s, T+2s, T+3s...T+10s
	//    - Circuit breaker didn't pause collector
	// 6. Close circuit
	// 7. Trigger supervisor.Tick()
	// 8. Verify:
	//    - LoadSnapshot() returns T+10s data (or newer)
	//    - No staleness penalty (data is fresh)
}

// TestVariablePropagationThrough3LevelHierarchy verifies that user variables flow
// from grandparent → parent → child through the supervisor hierarchy.
//
// Acceptance Criteria (Task 3.6):
// 1. Grandparent sets Variables.User["IP"] = "192.168.1.100"
// 2. Grandparent creates child (parent)
// 3. Parent creates child (child)
// 4. Child receives IP variable from grandparent
//
// Expected Behavior:
// - Grandparent has Variables.User["IP"]
// - After grandparent.Tick(), parent is created
// - After parent.Tick(), child is created
// - Child's UserSpec.Variables.User["IP"] equals "192.168.1.100"
// - Variables propagate through DeriveDesiredState → ChildSpec → supervisor creation
//
// This is a TEST SPEC - not yet implemented.
// Implementation will follow TDD: RED → GREEN → REFACTOR.
func TestVariablePropagationThrough3LevelHierarchy(t *testing.T) {
	t.Skip("Task 3.6: Test spec for variable flow through supervisor hierarchy")

	// RED: This test will fail because:
	// 1. Need to create mock workers that declare children in DeriveDesiredState()
	// 2. Supervisor variable injection in Tick() may not be complete
	// 3. ChildSpec.UserSpec.Variables may not preserve parent variables
	// 4. No mechanism to access child supervisors for verification

	// Test outline:
	// 1. Create grandparent supervisor with Variables.User["IP"] = "192.168.1.100"
	// 2. Configure grandparent worker to return ChildSpec for parent
	// 3. Call grandparent.Tick(ctx)
	// 4. Verify parent supervisor created
	// 5. Configure parent worker to return ChildSpec for child
	// 6. Call parent.Tick(ctx)
	// 7. Verify child supervisor created
	// 8. Access child.UserSpec.Variables.User
	// 9. Verify child received grandparent's IP variable: "192.168.1.100"
	// 10. Verify variable flow: grandparent → parent → child (3 levels)
}

// TestLocationHierarchyComputation verifies that parent and child locations merge
// correctly to produce complete ISA-95 location paths.
//
// Acceptance Criteria (Task 3.7):
// 1. Parent has location: Enterprise=ACME, Site=Factory-1
// 2. Child has location: Line=Line-A, Cell=Cell-5
// 3. After tick, child receives merged location_path
// 4. Variables.Internal["location_path"] = "ACME.Factory-1.Line-A.Cell-5"
//
// Expected Behavior:
// - Parent location defines enterprise and site levels
// - Child location defines line and cell levels
// - Supervisor merges locations before creating child
// - Child receives full path in Internal variables
// - ISA-95 gaps filled (Area level empty but included)
//
// This is a TEST SPEC - not yet implemented.
// Implementation will follow TDD: RED → GREEN → REFACTOR.
func TestLocationHierarchyComputation(t *testing.T) {
	t.Skip("Task 3.7: Test spec for location hierarchy computation")

	// RED: This test will fail because:
	// 1. Supervisor location merging not implemented in child creation
	// 2. Internal["location_path"] may not be populated
	// 3. location.MergeLocations() integration with supervisor may be incomplete
	// 4. ISA-95 gap filling might not happen automatically

	// Test outline:
	// 1. Create parent supervisor with location: [{Enterprise: "ACME"}, {Site: "Factory-1"}]
	// 2. Configure parent worker to declare child with location: [{Line: "Line-A"}, {Cell: "Cell-5"}]
	// 3. Call parent.Tick(ctx)
	// 4. Verify child supervisor created
	// 5. Access child.Variables.Internal["location_path"]
	// 6. Verify: "ACME.Factory-1.Line-A.Cell-5" (or with Area gap: "ACME.Factory-1..Line-A.Cell-5")
	// 7. Verify ISA-95 hierarchy: enterprise → site → area (empty) → line → cell
}

// TestProtocolConverterEndToEndTick verifies that a complete ProtocolConverter
// workflow executes all FSMv2 phases correctly in integration.
//
// Acceptance Criteria (Task 3.8):
// 1. ProtocolConverter declares children: Connection + SourceFlow + SinkFlow
// 2. State mapping applies: idle → stopped for connection
// 3. Templates render with variables (IP, PORT from connection config)
// 4. Async actions execute when state transitions
//
// Expected Behavior:
// Tick 1: Create children (Connection + SourceFlow + SinkFlow)
// Tick 2: Apply state mapping (ProtocolConverter idle → Connection stopped)
// Tick 3: Render templates with variables ({{ .IP }} → "192.168.1.100")
// Tick 4: Execute async action (ProtocolConverter active → start flows)
// Eventually: ProtocolConverter reaches running state
//
// This is a TEST SPEC - not yet implemented.
// Implementation will follow TDD: RED → GREEN → REFACTOR.
func TestProtocolConverterEndToEndTick(t *testing.T) {
	t.Skip("Task 3.8: Test spec for ProtocolConverter end-to-end integration")

	// RED: This test will fail because:
	// 1. ProtocolConverter mock worker not created yet
	// 2. State mapping application in Tick() may be incomplete
	// 3. Template rendering integration with ChildSpec not verified
	// 4. Async action execution for state transitions not tested end-to-end

	// Test outline:
	// 1. Create ProtocolConverter supervisor (grandparent)
	// 2. Set Variables.User: IP="192.168.1.100", PORT=502
	// 3. Configure ProtocolConverter to declare 3 children:
	//    - Connection (monitors network)
	//    - SourceFlow (reads from protocol)
	//    - SinkFlow (writes to Kafka)
	// 4. Tick 1: Call supervisor.Tick(ctx)
	// 5. Verify: 3 children created
	// 6. Tick 2: Set DesiredState{State: "idle"}
	// 7. Call supervisor.Tick(ctx)
	// 8. Verify: State mapping applied (Connection.observedState.State == "stopped")
	// 9. Tick 3: Access SourceFlow child
	// 10. Verify: Template rendered ("{{ .IP }}" → "192.168.1.100" in config)
	// 11. Tick 4: Set DesiredState{State: "active"}
	// 12. Call supervisor.Tick(ctx)
	// 13. Verify: Async action enqueued (start flows)
	// 14. Wait for action completion
	// 15. Verify: ProtocolConverter.observedState.State == "running"
	// 16. Verify: All phases integrated: Hierarchy + Templates + Variables + ChildStartStates + AsyncActions
}

// TestSupervisorSavesIdentityToTriangularStore verifies that Supervisor uses TriangularStore.SaveIdentity
// when adding a worker.
func TestSupervisorSavesIdentityToTriangularStore(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	// Create in-memory store for testing
	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	// Create collections in database
	var err error

	err = basicStore.CreateCollection(ctx, "test_identity", nil)
	if err != nil {
		t.Fatalf("Failed to create identity collection: %v", err)
	}

	err = basicStore.CreateCollection(ctx, "test_desired", nil)
	if err != nil {
		t.Fatalf("Failed to create desired collection: %v", err)
	}

	err = basicStore.CreateCollection(ctx, "test_observed", nil)
	if err != nil {
		t.Fatalf("Failed to create observed collection: %v", err)
	}

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "worker-1",
		Name:       "Test Worker",
		WorkerType: "test",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "initial"},
		observed: persistence.Document{
			"id":     "worker-1",
			"status": "running",
		},
	}

	err = supervisor.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add worker: %v", err)
	}

	loadedIdentity, err := triangularStore.LoadIdentity(ctx, "test", "worker-1")
	if err != nil {
		t.Fatalf("Failed to load identity: %v", err)
	}

	if loadedIdentity["id"] != "worker-1" {
		t.Errorf("Expected identity id 'worker-1', got %v", loadedIdentity["id"])
	}

	if loadedIdentity["name"] != "Test Worker" {
		t.Errorf("Expected identity name 'Test Worker', got %v", loadedIdentity["name"])
	}
}

// TestSupervisorLoadsSnapshotFromTriangularStore verifies that Supervisor uses TriangularStore.LoadSnapshot
// during tick operations.
func TestSupervisorLoadsSnapshotFromTriangularStore(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	// Create in-memory store for testing
	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	// Create collections in database
	var err error

	err = basicStore.CreateCollection(ctx, "test_identity", nil)
	if err != nil {
		t.Fatalf("Failed to create identity collection: %v", err)
	}

	err = basicStore.CreateCollection(ctx, "test_desired", nil)
	if err != nil {
		t.Fatalf("Failed to create desired collection: %v", err)
	}

	err = basicStore.CreateCollection(ctx, "test_observed", nil)
	if err != nil {
		t.Fatalf("Failed to create observed collection: %v", err)
	}

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "worker-1",
		Name:       "Test Worker",
		WorkerType: "test",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "initial"},
		observed: persistence.Document{
			"id":     "worker-1",
			"status": "running",
		},
	}

	err = supervisor.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add worker: %v", err)
	}

	_, err = triangularStore.SaveDesired(ctx, "test", "worker-1", persistence.Document{
		"id":     "worker-1",
		"config": "production",
	})
	if err != nil {
		t.Fatalf("Failed to save desired: %v", err)
	}

	err = supervisor.tick(ctx)
	if err != nil {
		t.Fatalf("Failed to tick: %v", err)
	}
}

type mockWorker struct {
	identity     fsmv2.Identity
	initialState fsmv2.State[any, any]
	observed     persistence.Document
}

func (m *mockWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc:       m.observed,
		timestamp: time.Now(),
	}, nil
}

func (m *mockWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{State: "running"}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State[any, any] {
	return m.initialState
}

type mockObservedState struct {
	doc       persistence.Document
	timestamp time.Time
}

func (m *mockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &mockDesiredState{}
}

func (m *mockObservedState) GetTimestamp() time.Time {
	return m.timestamp
}

// MarshalJSON ensures mockObservedState serializes to its document content.
func (m *mockObservedState) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.doc)
}

type mockDesiredState struct {
	State string
}

func (m *mockDesiredState) IsShutdownRequested() bool {
	return false
}

func (m *mockDesiredState) SetShutdownRequested(_ bool) {
}

func (m *mockDesiredState) GetState() string {
	if m.State == "" {
		return "running"
	}

	return m.State
}

type mockState struct {
	name   string
	reason string
}

func (m *mockState) String() string {
	return m.name
}

func (m *mockState) Reason() string {
	return m.reason
}

func (m *mockState) Next(_ any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	return m, fsmv2.SignalNone, nil
}

type mockWorkerWithChildren struct {
	identity      fsmv2.Identity
	initialState  fsmv2.State[any, any]
	observed      persistence.Document
	childrenSpecs []config.ChildSpec
}

func (m *mockWorkerWithChildren) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc:       m.observed,
		timestamp: time.Now(),
	}, nil
}

func (m *mockWorkerWithChildren) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{
		State:         "running",
		ChildrenSpecs: m.childrenSpecs,
	}, nil
}

func (m *mockWorkerWithChildren) GetInitialState() fsmv2.State[any, any] {
	return m.initialState
}

func TestChildStartStates_ParentInList(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	registerTestWorkerFactories()

	defer factory.ResetRegistry()

	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)
	// Collections are named after WorkerType, not child Name
	_ = basicStore.CreateCollection(ctx, "child_identity", nil)
	_ = basicStore.CreateCollection(ctx, "child_desired", nil)
	_ = basicStore.CreateCollection(ctx, "child_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	// ChildStartStates: child runs when parent is in "active" state
	childSpecs := []config.ChildSpec{
		{
			Name:             "child-1",
			WorkerType:       "child",
			UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
			ChildStartStates: []string{"active"},
		},
	}

	// Parent is in "active" state (which IS in ChildStartStates)
	worker := &mockWorkerWithChildren{
		identity:      identity,
		initialState:  &mockState{name: "active"},
		observed:      persistence.Document{"id": "parent-1", "status": "active"},
		childrenSpecs: childSpecs,
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	err = parent.tick(ctx)
	if err != nil {
		t.Fatalf("tick failed: %v", err)
	}

	children := parent.GetChildren()
	if len(children) != 1 {
		t.Fatalf("Expected 1 child, got %d", len(children))
	}

	child, exists := children["child-1"]
	if !exists {
		t.Fatal("Expected child-1 to exist")
	}

	typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
	if !ok {
		t.Fatal("Child supervisor should be correct type")
	}

	mappedState := typedChild.GetMappedParentState()
	if mappedState != "running" {
		t.Errorf("Expected child mappedParentState 'running' (parent in ChildStartStates), got '%s'", mappedState)
	}
}

func TestChildStartStates_EmptyList(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	registerTestWorkerFactories()

	defer factory.ResetRegistry()

	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)
	// Collections are named after WorkerType, not child Name
	_ = basicStore.CreateCollection(ctx, "child_identity", nil)
	_ = basicStore.CreateCollection(ctx, "child_desired", nil)
	_ = basicStore.CreateCollection(ctx, "child_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	// Empty ChildStartStates = child always runs
	childSpecs := []config.ChildSpec{
		{
			Name:             "child-1",
			WorkerType:       "child",
			UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
			ChildStartStates: nil, // Empty = always run
		},
	}

	worker := &mockWorkerWithChildren{
		identity:      identity,
		initialState:  &mockState{name: "running"},
		observed:      persistence.Document{"id": "parent-1", "status": "running"},
		childrenSpecs: childSpecs,
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	err = parent.tick(ctx)
	if err != nil {
		t.Fatalf("tick failed: %v", err)
	}

	children := parent.GetChildren()

	child, exists := children["child-1"]
	if !exists {
		t.Fatal("Expected child-1 to exist")
	}

	typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
	if !ok {
		t.Fatal("Child supervisor should be correct type")
	}

	mappedState := typedChild.GetMappedParentState()
	if mappedState != "running" {
		t.Errorf("Expected child mappedParentState 'running' (empty ChildStartStates = always run), got '%s'", mappedState)
	}
}

func TestChildStartStates_ParentNotInList(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	registerTestWorkerFactories()

	defer factory.ResetRegistry()

	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)
	// Collections are named after WorkerType, not child Name
	_ = basicStore.CreateCollection(ctx, "child_identity", nil)
	_ = basicStore.CreateCollection(ctx, "child_desired", nil)
	_ = basicStore.CreateCollection(ctx, "child_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	// ChildStartStates: child runs when parent is in "active" state
	// Parent is in "unknown" state (NOT in list) → child should be stopped
	childSpecs := []config.ChildSpec{
		{
			Name:             "child-1",
			WorkerType:       "child",
			UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
			ChildStartStates: []string{"active"},
		},
	}

	worker := &mockWorkerWithChildren{
		identity:      identity,
		initialState:  &mockState{name: "unknown"},
		observed:      persistence.Document{"id": "parent-1", "status": "unknown"},
		childrenSpecs: childSpecs,
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	err = parent.tick(ctx)
	if err != nil {
		t.Fatalf("tick failed: %v", err)
	}

	children := parent.GetChildren()

	child, exists := children["child-1"]
	if !exists {
		t.Fatal("Expected child-1 to exist")
	}

	typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
	if !ok {
		t.Fatal("Child supervisor should be correct type")
	}

	mappedState := typedChild.GetMappedParentState()
	if mappedState != "stopped" {
		t.Errorf("Expected child mappedParentState 'stopped' (parent NOT in ChildStartStates), got '%s'", mappedState)
	}
}

func TestChildStartStates_MultipleChildren(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	registerTestWorkerFactories()

	defer factory.ResetRegistry()

	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)
	// Collections are named after WorkerType, not child Name
	_ = basicStore.CreateCollection(ctx, "child_identity", nil)
	_ = basicStore.CreateCollection(ctx, "child_desired", nil)
	_ = basicStore.CreateCollection(ctx, "child_observed", nil)
	_ = basicStore.CreateCollection(ctx, "child-2_identity", nil)
	_ = basicStore.CreateCollection(ctx, "child-2_desired", nil)
	_ = basicStore.CreateCollection(ctx, "child-2_observed", nil)
	_ = basicStore.CreateCollection(ctx, "child-3_identity", nil)
	_ = basicStore.CreateCollection(ctx, "child-3_desired", nil)
	_ = basicStore.CreateCollection(ctx, "child-3_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	// Test different ChildStartStates configurations
	childSpecs := []config.ChildSpec{
		{
			Name:             "child-1",
			WorkerType:       "child",
			UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
			ChildStartStates: []string{"active"}, // Runs when parent is "active"
		},
		{
			Name:             "child-2",
			WorkerType:       "child",
			UserSpec:         config.UserSpec{Config: "address: 192.168.1.100:502"},
			ChildStartStates: []string{"idle"}, // Runs when parent is "idle" (NOT "active")
		},
		{
			Name:             "child-3",
			WorkerType:       "child",
			UserSpec:         config.UserSpec{Config: "endpoint: opc.tcp://localhost:4840"},
			ChildStartStates: nil, // Empty = always runs
		},
	}

	// Parent is in "active" state
	worker := &mockWorkerWithChildren{
		identity:      identity,
		initialState:  &mockState{name: "active"},
		observed:      persistence.Document{"id": "parent-1", "status": "active"},
		childrenSpecs: childSpecs,
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	err = parent.tick(ctx)
	if err != nil {
		t.Fatalf("tick failed: %v", err)
	}

	children := parent.GetChildren()
	if len(children) != 3 {
		t.Fatalf("Expected 3 children, got %d", len(children))
	}

	// child-1: ChildStartStates: ["active"], parent is "active" → runs
	child1, exists := children["child-1"]
	if !exists {
		t.Fatal("Expected child-1 to exist")
	}

	typedChild1, ok := child1.(*Supervisor[*TestObservedState, *TestDesiredState])
	if !ok {
		t.Fatal("Child-1 supervisor should be correct type")
	}

	if typedChild1.GetMappedParentState() != "running" {
		t.Errorf("Expected child-1 mappedParentState 'running' (parent in ChildStartStates), got '%s'", typedChild1.GetMappedParentState())
	}

	// child-2: ChildStartStates: ["idle"], parent is "active" → stopped
	child2, exists := children["child-2"]
	if !exists {
		t.Fatal("Expected child-2 to exist")
	}

	typedChild2, ok := child2.(*Supervisor[*TestObservedState, *TestDesiredState])
	if !ok {
		t.Fatal("Child-2 supervisor should be correct type")
	}

	if typedChild2.GetMappedParentState() != "stopped" {
		t.Errorf("Expected child-2 mappedParentState 'stopped' (parent NOT in ChildStartStates), got '%s'", typedChild2.GetMappedParentState())
	}

	// child-3: ChildStartStates: nil (empty) → always runs
	child3, exists := children["child-3"]
	if !exists {
		t.Fatal("Expected child-3 to exist")
	}

	typedChild3, ok := child3.(*Supervisor[*TestObservedState, *TestDesiredState])
	if !ok {
		t.Fatal("Child-3 supervisor should be correct type")
	}

	if typedChild3.GetMappedParentState() != "running" {
		t.Errorf("Expected child-3 mappedParentState 'running' (empty ChildStartStates = always run), got '%s'", typedChild3.GetMappedParentState())
	}
}

func TestChildStartStates_EmptySlice(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	registerTestWorkerFactories()

	defer factory.ResetRegistry()

	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)
	// Collections are named after WorkerType, not child Name
	_ = basicStore.CreateCollection(ctx, "mqtt_client_identity", nil)
	_ = basicStore.CreateCollection(ctx, "mqtt_client_desired", nil)
	_ = basicStore.CreateCollection(ctx, "mqtt_client_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	// Empty slice = always runs
	childSpecs := []config.ChildSpec{
		{
			Name:             "child-1",
			WorkerType:       "mqtt_client",
			UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
			ChildStartStates: []string{},
		},
	}

	worker := &mockWorkerWithChildren{
		identity:      identity,
		initialState:  &mockState{name: "running"},
		observed:      persistence.Document{"id": "parent-1", "status": "running"},
		childrenSpecs: childSpecs,
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	err = parent.tick(ctx)
	if err != nil {
		t.Fatalf("tick failed: %v", err)
	}

	children := parent.GetChildren()

	child, exists := children["child-1"]
	if !exists {
		t.Fatal("Expected child-1 to exist")
	}

	typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
	if !ok {
		t.Fatal("Child supervisor should be correct type")
	}

	mappedState := typedChild.GetMappedParentState()
	if mappedState != "running" {
		t.Errorf("Expected child mappedParentState 'running' (empty ChildStartStates = always run), got '%s'", mappedState)
	}
}

func TestChildStartStates_NilSlice(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	registerTestWorkerFactories()

	defer factory.ResetRegistry()

	basicStore := memory.NewInMemoryStore()

	defer func() {
		_ = basicStore.Close(ctx)
	}()

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)
	// Collections are named after WorkerType, not child Name
	_ = basicStore.CreateCollection(ctx, "mqtt_client_identity", nil)
	_ = basicStore.CreateCollection(ctx, "mqtt_client_desired", nil)
	_ = basicStore.CreateCollection(ctx, "mqtt_client_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	// Nil slice = always runs (same as empty)
	childSpecs := []config.ChildSpec{
		{
			Name:             "child-1",
			WorkerType:       "mqtt_client",
			UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
			ChildStartStates: nil,
		},
	}

	worker := &mockWorkerWithChildren{
		identity:      identity,
		initialState:  &mockState{name: "starting"},
		observed:      persistence.Document{"id": "parent-1", "status": "starting"},
		childrenSpecs: childSpecs,
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	err = parent.tick(ctx)
	if err != nil {
		t.Fatalf("tick failed: %v", err)
	}

	children := parent.GetChildren()

	child, exists := children["child-1"]
	if !exists {
		t.Fatal("Expected child-1 to exist")
	}

	typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
	if !ok {
		t.Fatal("Child supervisor should be correct type")
	}

	mappedState := typedChild.GetMappedParentState()
	if mappedState != "running" {
		t.Errorf("Expected child mappedParentState 'running' (nil ChildStartStates = always run), got '%s'", mappedState)
	}
}
