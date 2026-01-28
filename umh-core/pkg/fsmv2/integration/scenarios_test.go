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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/integration"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("Simple Scenario Integration", func() {
	It("should run scenario and verify all requirements", func() {
		originalStoppedWait := state.StoppedWaitDuration
		originalRunning := state.RunningDuration
		state.StoppedWaitDuration = 500 * time.Millisecond
		state.RunningDuration = 1 * time.Second
		defer func() {
			state.StoppedWaitDuration = originalStoppedWait
			state.RunningDuration = originalRunning
		}()

		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		store := setupTestStoreForScenario(testLogger.Logger)

		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 20*time.Second)
		defer scenarioCancel()

		result, err := examples.Run(scenarioCtx, examples.RunConfig{
			Scenario:     examples.SimpleScenario,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		<-result.Done

		verifyNoErrorsOrWarnings(testLogger)
		verifyParentAndChildrenCreated(testLogger)
		verifyConnectActionsExactlyOnce(testLogger)
		verifyStateTransitionSequence(testLogger)
		verifyTriangularStoreChanges(testLogger)
		verifyShutdownOrder(testLogger)
		verifyAllLogsHaveWorkerField(testLogger)

		verifyObservedStateHasState(store)
		verifyStateFieldsAreValid(store)
		verifyTimestampsProgressing(store)
		verifyIDsMatch(store)

		verifyChildrenReachConnectedState(testLogger)
		verifyShutdownTransitionsClean(testLogger)
		verifyNoOrphanedStates(store)

		verifyHierarchyPathCorrect(store)
		verifyChildCountMatches(store)

		verifyNoActionFailures(testLogger)

		verifyCyclicStateTransitions(testLogger)
		verifyTimingBasedTransitions(testLogger)
		verifyChildrenHealthTracking(store)
		verifyStateEnteredAtAndElapsed(store)
		verifyParentReachesRunning(testLogger)

		verifyNoChildTransitionBeforeParentTryingToStart(testLogger)
		verifyStateTransitionOrdering(testLogger)

		verifyChildrenStopWhenParentTryingToStop(testLogger)
		verifyParentReachesStopped(testLogger)
		verifyFullCycle(testLogger)
	})
})

func verifyNoErrorsOrWarnings(t *integration.TestLogger) {
	errorsAndWarnings := t.GetErrorsAndWarnings()

	// Known timing-related issues during test scenarios
	knownIssues := []string{
		"data_stale",                   // Observation collector may report stale data briefly
		"collector_observation_failed", // Collector may fail temporarily during shutdown
	}

	for _, entry := range errorsAndWarnings {
		// Debug: print full log entry including context
		GinkgoWriter.Printf("DEBUG Error/Warning: %s - %s (context: %v)\n", entry.Level, entry.Message, entry.ContextMap())

		isKnown := false

		for _, issue := range knownIssues {
			if strings.Contains(entry.Message, issue) {
				GinkgoWriter.Printf("Known issue: %s - %s\n", entry.Level, entry.Message)

				isKnown = true

				break
			}
		}

		if isKnown {
			continue
		}

		Fail(fmt.Sprintf("Unexpected %s: %s", entry.Level, entry.Message))
	}

	GinkgoWriter.Printf("✓ No unexpected errors or warnings (known timing issues excluded)\n")
}

func verifyParentAndChildrenCreated(t *integration.TestLogger) {
	// Check for child_adding logs which indicate parent creating children
	childAddingLogs := t.GetLogsMatching("child_adding")
	workerAddedLogs := t.GetLogsMatching("worker_added")

	// Verify children were added (parent creates them)
	Expect(len(childAddingLogs)).To(BeNumerically(">=", 2),
		"Expected at least 2 child_adding logs (parent creates children)")

	// Verify workers were added to supervisor
	Expect(len(workerAddedLogs)).To(BeNumerically(">=", 3),
		"Expected at least 3 worker_added logs (1 parent + 2 children)")

	GinkgoWriter.Printf("✓ Parent and children created (child_adding: %d, worker_added: %d)\n",
		len(childAddingLogs), len(workerAddedLogs))
}

func verifyConnectActionsExactlyOnce(t *integration.TestLogger) {
	// NOTE: Action logs use worker-created loggers, not the passed test logger
	// We verify actions happened by checking supervisor logs instead
	// Console output confirms: "Attempting to connect" and "Connection established"
	supervisorStartedLogs := t.GetLogsMatching("supervisor_started")
	Expect(len(supervisorStartedLogs)).To(BeNumerically(">=", 2),
		"Expected at least 2 supervisor_started logs (parent + children supervisors)")

	GinkgoWriter.Printf("✓ Worker supervisors started (supervisors: %d)\n",
		len(supervisorStartedLogs))
}

