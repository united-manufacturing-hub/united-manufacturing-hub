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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/integration"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

var _ = Describe("Inheritance Scenario Integration", Serial, func() {
	// Serial: This test modifies package-level variables (state.StoppedWaitDuration, state.RunningDuration)
	// to speed up test execution. Serial ensures no parallel test interference.
	It("should demonstrate variable inheritance from parent to children", func() {
		By("Setting short durations for fast testing")
		// Override durations for fast test execution
		originalStoppedWait := state.StoppedWaitDuration
		originalRunning := state.RunningDuration
		state.StoppedWaitDuration = 500 * time.Millisecond
		state.RunningDuration = 1 * time.Second
		defer func() {
			state.StoppedWaitDuration = originalStoppedWait
			state.RunningDuration = originalRunning
		}()

		By("Setting up test logger at DebugLevel")
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		By("Setting up context with 30s timeout")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		By("Setting up triangular store")
		store := setupTestStoreForScenario(testLogger.Logger)

		By("Creating scenario context with 10s duration")
		// 10s is enough to see:
		// - Parent starts with IP, PORT, CONNECTION_NAME variables
		// - Children are created with DEVICE_ID variable
		// - Variables merge: children get IP, PORT, CONNECTION_NAME from parent + DEVICE_ID
		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 10*time.Second)
		defer scenarioCancel()

		By("Running InheritanceScenario with 100ms tick interval")
		result, err := examples.Run(scenarioCtx, examples.RunConfig{
			Scenario:     examples.InheritanceScenario,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for scenario completion")
		<-result.Done

		// =====================================================================
		// Inheritance Scenario Verifications
		// =====================================================================

		By("Verifying parent worker was created with variables")
		verifyInheritanceParentCreated(testLogger)

		By("Verifying children were created")
		verifyInheritanceChildrenCreated(testLogger)

		By("Verifying variable propagation logs show inheritance")
		verifyVariablePropagation(testLogger)

		By("Verifying parent variables include IP and PORT")
		verifyParentHasVariables(testLogger)

		By("Verifying children received merged variables (store-based)")
		verifyChildrenReceivedMergedVariablesFromStore(store)

		By("Verifying template rendering works with merged variables")
		verifyTemplateRenderingWorks(store)
	})
})

// verifyInheritanceParentCreated checks that the inheritance parent was created.
func verifyInheritanceParentCreated(t *integration.TestLogger) {
	// Look for worker creation or state transitions as indicator
	createdLogs := t.GetLogsMatching("worker_created")
	stateTransitions := t.GetLogsMatching("state_transition")

	parentCreated := false

	for _, entry := range createdLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "inheritance-parent") {
			parentCreated = true

			break
		}
	}

	// Also check state transitions as alternative indicator
	if !parentCreated {
		for _, entry := range stateTransitions {
			worker := ""

			for _, field := range entry.Context {
				if field.Key == "worker" {
					worker = field.String
				}
			}

			if strings.Contains(worker, "inheritance-parent") {
				parentCreated = true

				break
			}
		}
	}

	Expect(parentCreated).To(BeTrue(),
		"Expected inheritance-parent worker to be created")

	GinkgoWriter.Printf("✓ Inheritance parent worker created\n")
}

// verifyInheritanceChildrenCreated checks that children were created.
func verifyInheritanceChildrenCreated(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")
	childAddingLogs := t.GetLogsMatching("child_adding")

	childrenFound := make(map[string]bool)

	// Check state transitions for child workers
	for _, entry := range stateTransitions {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "child-0") || strings.Contains(worker, "child-1") {
			childrenFound[worker] = true
		}
	}

	// Also check child_adding logs
	for _, entry := range childAddingLogs {
		childName := ""

		for _, field := range entry.Context {
			if field.Key == "child_name" {
				childName = field.String
			}
		}

		if childName != "" {
			childrenFound[childName] = true
		}
	}

	Expect(len(childrenFound)).To(BeNumerically(">=", 2),
		fmt.Sprintf("Expected at least 2 children to be created, found: %v", childrenFound))

	GinkgoWriter.Printf("✓ Inheritance children created: %d children\n", len(childrenFound))
}

// verifyVariablePropagation checks that variable propagation occurred.
// Note: variables_propagated logs are at TRACE level, so we verify indirectly
// by checking that children were created and have proper states (which requires
// variable propagation to work).
func verifyVariablePropagation(t *integration.TestLogger) {
	// The variables_propagated log is at TRACE level (logTrace function),
	// so it won't appear in DEBUG logs. Instead, we verify propagation occurred
	// by checking that:
	// 1. Children were created (requires parent config to be processed)
	// 2. Children have valid state transitions (requires variables to be available)

	// Check for child state transitions as indirect proof of variable propagation
	stateTransitions := t.GetLogsMatching("state_transition")

	childTransitions := 0

	for _, entry := range stateTransitions {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "examplechild") {
			childTransitions++
		}
	}

	Expect(childTransitions).To(BeNumerically(">=", 1),
		"Expected at least 1 child state transition (proves variables were propagated)")

	GinkgoWriter.Printf("✓ Variable propagation verified via %d child state transitions\n", childTransitions)
}

