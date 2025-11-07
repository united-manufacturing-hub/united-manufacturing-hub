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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

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

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_identity",
		WorkerType:    "test",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_desired",
		WorkerType:    "test",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_observed",
		WorkerType:    "test",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

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

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

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
	// 16. Verify: All phases integrated: Hierarchy + Templates + Variables + StateMapping + AsyncActions
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

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_identity",
		WorkerType:    "test",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_desired",
		WorkerType:    "test",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_observed",
		WorkerType:    "test",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

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

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

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

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_identity",
		WorkerType:    "test",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_desired",
		WorkerType:    "test",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_observed",
		WorkerType:    "test",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

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

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

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

	err = triangularStore.SaveDesired(ctx, "test", "worker-1", persistence.Document{
		"id":     "worker-1",
		"config": "production",
	})
	if err != nil {
		t.Fatalf("Failed to save desired: %v", err)
	}

	err = supervisor.Tick(ctx)
	if err != nil {
		t.Fatalf("Failed to tick: %v", err)
	}
}

type mockWorker struct {
	identity     fsmv2.Identity
	initialState fsmv2.State
	observed     persistence.Document
}

func (m *mockWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc:       m.observed,
		timestamp: time.Now(),
	}, nil
}

func (m *mockWorker) DeriveDesiredState(_ interface{}) (types.DesiredState, error) {
	return types.DesiredState{State: "running"}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State {
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

type mockDesiredState struct{}

func (m *mockDesiredState) ShutdownRequested() bool {
	return false
}

func (m *mockDesiredState) SetShutdownRequested(_ bool) {
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

func (m *mockState) Next(_ fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	return m, fsmv2.SignalNone, nil
}

func TestReconcileChildren_AddNewChildWhenNoneExist(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	specs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
	}

	err := supervisor.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("reconcileChildren failed: %v", err)
	}

	if len(supervisor.children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(supervisor.children))
	}

	if _, exists := supervisor.children["child-1"]; !exists {
		t.Error("Expected child-1 to exist in children map")
	}
}

func TestReconcileChildren_AddMultipleChildren(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	specs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
		{
			Name:       "child-2",
			WorkerType: "modbus_client",
			UserSpec:   types.UserSpec{Config: "address: 192.168.1.100:502"},
		},
		{
			Name:       "child-3",
			WorkerType: "opcua_client",
			UserSpec:   types.UserSpec{Config: "endpoint: opc.tcp://localhost:4840"},
		},
	}

	err := supervisor.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("reconcileChildren failed: %v", err)
	}

	if len(supervisor.children) != 3 {
		t.Errorf("Expected 3 children, got %d", len(supervisor.children))
	}

	for _, spec := range specs {
		if _, exists := supervisor.children[spec.Name]; !exists {
			t.Errorf("Expected %s to exist in children map", spec.Name)
		}
	}
}

func TestReconcileChildren_SkipExistingChild(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	specs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
	}

	err := supervisor.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("First reconcileChildren failed: %v", err)
	}

	firstChild := supervisor.children["child-1"]

	err = supervisor.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("Second reconcileChildren failed: %v", err)
	}

	if len(supervisor.children) != 1 {
		t.Errorf("Expected 1 child after second reconciliation, got %d", len(supervisor.children))
	}

	if supervisor.children["child-1"] != firstChild {
		t.Error("Expected child-1 to be the same instance after second reconciliation")
	}
}

func TestReconcileChildren_UpdateUserSpec(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	initialSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
	}

	err := supervisor.reconcileChildren(initialSpecs)
	if err != nil {
		t.Fatalf("Initial reconcileChildren failed: %v", err)
	}

	updatedSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://remotehost:1883"},
		},
	}

	err = supervisor.reconcileChildren(updatedSpecs)
	if err != nil {
		t.Fatalf("Update reconcileChildren failed: %v", err)
	}

	child := supervisor.children["child-1"]
	if child.userSpec.Config != "url: tcp://remotehost:1883" {
		t.Errorf("Expected UserSpec to be updated to 'url: tcp://remotehost:1883', got '%s'", child.userSpec.Config)
	}
}

func TestReconcileChildren_UpdateStateMapping(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	initialSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
			StateMapping: map[string]string{
				"running": "connected",
			},
		},
	}

	err := supervisor.reconcileChildren(initialSpecs)
	if err != nil {
		t.Fatalf("Initial reconcileChildren failed: %v", err)
	}

	updatedSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
			StateMapping: map[string]string{
				"running":  "connected",
				"stopping": "disconnected",
			},
		},
	}

	err = supervisor.reconcileChildren(updatedSpecs)
	if err != nil {
		t.Fatalf("Update reconcileChildren failed: %v", err)
	}

	child := supervisor.children["child-1"]
	if len(child.stateMapping) != 2 {
		t.Errorf("Expected StateMapping to have 2 entries, got %d", len(child.stateMapping))
	}
	if child.stateMapping["stopping"] != "disconnected" {
		t.Errorf("Expected stopping -> disconnected mapping, got %v", child.stateMapping["stopping"])
	}
}