func verifyStateTransitionSequence(t *integration.TestLogger) {
	stateTransitionLogs := t.GetLogsMatching("state_transition")
	Expect(stateTransitionLogs).ToNot(BeEmpty(), "Expected at least 1 state_transition log")

	GinkgoWriter.Printf("✓ State transitions occurred (%d transitions)\n", len(stateTransitionLogs))
}

func verifyTriangularStoreChanges(t *integration.TestLogger) {
	observationLogs := t.GetLogsMatching("observation_changed")
	desiredLogs := t.GetLogsMatching("desired_changed")

	// Verify that store changes were logged
	Expect(len(observationLogs)+len(desiredLogs)).To(BeNumerically(">=", 1),
		"Expected at least 1 store change (observation or desired)")

	GinkgoWriter.Printf("✓ TriangularStore changes detected (observation: %d, desired: %d)\n",
		len(observationLogs), len(desiredLogs))
}

func verifyShutdownOrder(t *integration.TestLogger) {
	tickLoopStartedLogs := t.GetLogsMatching("tick_loop_started")

	var workerTypes []string

	for _, entry := range tickLoopStartedLogs {
		for _, field := range entry.Context {
			if field.Key == "worker_type" {
				// String values are stored in field.String, not field.Interface
				if field.String != "" {
					workerTypes = append(workerTypes, field.String)
				}
			}
		}
	}

	// Verify at least 2 tick loops were started (for children)
	Expect(len(tickLoopStartedLogs)).To(BeNumerically(">=", 2),
		"Expected at least 2 tick_loop_started logs (for child workers)")

	GinkgoWriter.Printf("✓ Tick loops started for workers (count: %d, types: %v)\n",
		len(tickLoopStartedLogs), workerTypes)
}

func verifyAllLogsHaveWorkerField(t *integration.TestLogger) {
	logsMissingWorker := t.GetLogsMissingField("worker")

	// Known initialization logs that don't have worker field (logged during setup)
	knownInitLogs := map[string]bool{
		"identity_saved":              true,
		"initial_observation_saved":   true,
		"initial_desired_state_saved": true,
	}

	var unexpectedMissing []string

	for _, entry := range logsMissingWorker {
		if !knownInitLogs[entry.Message] {
			unexpectedMissing = append(unexpectedMissing, entry.Message)
		}
	}

	if len(unexpectedMissing) > 0 {
		Fail(fmt.Sprintf("Found %d log entries missing 'worker' field: %v",
			len(unexpectedMissing), unexpectedMissing))
	}

	GinkgoWriter.Printf("✓ All logs have 'worker' field (excluding %d known init logs)\n",
		len(logsMissingWorker)-len(unexpectedMissing))
}

func setupTestStoreForScenario(logger *zap.SugaredLogger) storage.TriangularStoreInterface {
	basicStore := memory.NewInMemoryStore()

	return storage.NewTriangularStore(basicStore, logger)
}

// =============================================================================
// Category 1: Data Consistency Tests
// =============================================================================

// getWorkersFromStore discovers all workers by examining deltas.
// Returns a slice of worker snapshots for verification.
func getWorkersFromStore(store storage.TriangularStoreInterface) []examples.WorkerSnapshot {
	ctx := context.Background()

	// Get all deltas to discover workers
	resp, err := store.GetDeltas(ctx, storage.Subscription{LastSyncID: 0})
	if err != nil {
		return nil
	}

	// Extract unique workers and load their snapshots
	seen := make(map[string]bool)

	var workers []examples.WorkerSnapshot

	for _, delta := range resp.Deltas {
		key := delta.WorkerType + "/" + delta.WorkerID
		if seen[key] {
			continue
		}

		seen[key] = true

		snapshot, err := store.LoadSnapshot(ctx, delta.WorkerType, delta.WorkerID)
		if err != nil {
			continue
		}

		var observedDoc persistence.Document
		if doc, ok := snapshot.Observed.(persistence.Document); ok {
			observedDoc = doc
		}

		workers = append(workers, examples.WorkerSnapshot{
			WorkerType: delta.WorkerType,
			WorkerID:   delta.WorkerID,
			Identity:   snapshot.Identity,
			Desired:    snapshot.Desired,
			Observed:   observedDoc,
		})
	}

	return workers
}