// verifyParentHasVariables checks parent configuration includes expected variables.
func verifyParentHasVariables(t *integration.TestLogger) {
	// The parent variables should be in the YAML config
	// We verify by checking the scenario was run (parent created and children have parent's variables)
	// The actual variable values are verified in verifyChildrenReceivedMergedVariables
	stateTransitions := t.GetLogsMatching("state_transition")

	parentReachedRunning := false

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

		if strings.Contains(worker, "inheritance-parent") && toState == "Running" {
			parentReachedRunning = true

			break
		}
	}

	Expect(parentReachedRunning).To(BeTrue(),
		"Expected inheritance-parent to reach Running state (indicates variables were processed)")

	GinkgoWriter.Printf("✓ Parent has variables and reached Running state\n")
}

// verifyChildrenReceivedMergedVariablesFromStore verifies children received merged variables
// by examining the store. This is the key test for variable inheritance.
//
// Expected flow:
// 1. Parent config defines: IP="192.168.1.100", PORT=502, CONNECTION_NAME="factory-plc".
// 2. Parent's GetChildSpecs() returns children with DEVICE_ID="device-N".
// 3. Supervisor calls config.Merge() to combine parent + child variables.
// 4. Child ends up with: IP, PORT, CONNECTION_NAME (inherited) + DEVICE_ID (own).
func verifyChildrenReceivedMergedVariablesFromStore(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	// Find parent and children
	var parentWorker *examples.WorkerSnapshot

	var childWorkers []examples.WorkerSnapshot

	for i := range workers {
		w := &workers[i]
		if w.WorkerType == "exampleparent" && strings.Contains(w.WorkerID, "inheritance-parent") {
			parentWorker = w
		} else if w.WorkerType == "examplechild" {
			childWorkers = append(childWorkers, *w)
		}
	}

	Expect(parentWorker).NotTo(BeNil(),
		"Expected to find inheritance-parent worker in store")
	Expect(len(childWorkers)).To(BeNumerically(">=", 2),
		fmt.Sprintf("Expected at least 2 child workers, found %d", len(childWorkers)))

	// Explicit nil check for nilaway (it doesn't recognize Ginkgo's Expect assertions)
	if parentWorker == nil {
		return
	}

	GinkgoWriter.Printf("✓ Found parent worker: %s/%s\n", parentWorker.WorkerType, parentWorker.WorkerID)
	GinkgoWriter.Printf("✓ Found %d child workers\n", len(childWorkers))

	// Verify children have valid states (indicating they were properly configured)
	for _, child := range childWorkers {
		if child.Observed == nil {
			continue
		}

		state, hasState := child.Observed["state"].(string)
		Expect(hasState).To(BeTrue(),
			fmt.Sprintf("Child %s should have state in observed", child.WorkerID))
		Expect(state).NotTo(BeEmpty(),
			fmt.Sprintf("Child %s state should not be empty", child.WorkerID))

		GinkgoWriter.Printf("  Child %s state: %s\n", child.WorkerID, state)
	}

	// DIRECT VARIABLE ASSERTIONS
	// Verify the actual merged variables in the store
	//
	// The actual variable merging happens in reconcileChildren():
	//   childUserSpec.Variables = config.Merge(s.userSpec.Variables, spec.UserSpec.Variables)
	//
	// This merges:
	//   Parent: {IP: "192.168.1.100", PORT: 502, CONNECTION_NAME: "factory-plc"}
	//   Child:  {DEVICE_ID: "device-0"}
	//   Result: {IP: "192.168.1.100", PORT: 502, CONNECTION_NAME: "factory-plc", DEVICE_ID: "device-0"}

	for i, child := range childWorkers {
		GinkgoWriter.Printf("\n  Verifying child %d (%s) variables:\n", i, child.WorkerID)

		// Navigate the Desired document to extract variables
		// Path: Desired["originalUserSpec"]["variables"]["user"]["<var_name>"]
		Expect(child.Desired).NotTo(BeNil(),
			fmt.Sprintf("Child %s should have Desired state", child.WorkerID))

		// Get originalUserSpec from Desired
		originalUserSpec, hasSpec := child.Desired["originalUserSpec"]
		Expect(hasSpec).To(BeTrue(),
			fmt.Sprintf("Child %s Desired should have originalUserSpec, got: %+v", child.WorkerID, child.Desired))

		userSpecMap, ok := originalUserSpec.(map[string]any)
		Expect(ok).To(BeTrue(),
			fmt.Sprintf("Child %s originalUserSpec should be map[string]any, got: %T", child.WorkerID, originalUserSpec))

		// Get variables from userSpec
		variables, hasVars := userSpecMap["variables"]
		Expect(hasVars).To(BeTrue(),
			fmt.Sprintf("Child %s userSpec should have variables, got: %+v", child.WorkerID, userSpecMap))

		varsMap, ok := variables.(map[string]any)
		Expect(ok).To(BeTrue(),
			fmt.Sprintf("Child %s variables should be map[string]any, got: %T", child.WorkerID, variables))

		// Get user namespace from variables
		userVars, hasUser := varsMap["user"]
		Expect(hasUser).To(BeTrue(),
			fmt.Sprintf("Child %s variables should have user namespace, got: %+v", child.WorkerID, varsMap))

		userVarsMap, ok := userVars.(map[string]any)
		Expect(ok).To(BeTrue(),
			fmt.Sprintf("Child %s user variables should be map[string]any, got: %T", child.WorkerID, userVars))

		GinkgoWriter.Printf("    User variables: %+v\n", userVarsMap)

		// Assert inherited variables from parent
		Expect(userVarsMap).To(HaveKeyWithValue("IP", "192.168.1.100"),
			fmt.Sprintf("Child %s should inherit IP from parent", child.WorkerID))
		GinkgoWriter.Printf("    ✓ IP: %v (inherited from parent)\n", userVarsMap["IP"])

		// PORT is stored as float64 in JSON unmarshalling
		portValue, hasPort := userVarsMap["PORT"]
		Expect(hasPort).To(BeTrue(),
			fmt.Sprintf("Child %s should inherit PORT from parent", child.WorkerID))
		// Handle both int and float64 (JSON unmarshalling converts to float64)
		var portInt int
		switch v := portValue.(type) {
		case float64:
			portInt = int(v)
		case int:
			portInt = v
		default:
			Fail(fmt.Sprintf("Child %s PORT should be numeric, got: %T", child.WorkerID, portValue))
		}

		Expect(portInt).To(Equal(502),
			fmt.Sprintf("Child %s should inherit PORT=502 from parent, got %d", child.WorkerID, portInt))
		GinkgoWriter.Printf("    ✓ PORT: %v (inherited from parent)\n", portInt)

		Expect(userVarsMap).To(HaveKeyWithValue("CONNECTION_NAME", "factory-plc"),
			fmt.Sprintf("Child %s should inherit CONNECTION_NAME from parent", child.WorkerID))
		GinkgoWriter.Printf("    ✓ CONNECTION_NAME: %v (inherited from parent)\n", userVarsMap["CONNECTION_NAME"])

		// Assert child's own variable (DEVICE_ID)
		deviceID, hasDeviceID := userVarsMap["DEVICE_ID"]
		Expect(hasDeviceID).To(BeTrue(),
			fmt.Sprintf("Child %s should have DEVICE_ID variable", child.WorkerID))
		Expect(deviceID).To(HavePrefix("device-"),
			fmt.Sprintf("Child %s DEVICE_ID should start with 'device-', got: %v", child.WorkerID, deviceID))
		GinkgoWriter.Printf("    ✓ DEVICE_ID: %v (child's own variable)\n", deviceID)
	}

	GinkgoWriter.Printf("\n✓ Variable inheritance verified with direct assertions!\n")
}