func TestReconcileChildren_RemoveChild(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	initialSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
		{
			Name:       "child-2",
			WorkerType: "modbus_client",
			UserSpec:   types.UserSpec{Config: "address: 192.168.1.100:502"},
		},
	}

	err := supervisor.reconcileChildren(initialSpecs)
	if err != nil {
		t.Fatalf("Initial reconcileChildren failed: %v", err)
	}

	if len(supervisor.children) != 2 {
		t.Errorf("Expected 2 children initially, got %d", len(supervisor.children))
	}

	updatedSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
	}

	err = supervisor.reconcileChildren(updatedSpecs)
	if err != nil {
		t.Fatalf("Update reconcileChildren failed: %v", err)
	}

	if len(supervisor.children) != 1 {
		t.Errorf("Expected 1 child after removal, got %d", len(supervisor.children))
	}

	if _, exists := supervisor.children["child-2"]; exists {
		t.Error("Expected child-2 to be removed")
	}

	if _, exists := supervisor.children["child-1"]; !exists {
		t.Error("Expected child-1 to still exist")
	}
}

func TestReconcileChildren_MixedOperations(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	initialSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
		{
			Name:       "child-2",
			WorkerType: "modbus_client",
			UserSpec:   types.UserSpec{Config: "address: 192.168.1.100:502"},
		},
	}

	err := supervisor.reconcileChildren(initialSpecs)
	if err != nil {
		t.Fatalf("Initial reconcileChildren failed: %v", err)
	}

	mixedSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://remotehost:1883"},
		},
		{
			Name:       "child-3",
			WorkerType: "opcua_client",
			UserSpec:   types.UserSpec{Config: "endpoint: opc.tcp://localhost:4840"},
		},
	}

	err = supervisor.reconcileChildren(mixedSpecs)
	if err != nil {
		t.Fatalf("Mixed reconcileChildren failed: %v", err)
	}

	if len(supervisor.children) != 2 {
		t.Errorf("Expected 2 children after mixed operations, got %d", len(supervisor.children))
	}

	if _, exists := supervisor.children["child-1"]; !exists {
		t.Error("Expected child-1 to exist after update")
	}

	if supervisor.children["child-1"].userSpec.Config != "url: tcp://remotehost:1883" {
		t.Error("Expected child-1 UserSpec to be updated")
	}

	if _, exists := supervisor.children["child-2"]; exists {
		t.Error("Expected child-2 to be removed")
	}

	if _, exists := supervisor.children["child-3"]; !exists {
		t.Error("Expected child-3 to be added")
	}
}

func TestReconcileChildren_EmptySpecs(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	initialSpecs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
		{
			Name:       "child-2",
			WorkerType: "modbus_client",
			UserSpec:   types.UserSpec{Config: "address: 192.168.1.100:502"},
		},
	}

	err := supervisor.reconcileChildren(initialSpecs)
	if err != nil {
		t.Fatalf("Initial reconcileChildren failed: %v", err)
	}

	if len(supervisor.children) != 2 {
		t.Errorf("Expected 2 children initially, got %d", len(supervisor.children))
	}

	err = supervisor.reconcileChildren([]types.ChildSpec{})
	if err != nil {
		t.Fatalf("Empty reconcileChildren failed: %v", err)
	}

	if len(supervisor.children) != 0 {
		t.Errorf("Expected 0 children after empty reconciliation, got %d", len(supervisor.children))
	}
}

func TestApplyStateMapping_WithMapping(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor(supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "idle"},
		observed: persistence.Document{
			"id":     "parent-1",
			"status": "idle",
		},
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	specs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
			StateMapping: map[string]string{
				"idle":   "stopped",
				"active": "connected",
			},
		},
	}

	err = parent.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("reconcileChildren failed: %v", err)
	}

	parent.applyStateMapping()

	child := parent.children["child-1"]
	if child.mappedParentState != "stopped" {
		t.Errorf("Expected child mappedParentState 'stopped', got '%s'", child.mappedParentState)
	}
}