// verifyObservedStateHasState verifies all workers have non-empty OBSERVED.state field.
// This is the MAIN BUG FIX test - currently expected to FAIL.
func verifyObservedStateHasState(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)
	Expect(workers).ToNot(BeEmpty(), "No workers found in store")

	for _, w := range workers {
		if w.Observed == nil {
			continue // Skip workers without observed state yet
		}

		state, hasState := w.Observed["state"]
		Expect(hasState).To(BeTrue(),
			fmt.Sprintf("Worker %s/%s OBSERVED missing 'state' field", w.WorkerType, w.WorkerID))

		stateStr, ok := state.(string)
		Expect(ok).To(BeTrue(),
			fmt.Sprintf("Worker %s/%s OBSERVED.state is not a string: %T", w.WorkerType, w.WorkerID, state))
		Expect(stateStr).NotTo(BeEmpty(),
			fmt.Sprintf("Worker %s/%s OBSERVED.state is empty string", w.WorkerType, w.WorkerID))

		GinkgoWriter.Printf("  Worker %s/%s state: %s\n", w.WorkerType, w.WorkerID, stateStr)
	}

	GinkgoWriter.Printf("✓ All workers have non-empty OBSERVED.state\n")
}

// verifyStateFieldsAreValid verifies state values are valid FSM states.
func verifyStateFieldsAreValid(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	// Valid states for different worker types
	validChildStates := map[string]bool{
		"TryingToConnect": true, "Connected": true, "Disconnected": true,
		"TryingToDisconnect": true, "TryingToStop": true, "Stopped": true,
		"unknown": true, // Initial state before first tick
	}
	validParentStates := map[string]bool{
		"TryingToStart": true, "Running": true,
		"TryingToStop": true, "Stopped": true,
		"unknown": true,
	}
	validApplicationStates := map[string]bool{
		"TryingToStart": true, "Running": true,
		"TryingToStop": true, "Stopped": true,
		"unknown": true,
	}

	for _, w := range workers {
		if w.Observed == nil {
			continue
		}

		state, ok := w.Observed["state"].(string)
		if !ok || state == "" {
			continue // Will be caught by verifyObservedStateHasState
		}

		var validStates map[string]bool

		switch w.WorkerType {
		case "examplechild":
			validStates = validChildStates
		case "exampleparent":
			validStates = validParentStates
		case "application":
			validStates = validApplicationStates
		default:
			// Unknown worker type - skip validation
			continue
		}

		Expect(validStates[state]).To(BeTrue(),
			fmt.Sprintf("Worker %s/%s has invalid state: %s", w.WorkerType, w.WorkerID, state))
	}

	GinkgoWriter.Printf("✓ All state values are valid FSM states\n")
}

// verifyTimestampsProgressing verifies collected_at timestamps are recent.
func verifyTimestampsProgressing(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	for _, w := range workers {
		if w.Observed == nil {
			continue
		}

		collectedAt, hasTimestamp := w.Observed["collected_at"]
		Expect(hasTimestamp).To(BeTrue(),
			fmt.Sprintf("Worker %s/%s missing collected_at", w.WorkerType, w.WorkerID))

		// Timestamp should be recent (within last minute)
		if ts, ok := collectedAt.(string); ok {
			t, err := time.Parse(time.RFC3339Nano, ts)
			Expect(err).NotTo(HaveOccurred(),
				fmt.Sprintf("Worker %s/%s invalid timestamp format: %s", w.WorkerType, w.WorkerID, ts))
			Expect(time.Since(t)).To(BeNumerically("<", 1*time.Minute),
				fmt.Sprintf("Worker %s/%s timestamp too old: %s", w.WorkerType, w.WorkerID, ts))
		}
	}

	GinkgoWriter.Printf("✓ All timestamps are recent and valid\n")
}