// verifyTemplateRenderingWorks verifies that template rendering works correctly
// by calling config.RenderConfigTemplate directly with the child's UserSpec.
//
// This test:
// 1. Gets children from store
// 2. Extracts UserSpec from child.Desired["originalUserSpec"]
// 3. Calls config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
// 4. Asserts rendered output contains "192.168.1.100" (IP rendered)
// 5. Asserts rendered output does NOT contain "{{ .IP }}" (no placeholders)
// 6. Asserts rendered output contains "device-" (DEVICE_ID rendered)
//
// This test is expected to FAIL in the RED phase because the child's UserSpec.Config
// is currently empty (parent doesn't pass a config template yet).
func verifyTemplateRenderingWorks(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	// Find child workers
	var childWorkers []examples.WorkerSnapshot

	for i := range workers {
		w := &workers[i]
		if w.WorkerType == "examplechild" {
			childWorkers = append(childWorkers, *w)
		}
	}

	Expect(len(childWorkers)).To(BeNumerically(">=", 2),
		fmt.Sprintf("Expected at least 2 child workers, found %d", len(childWorkers)))

	GinkgoWriter.Printf("\n=== Verifying template rendering works ===\n")
	GinkgoWriter.Printf("Found %d child workers to verify\n", len(childWorkers))

	for i, child := range childWorkers {
		GinkgoWriter.Printf("\n  Verifying child %d (%s) template rendering:\n", i, child.WorkerID)

		// Child should have Desired state
		Expect(child.Desired).NotTo(BeNil(),
			fmt.Sprintf("Child %s should have Desired state", child.WorkerID))

		// Extract UserSpec from originalUserSpec
		userSpec, err := extractUserSpec(child.Desired)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("Child %s: failed to extract UserSpec: %v", child.WorkerID, err))

		GinkgoWriter.Printf("    UserSpec.Config: %q\n", userSpec.Config)
		GinkgoWriter.Printf("    UserSpec.Variables: %+v\n", userSpec.Variables)

		// Config should not be empty (this is expected to FAIL in RED phase)
		Expect(userSpec.Config).NotTo(BeEmpty(),
			fmt.Sprintf("Child %s UserSpec.Config should not be empty - parent must provide a config template", child.WorkerID))

		// Call RenderConfigTemplate directly
		renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("Child %s: RenderConfigTemplate failed: %v", child.WorkerID, err))

		GinkgoWriter.Printf("    Rendered config: %q\n", renderedConfig)

		// Assert rendered output contains IP (192.168.1.100 from parent variables)
		Expect(renderedConfig).To(ContainSubstring("192.168.1.100"),
			fmt.Sprintf("Child %s rendered config should contain IP '192.168.1.100'", child.WorkerID))
		GinkgoWriter.Printf("    [PASS] Contains rendered IP: 192.168.1.100\n")

		// Assert rendered output does NOT contain unrendered placeholders
		Expect(renderedConfig).NotTo(ContainSubstring("{{ .IP }}"),
			fmt.Sprintf("Child %s rendered config should NOT contain unrendered placeholder '{{ .IP }}'", child.WorkerID))
		GinkgoWriter.Printf("    [PASS] Does not contain unrendered placeholder {{ .IP }}\n")

		// Assert rendered output contains DEVICE_ID prefix (device-0, device-1, etc.)
		Expect(renderedConfig).To(ContainSubstring("device-"),
			fmt.Sprintf("Child %s rendered config should contain DEVICE_ID prefix 'device-'", child.WorkerID))
		GinkgoWriter.Printf("    [PASS] Contains rendered DEVICE_ID prefix: device-\n")
	}

	GinkgoWriter.Printf("\n[PASS] Template rendering verified in all children!\n")
	GinkgoWriter.Printf("\n=== Production Note ===\n")
	GinkgoWriter.Printf("In production, workers call RenderConfigTemplate in DeriveDesiredState()\n")
	GinkgoWriter.Printf("to convert UserSpec.Config template + Variables into rendered config.\n")
}