func TestApplyStateMapping_NoMapping(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor(supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "running"},
		observed: persistence.Document{
			"id":     "parent-1",
			"status": "running",
		},
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	specs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
		},
	}

	err = parent.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("reconcileChildren failed: %v", err)
	}

	parent.applyStateMapping()

	child := parent.children["child-1"]
	if child.mappedParentState != "running" {
		t.Errorf("Expected child mappedParentState 'running' (parent state), got '%s'", child.mappedParentState)
	}
}

func TestApplyStateMapping_MissingStateInMapping(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor(supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "unknown"},
		observed: persistence.Document{
			"id":     "parent-1",
			"status": "unknown",
		},
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	specs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
			StateMapping: map[string]string{
				"idle":   "stopped",
				"active": "connected",
			},
		},
	}

	err = parent.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("reconcileChildren failed: %v", err)
	}

	parent.applyStateMapping()

	child := parent.children["child-1"]
	if child.mappedParentState != "unknown" {
		t.Errorf("Expected child mappedParentState 'unknown' (parent state when not in mapping), got '%s'", child.mappedParentState)
	}
}

func TestApplyStateMapping_MultipleChildren(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor(supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "active"},
		observed: persistence.Document{
			"id":     "parent-1",
			"status": "active",
		},
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	specs := []types.ChildSpec{
		{
			Name:       "child-1",
			WorkerType: "mqtt_client",
			UserSpec:   types.UserSpec{Config: "url: tcp://localhost:1883"},
			StateMapping: map[string]string{
				"active": "connected",
				"idle":   "disconnected",
			},
		},
		{
			Name:       "child-2",
			WorkerType: "modbus_client",
			UserSpec:   types.UserSpec{Config: "address: 192.168.1.100:502"},
			StateMapping: map[string]string{
				"active": "polling",
				"idle":   "stopped",
			},
		},
		{
			Name:       "child-3",
			WorkerType: "opcua_client",
			UserSpec:   types.UserSpec{Config: "endpoint: opc.tcp://localhost:4840"},
		},
	}

	err = parent.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("reconcileChildren failed: %v", err)
	}

	parent.applyStateMapping()

	child1 := parent.children["child-1"]
	if child1.mappedParentState != "connected" {
		t.Errorf("Expected child-1 mappedParentState 'connected', got '%s'", child1.mappedParentState)
	}

	child2 := parent.children["child-2"]
	if child2.mappedParentState != "polling" {
		t.Errorf("Expected child-2 mappedParentState 'polling', got '%s'", child2.mappedParentState)
	}

	child3 := parent.children["child-3"]
	if child3.mappedParentState != "active" {
		t.Errorf("Expected child-3 mappedParentState 'active' (parent state, no mapping), got '%s'", child3.mappedParentState)
	}
}

func TestApplyStateMapping_EmptyStateMapping(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor(supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "running"},
		observed: persistence.Document{
			"id":     "parent-1",
			"status": "running",
		},
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	specs := []types.ChildSpec{
		{
			Name:         "child-1",
			WorkerType:   "mqtt_client",
			UserSpec:     types.UserSpec{Config: "url: tcp://localhost:1883"},
			StateMapping: map[string]string{},
		},
	}

	err = parent.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("reconcileChildren failed: %v", err)
	}

	parent.applyStateMapping()

	child := parent.children["child-1"]
	if child.mappedParentState != "running" {
		t.Errorf("Expected child mappedParentState 'running' (parent state with empty map), got '%s'", child.mappedParentState)
	}
}

func TestApplyStateMapping_NilStateMapping(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_identity",
		WorkerType:    "parent",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_desired",
		WorkerType:    "parent",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "parent_observed",
		WorkerType:    "parent",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "parent_identity", nil)
	_ = basicStore.CreateCollection(ctx, "parent_desired", nil)
	_ = basicStore.CreateCollection(ctx, "parent_observed", nil)

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "parent",
		Store:      triangularStore,
		Logger:     logger,
	}

	parent := NewSupervisor(supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "parent-1",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "starting"},
		observed: persistence.Document{
			"id":     "parent-1",
			"status": "starting",
		},
	}

	err := parent.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add parent worker: %v", err)
	}

	specs := []types.ChildSpec{
		{
			Name:         "child-1",
			WorkerType:   "mqtt_client",
			UserSpec:     types.UserSpec{Config: "url: tcp://localhost:1883"},
			StateMapping: nil,
		},
	}

	err = parent.reconcileChildren(specs)
	if err != nil {
		t.Fatalf("reconcileChildren failed: %v", err)
	}

	parent.applyStateMapping()

	child := parent.children["child-1"]
	if child.mappedParentState != "starting" {
		t.Errorf("Expected child mappedParentState 'starting' (parent state with nil map), got '%s'", child.mappedParentState)
	}
}