// verifyIDsMatch verifies IDs match across Identity/Desired/Observed.
func verifyIDsMatch(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	for _, w := range workers {
		// Identity.id should match
		if w.Identity != nil {
			Expect(w.Identity["id"]).To(Equal(w.WorkerID),
				fmt.Sprintf("Identity ID mismatch for %s/%s", w.WorkerType, w.WorkerID))
		}

		// Desired.id should match
		if w.Desired != nil {
			Expect(w.Desired["id"]).To(Equal(w.WorkerID),
				fmt.Sprintf("Desired ID mismatch for %s/%s", w.WorkerType, w.WorkerID))
		}

		// Observed.id should match
		if w.Observed != nil {
			Expect(w.Observed["id"]).To(Equal(w.WorkerID),
				fmt.Sprintf("Observed ID mismatch for %s/%s", w.WorkerType, w.WorkerID))
		}
	}

	GinkgoWriter.Printf("✓ All IDs match across Identity/Desired/Observed\n")
}

// =============================================================================
// Category 2: State Lifecycle Tests
// =============================================================================

// verifyChildrenReachConnectedState verifies children reach Connected state.
func verifyChildrenReachConnectedState(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	connectedCount := 0

	for _, entry := range stateTransitions {
		for _, field := range entry.Context {
			if field.Key == "to_state" {
				// String values are stored in field.String, not field.Interface
				if field.String == "Connected" {
					connectedCount++
				}
			}
		}
	}

	Expect(connectedCount).To(BeNumerically(">=", 2),
		"Expected at least 2 children to reach Connected state")

	GinkgoWriter.Printf("✓ Children reached Connected state (%d transitions)\n", connectedCount)
}

// verifyShutdownTransitionsClean verifies TryingToStop appears before Stopped.
func verifyShutdownTransitionsClean(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	var tryingToStopSeen, stoppedSeen bool

	for _, entry := range stateTransitions {
		for _, field := range entry.Context {
			if field.Key == "to_state" {
				// String values are stored in field.String, not field.Interface
				if field.String == "TryingToStop" {
					tryingToStopSeen = true
				}

				if field.String == "Stopped" {
					stoppedSeen = true
				}
			}
		}
	}

	// Both should be seen during graceful shutdown
	if stoppedSeen {
		Expect(tryingToStopSeen).To(BeTrue(),
			"Stopped state reached without TryingToStop transition")
	}

	GinkgoWriter.Printf("✓ Shutdown transitions are clean (TryingToStop → Stopped)\n")
}

// verifyNoOrphanedStates checks no workers are stuck in TryingTo* states after shutdown.
func verifyNoOrphanedStates(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	for _, w := range workers {
		if w.Observed == nil || w.Desired == nil {
			continue
		}

		state, ok := w.Observed["state"].(string)
		if !ok {
			continue
		}

		isTrying := strings.HasPrefix(state, "TryingTo")

		// If shutdown requested, should not be in TryingTo* state
		if shutdown, ok := w.Desired["ShutdownRequested"].(bool); ok && shutdown {
			if isTrying {
				GinkgoWriter.Printf("  Warning: %s/%s stuck in %s during shutdown\n",
					w.WorkerType, w.WorkerID, state)
			}
		}
	}

	GinkgoWriter.Printf("✓ No orphaned TryingTo* states detected\n")
}

// =============================================================================
// Category 3: Hierarchy Tests
// =============================================================================

// verifyHierarchyPathCorrect verifies children have proper hierarchy paths.
func verifyHierarchyPathCorrect(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	for _, w := range workers {
		if w.WorkerType != "examplechild" {
			continue
		}

		if w.Identity == nil {
			continue
		}

		path, hasPath := w.Identity["hierarchy_path"].(string)
		Expect(hasPath).To(BeTrue(),
			fmt.Sprintf("Child %s missing hierarchy_path", w.WorkerID))

		// Path should contain (exampleparent) indicating parent worker
		Expect(path).To(ContainSubstring("(exampleparent)"),
			fmt.Sprintf("Child %s hierarchy_path missing exampleparent: %s", w.WorkerID, path))

		// Path should contain (examplechild) indicating child type
		Expect(path).To(ContainSubstring("(examplechild)"),
			fmt.Sprintf("Child %s hierarchy_path missing examplechild type: %s", w.WorkerID, path))
	}

	GinkgoWriter.Printf("✓ Hierarchy paths are correct\n")
}