// extractUserSpec extracts a config.UserSpec from the originalUserSpec field in Desired state.
// Returns error if the field is missing or has unexpected structure.
func extractUserSpec(desired map[string]any) (config.UserSpec, error) {
	// Get originalUserSpec from Desired
	originalUserSpec, hasSpec := desired["originalUserSpec"]
	if !hasSpec {
		return config.UserSpec{}, fmt.Errorf("missing originalUserSpec field, got keys: %v", getMapKeys(desired))
	}

	userSpecMap, ok := originalUserSpec.(map[string]any)
	if !ok {
		return config.UserSpec{}, fmt.Errorf("originalUserSpec should be map[string]any, got: %T", originalUserSpec)
	}

	// Extract config string
	configStr := ""

	if configVal, hasConfig := userSpecMap["config"]; hasConfig {
		if s, ok := configVal.(string); ok {
			configStr = s
		}
	}

	// Extract variables
	var variables config.VariableBundle

	if varsVal, hasVars := userSpecMap["variables"]; hasVars {
		if varsMap, ok := varsVal.(map[string]any); ok {
			// Extract user namespace
			if userVars, hasUser := varsMap["user"]; hasUser {
				if userVarsMap, ok := userVars.(map[string]any); ok {
					variables.User = userVarsMap
				}
			}
			// Extract global namespace
			if globalVars, hasGlobal := varsMap["global"]; hasGlobal {
				if globalVarsMap, ok := globalVars.(map[string]any); ok {
					variables.Global = globalVarsMap
				}
			}
			// Extract internal namespace
			if internalVars, hasInternal := varsMap["internal"]; hasInternal {
				if internalVarsMap, ok := internalVars.(map[string]any); ok {
					variables.Internal = internalVarsMap
				}
			}
		}
	}

	return config.UserSpec{
		Config:    configStr,
		Variables: variables,
	}, nil
}

// getMapKeys returns the keys of a map for debugging purposes.
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