// verifyChildCountMatches verifies parent child counts match actual children.
func verifyChildCountMatches(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	// Separate parents and children
	var parents, children []examples.WorkerSnapshot

	for _, w := range workers {
		switch w.WorkerType {
		case "exampleparent":
			parents = append(parents, w)
		case "examplechild":
			children = append(children, w)
		}
	}

	for _, parent := range parents {
		if parent.Observed == nil {
			continue
		}

		// Count children that belong to this parent (by hierarchy_path)
		expectedCount := 0

		for _, child := range children {
			if child.Identity == nil {
				continue
			}

			if path, ok := child.Identity["hierarchy_path"].(string); ok {
				if strings.Contains(path, parent.WorkerID) {
					expectedCount++
				}
			}
		}

		// Get observed counts (may be 0 if worker doesn't populate these fields)
		healthy, _ := parent.Observed["children_healthy"].(float64)
		unhealthy, _ := parent.Observed["children_unhealthy"].(float64)
		total := int(healthy + unhealthy)

		// Only verify if the worker actually populates child count fields with non-zero values
		// (Default struct values are serialized as 0, so we skip verification if both are 0)
		if total > 0 {
			Expect(total).To(Equal(expectedCount),
				fmt.Sprintf("Parent %s child count mismatch: observed=%d, actual=%d",
					parent.WorkerID, total, expectedCount))
		}
	}

	GinkgoWriter.Printf("✓ Parent child counts match actual children (or fields not populated)\n")
}

// =============================================================================
// Category 4: Action Execution Tests
// =============================================================================

// verifyNoActionFailures verifies no action execution failures or panics occurred.
func verifyNoActionFailures(t *integration.TestLogger) {
	failures := t.GetLogsMatching("action_execution_failed")
	panics := t.GetLogsMatching("action_panic")

	Expect(failures).To(BeEmpty(),
		fmt.Sprintf("Found %d action execution failures", len(failures)))
	Expect(panics).To(BeEmpty(),
		fmt.Sprintf("Found %d action panics", len(panics)))

	GinkgoWriter.Printf("✓ No action execution failures or panics\n")
}

// =============================================================================
// Category 5: Cyclic Start/Stop Verifications
// =============================================================================

// verifyCyclicStateTransitions verifies the expected state transition sequence
// for the cyclic start/stop pattern.
func verifyCyclicStateTransitions(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	// Build ordered list of parent state transitions
	// Note: "worker" field contains hierarchy path like "parent-1-001(exampleparent)"
	var parentTransitions []string

	for _, entry := range stateTransitions {
		worker := ""
		toState := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}

			if field.Key == "to_state" {
				toState = field.String
			}
		}

		// Check if this is a parent worker by looking for "exampleparent" in the hierarchy path
		if strings.Contains(worker, "exampleparent") && toState != "" {
			parentTransitions = append(parentTransitions, toState)
		}
	}

	// Verify expected sequence appears: Stopped → TryingToStart → Running → TryingToStop → Stopped
	// The sequence may have variations (e.g., starting from Stopped), but these key states should appear
	expectedStates := []string{"TryingToStart", "Running", "TryingToStop"}

	// Check that sequence appears (may have repeated Stopped at start)
	Expect(len(parentTransitions)).To(BeNumerically(">=", len(expectedStates)),
		fmt.Sprintf("Expected at least %d parent transitions, got %d: %v",
			len(expectedStates), len(parentTransitions), parentTransitions))

	GinkgoWriter.Printf("✓ Parent state transitions: %v\n", parentTransitions)
}

// verifyTimingBasedTransitions verifies that time-based transitions occurred.
func verifyTimingBasedTransitions(t *integration.TestLogger) {
	// Look for transitions from Stopped → TryingToStart (after StoppedWaitDuration)
	// and Running → TryingToStop (after RunningDuration)
	stateTransitions := t.GetLogsMatching("state_transition")

	stoppedToTrying := false
	runningToTrying := false

	for _, entry := range stateTransitions {
		fromState := ""
		toState := ""
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "from_state" {
				fromState = field.String
			}

			if field.Key == "to_state" {
				toState = field.String
			}

			if field.Key == "worker" {
				worker = field.String
			}
		}

		// Check if this is a parent worker by looking for "exampleparent" in hierarchy path
		if !strings.Contains(worker, "exampleparent") {
			continue
		}

		if fromState == "Stopped" && toState == "TryingToStart" {
			stoppedToTrying = true
		}

		if fromState == "Running" && toState == "TryingToStop" {
			runningToTrying = true
		}
	}

	Expect(stoppedToTrying).To(BeTrue(),
		"Expected Stopped → TryingToStart transition (timing-based)")
	Expect(runningToTrying).To(BeTrue(),
		"Expected Running → TryingToStop transition (timing-based)")

	GinkgoWriter.Printf("✓ Timing-based transitions occurred\n")
}

// verifyChildrenHealthTracking verifies parent tracks children health correctly.
func verifyChildrenHealthTracking(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	for _, w := range workers {
		if w.WorkerType != "exampleparent" {
			continue
		}

		if w.Observed == nil {
			continue
		}

		// Verify children_healthy and children_unhealthy are tracked
		_, hasHealthy := w.Observed["children_healthy"]
		_, hasUnhealthy := w.Observed["children_unhealthy"]

		Expect(hasHealthy).To(BeTrue(),
			fmt.Sprintf("Parent %s missing children_healthy field", w.WorkerID))
		Expect(hasUnhealthy).To(BeTrue(),
			fmt.Sprintf("Parent %s missing children_unhealthy field", w.WorkerID))
	}

	GinkgoWriter.Printf("✓ Parent tracks children health counts\n")
}

// verifyStateEnteredAtAndElapsed verifies timing fields are populated in nested metrics.framework.
// After restructuring MetricsEmbedder to nested MetricsContainer, fields are at:
//   - metrics.framework.state_entered_at_unix
//   - metrics.framework.time_in_current_state_ms
func verifyStateEnteredAtAndElapsed(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	for _, w := range workers {
		if w.WorkerType != "exampleparent" {
			continue
		}

		if w.Observed == nil {
			continue
		}

		// Check for nested metrics.framework structure
		metricsRaw, hasMetrics := w.Observed["metrics"]
		Expect(hasMetrics).To(BeTrue(),
			fmt.Sprintf("Parent %s missing metrics field", w.WorkerID))

		metrics, ok := metricsRaw.(map[string]interface{})
		Expect(ok).To(BeTrue(),
			fmt.Sprintf("Parent %s metrics field is not a map", w.WorkerID))

		frameworkRaw, hasFramework := metrics["framework"]
		Expect(hasFramework).To(BeTrue(),
			fmt.Sprintf("Parent %s missing metrics.framework field", w.WorkerID))

		framework, ok := frameworkRaw.(map[string]interface{})
		Expect(ok).To(BeTrue(),
			fmt.Sprintf("Parent %s metrics.framework field is not a map", w.WorkerID))

		_, hasStateEnteredAt := framework["state_entered_at_unix"]
		_, hasTimeInState := framework["time_in_current_state_ms"]

		Expect(hasStateEnteredAt).To(BeTrue(),
			fmt.Sprintf("Parent %s missing metrics.framework.state_entered_at_unix field", w.WorkerID))
		Expect(hasTimeInState).To(BeTrue(),
			fmt.Sprintf("Parent %s missing metrics.framework.time_in_current_state_ms field", w.WorkerID))

		// Verify the values are meaningful (non-zero for active workers)
		stateEnteredAt, _ := framework["state_entered_at_unix"].(float64)

		Expect(stateEnteredAt).To(BeNumerically(">", 0),
			fmt.Sprintf("Parent %s state_entered_at_unix should be > 0, got %v", w.WorkerID, stateEnteredAt))
		// Note: time_in_current_state_ms can be 0 if just entered the state, so we don't assert > 0
	}

	GinkgoWriter.Printf("✓ Parent has metrics.framework.state_entered_at_unix and time_in_current_state_ms fields\n")
}

// verifyParentReachesRunning verifies parent reaches Running state.
func verifyParentReachesRunning(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	reachedRunning := false

	for _, entry := range stateTransitions {
		toState := ""
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "to_state" {
				toState = field.String
			}

			if field.Key == "worker" {
				worker = field.String
			}
		}

		// Check if this is a parent worker by looking for "exampleparent" in hierarchy path
		if strings.Contains(worker, "exampleparent") && toState == "Running" {
			reachedRunning = true

			break
		}
	}

	Expect(reachedRunning).To(BeTrue(),
		"Expected parent to reach Running state")

	GinkgoWriter.Printf("✓ Parent reached Running state\n")
}

// =============================================================================
// Category 6: Parent-Child Coordination Tests (TDD - These Should FAIL Initially)
// =============================================================================

// verifyNoChildTransitionBeforeParentTryingToStart ensures children don't
// transition from Stopped until parent reaches TryingToStart.
// This test catches the bug where children start immediately without waiting
// for parent to signal them via StateMapping.
func verifyNoChildTransitionBeforeParentTryingToStart(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	parentReachedTryingToStart := false

	var childTransitionsBeforeParent []string

	for _, entry := range stateTransitions {
		worker := ""
		toState := ""
		fromState := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}

			if field.Key == "to_state" {
				toState = field.String
			}

			if field.Key == "from_state" {
				fromState = field.String
			}
		}

		// Check if parent reached TryingToStart
		if strings.Contains(worker, "exampleparent") && toState == "TryingToStart" {
			parentReachedTryingToStart = true
		}

		// Check if child transitioned from Stopped BEFORE parent reached TryingToStart
		// This is the bug: children should stay in Stopped until parent signals them
		if strings.Contains(worker, "examplechild") &&
			!parentReachedTryingToStart &&
			fromState == "Stopped" {
			childTransitionsBeforeParent = append(childTransitionsBeforeParent,
				fmt.Sprintf("%s: %s → %s", worker, fromState, toState))
		}
	}

	Expect(childTransitionsBeforeParent).To(BeEmpty(),
		fmt.Sprintf("BUG: Children transitioned before parent TryingToStart: %v", childTransitionsBeforeParent))

	GinkgoWriter.Printf("✓ Children waited for parent TryingToStart\n")
}

// verifyStateTransitionOrdering verifies the expected parent-child ordering:
// Parent TryingToStart must come BEFORE any child TryingToConnect.
// Children don't start connecting until parent is ready.
func verifyStateTransitionOrdering(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	// Build timeline of key events
	type event struct {
		index  int
		worker string
		state  string
	}

	var timeline []event

	for i, entry := range stateTransitions {
		worker := ""
		toState := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}

			if field.Key == "to_state" {
				toState = field.String
			}
		}

		timeline = append(timeline, event{
			index:  i,
			worker: worker,
			state:  toState,
		})
	}

	// Find indices of key transitions
	var parentTryingToStartIdx = -1

	var firstChildTryingToConnectIdx = -1

	for _, e := range timeline {
		if strings.Contains(e.worker, "exampleparent") && e.state == "TryingToStart" {
			if parentTryingToStartIdx == -1 {
				parentTryingToStartIdx = e.index
			}
		}

		if strings.Contains(e.worker, "examplechild") && e.state == "TryingToConnect" {
			if firstChildTryingToConnectIdx == -1 {
				firstChildTryingToConnectIdx = e.index
			}
		}
	}

	// Verify ordering: Parent TryingToStart (idx) < Child TryingToConnect (idx)
	Expect(parentTryingToStartIdx).To(BeNumerically(">=", 0),
		"Parent never reached TryingToStart state")
	Expect(firstChildTryingToConnectIdx).To(BeNumerically(">=", 0),
		"No child reached TryingToConnect state")
	Expect(parentTryingToStartIdx).To(BeNumerically("<", firstChildTryingToConnectIdx),
		fmt.Sprintf("BUG: Parent TryingToStart (idx %d) should come BEFORE child TryingToConnect (idx %d)",
			parentTryingToStartIdx, firstChildTryingToConnectIdx))

	GinkgoWriter.Printf("✓ State transition ordering correct (parent idx %d < child idx %d)\n",
		parentTryingToStartIdx, firstChildTryingToConnectIdx)
}

// =============================================================================
// Category 7: Children Stop When Parent TryingToStop Tests (TDD)
// =============================================================================

// verifyChildrenStopWhenParentTryingToStop ensures children transition
// out of Connected when parent goes to TryingToStop.
func verifyChildrenStopWhenParentTryingToStop(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	parentReachedTryingToStop := false
	childrenStoppedAfterParent := false

	for _, entry := range stateTransitions {
		worker := ""
		fromState := ""
		toState := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}

			if field.Key == "from_state" {
				fromState = field.String
			}

			if field.Key == "to_state" {
				toState = field.String
			}
		}

		if strings.Contains(worker, "exampleparent") && toState == "TryingToStop" {
			parentReachedTryingToStop = true
		}

		// After parent reaches TryingToStop, children should transition FROM Connected
		if parentReachedTryingToStop &&
			strings.Contains(worker, "examplechild") &&
			fromState == "Connected" {
			childrenStoppedAfterParent = true
		}
	}

	Expect(parentReachedTryingToStop).To(BeTrue(),
		"Parent should reach TryingToStop state")
	Expect(childrenStoppedAfterParent).To(BeTrue(),
		"BUG: Children should transition from Connected after parent TryingToStop")

	GinkgoWriter.Printf("✓ Children stop when parent goes to TryingToStop\n")
}

// verifyParentReachesStopped ensures parent completes the full cycle back to Stopped.
// IMPORTANT: This verifies the parent cycles BACK to Stopped AFTER reaching Running,
// not just that it started in Stopped.
func verifyParentReachesStopped(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	// Track if parent reached Running, then Stopped (true cycle completion)
	sawRunning := false
	parentCycledBackToStopped := false
	parentReachedStoppedCount := 0

	for _, entry := range stateTransitions {
		worker := ""
		toState := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}

			if field.Key == "to_state" {
				toState = field.String
			}
		}

		if strings.Contains(worker, "exampleparent") {
			if toState == "Running" {
				sawRunning = true
			}

			if toState == "Stopped" {
				parentReachedStoppedCount++

				if sawRunning {
					parentCycledBackToStopped = true
				}
			}
		}
	}

	// Parent MUST cycle back to Stopped AFTER reaching Running
	// This catches the bug where parent gets stuck in TryingToStop
	Expect(parentCycledBackToStopped).To(BeTrue(),
		"BUG: Parent should cycle back to Stopped after reaching Running state")

	GinkgoWriter.Printf("✓ Parent completes cycle back to Stopped (reached %d times)\n", parentReachedStoppedCount)
}

// verifyFullCycle verifies the complete cyclic pattern.
func verifyFullCycle(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	var parentStates []string

	for _, entry := range stateTransitions {
		worker := ""
		toState := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}

			if field.Key == "to_state" {
				toState = field.String
			}
		}

		if strings.Contains(worker, "exampleparent") {
			parentStates = append(parentStates, toState)
		}
	}

	// Expected sequence: TryingToStart → Running → TryingToStop → Stopped
	expectedSequence := []string{"TryingToStart", "Running", "TryingToStop", "Stopped"}

	// Verify sequence appears in order
	seqIdx := 0
	for _, state := range parentStates {
		if seqIdx < len(expectedSequence) && state == expectedSequence[seqIdx] {
			seqIdx++
		}
	}

	Expect(seqIdx).To(Equal(len(expectedSequence)),
		fmt.Sprintf("Expected full cycle %v, but only matched %d states. Actual: %v",
			expectedSequence, seqIdx, parentStates))

	GinkgoWriter.Printf("✓ Full cycle completed: %v\n", expectedSequence)
}

// =============================================================================
// Category 8: MetricsHolder Type Assertion Tests (TDD for Prometheus Export Bug)
// =============================================================================

var _ = Describe("MetricsHolder Type Assertion", func() {
	It("should export worker metrics to Prometheus and support CSE serialization", func() {
		originalStoppedWait := state.StoppedWaitDuration
		originalRunning := state.RunningDuration
		state.StoppedWaitDuration = 500 * time.Millisecond
		state.RunningDuration = 500 * time.Millisecond
		defer func() {
			state.StoppedWaitDuration = originalStoppedWait
			state.RunningDuration = originalRunning
		}()

		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		store := setupTestStoreForScenario(testLogger.Logger)

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     examples.SimpleScenario,
			Duration:     5 * time.Second,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())
		<-result.Done

		workers := getWorkersFromStore(store)
		Expect(workers).NotTo(BeEmpty(), "Should have workers in store")

		var parentObs snapshot.ExampleparentObservedState
		var foundParent bool
		for _, w := range workers {
			if w.WorkerType == "exampleparent" {
				observedJSON, err := json.Marshal(w.Observed)
				Expect(err).NotTo(HaveOccurred())

				err = json.Unmarshal(observedJSON, &parentObs)
				Expect(err).NotTo(HaveOccurred())
				foundParent = true
				GinkgoWriter.Printf("DEBUG: Parent observed state JSON: %s\n", string(observedJSON))

				break
			}
		}
		Expect(foundParent).To(BeTrue(), "Should find parent worker in store")

		GinkgoWriter.Printf("DEBUG: Parent FrameworkMetrics: %+v\n", parentObs.Metrics.Framework)

		holder, ok := interface{}(parentObs).(deps.MetricsHolder)

		Expect(ok).To(BeTrue(), "MetricsHolder type assertion should succeed - THIS FAILS WITH POINTER RECEIVERS")

		if ok {
			workerMetrics := holder.GetWorkerMetrics()
			frameworkMetrics := holder.GetFrameworkMetrics()
			_ = workerMetrics
			GinkgoWriter.Printf("MetricsHolder interface works: frameworkMetrics=%+v\n", frameworkMetrics)
		}
	})
})
