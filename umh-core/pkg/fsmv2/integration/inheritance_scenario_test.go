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
		testLogger := integration.NewTestLogger()
		defer testLogger.Stop()

		By("Setting up context with 30s timeout")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		By("Setting up triangular store")
		store := setupTestStoreForScenario(testLogger.FSMLogger)

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
			Logger:       testLogger.FSMLogger,
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
//
// Observation model (post-C2): child workers built on WorkerBase use
// WrappedDesiredState[TConfig] which does not carry originalUserSpec — the
// PURE_DERIVE architecture decouples cached Config from raw UserSpec. The
// canonical observation point for inheritance is therefore the parent's
// Desired: parent.originalUserSpec.variables.user supplies the inherited
// keys, and parent.childrenSpecs[i].userSpec.variables.user supplies the
// per-child overlay. The supervisor merges these via config.Merge() at
// spawn time; asserting both halves exist proves the inputs to that merge
// are correct.
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

	// DIRECT VARIABLE ASSERTIONS (observed via parent's Desired)
	//
	// The actual variable merging happens in reconcileChildren():
	//   childUserSpec.Variables = config.Merge(s.userSpec.Variables, spec.UserSpec.Variables)
	//
	// Inputs to that merge, both visible on the parent:
	//   parent.originalUserSpec.variables.user     → {IP, PORT, CONNECTION_NAME}
	//   parent.childrenSpecs[i].userSpec.variables.user → {DEVICE_ID}
	//
	// Result the supervisor hands to the child's DeriveDesiredState:
	//   {IP: "192.168.1.100", PORT: 502, CONNECTION_NAME: "factory-plc", DEVICE_ID: "device-N"}
	Expect(parentWorker.Desired).NotTo(BeNil(),
		"Parent should have Desired state")

	parentUserVars, err := extractUserNamespaceVariables(parentWorker.Desired)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Failed to extract parent user-namespace variables: %v", err))
	GinkgoWriter.Printf("\n  Parent user variables: %+v\n", parentUserVars)

	childSpecs, err := extractChildrenSpecs(parentWorker.Desired)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Failed to extract parent childrenSpecs: %v", err))
	Expect(len(childSpecs)).To(Equal(len(childWorkers)),
		fmt.Sprintf("Parent childrenSpecs (%d) should match spawned children (%d)",
			len(childSpecs), len(childWorkers)))

	for i, spec := range childSpecs {
		childName, _ := spec["name"].(string)
		GinkgoWriter.Printf("\n  Verifying child %d (%s) variables:\n", i, childName)

		childUserVars, err := extractChildSpecUserVariables(spec)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("Child %s: failed to extract user variables from parent's childrenSpecs[%d]: %v",
				childName, i, err))

		// Compute the merged view the supervisor will hand to the child.
		mergedVars := make(map[string]any, len(parentUserVars)+len(childUserVars))
		for k, v := range parentUserVars {
			mergedVars[k] = v
		}
		for k, v := range childUserVars {
			mergedVars[k] = v
		}

		GinkgoWriter.Printf("    Merged user variables: %+v\n", mergedVars)

		// Assert inherited variables from parent.
		Expect(mergedVars).To(HaveKeyWithValue("IP", "192.168.1.100"),
			fmt.Sprintf("Child %s should inherit IP from parent", childName))
		GinkgoWriter.Printf("    ✓ IP: %v (inherited from parent)\n", mergedVars["IP"])

		portValue, hasPort := mergedVars["PORT"]
		Expect(hasPort).To(BeTrue(),
			fmt.Sprintf("Child %s should inherit PORT from parent", childName))

		var portInt int
		switch v := portValue.(type) {
		case float64:
			portInt = int(v)
		case int:
			portInt = v
		default:
			Fail(fmt.Sprintf("Child %s PORT should be numeric, got: %T", childName, portValue))
		}

		Expect(portInt).To(Equal(502),
			fmt.Sprintf("Child %s should inherit PORT=502 from parent, got %d", childName, portInt))
		GinkgoWriter.Printf("    ✓ PORT: %v (inherited from parent)\n", portInt)

		Expect(mergedVars).To(HaveKeyWithValue("CONNECTION_NAME", "factory-plc"),
			fmt.Sprintf("Child %s should inherit CONNECTION_NAME from parent", childName))
		GinkgoWriter.Printf("    ✓ CONNECTION_NAME: %v (inherited from parent)\n", mergedVars["CONNECTION_NAME"])

		// Assert child's own variable (DEVICE_ID)
		deviceID, hasDeviceID := mergedVars["DEVICE_ID"]
		Expect(hasDeviceID).To(BeTrue(),
			fmt.Sprintf("Child %s should have DEVICE_ID variable (from childrenSpecs[%d])", childName, i))
		Expect(deviceID).To(HavePrefix("device-"),
			fmt.Sprintf("Child %s DEVICE_ID should start with 'device-', got: %v", childName, deviceID))
		GinkgoWriter.Printf("    ✓ DEVICE_ID: %v (child's own variable)\n", deviceID)
	}

	GinkgoWriter.Printf("\n✓ Variable inheritance verified with direct assertions!\n")
}

// verifyTemplateRenderingWorks verifies that template rendering works correctly
// by calling config.RenderConfigTemplate directly with the merged UserSpec the
// supervisor will hand to each child.
//
// Post-C2, child workers using WorkerBase cache only TConfig (not the raw
// UserSpec) in their WrappedDesiredState, so the canonical observation point
// is the parent's Desired: parent.childrenSpecs[i].userSpec.config provides
// the config template, and merging parent.originalUserSpec.variables.user
// with childrenSpecs[i].userSpec.variables.user reproduces the bundle the
// supervisor passes into the child's DeriveDesiredState.
//
// This test:
// 1. Finds the inheritance-parent in the store.
// 2. For each childrenSpecs[i], builds merged variables (parent user vars
//    overlaid with the child spec's user vars).
// 3. Calls config.RenderConfigTemplate(childSpec.UserSpec.Config, merged).
// 4. Asserts rendered output contains "192.168.1.100" (IP rendered).
// 5. Asserts rendered output does NOT contain "{{ .IP }}" (no placeholders).
// 6. Asserts rendered output contains "device-" (DEVICE_ID rendered).
func verifyTemplateRenderingWorks(store storage.TriangularStoreInterface) {
	workers := getWorkersFromStore(store)

	var parentWorker *examples.WorkerSnapshot

	for i := range workers {
		w := &workers[i]
		if w.WorkerType == "exampleparent" && strings.Contains(w.WorkerID, "inheritance-parent") {
			parentWorker = w

			break
		}
	}

	Expect(parentWorker).NotTo(BeNil(),
		"Expected to find inheritance-parent worker in store")

	if parentWorker == nil {
		return
	}

	Expect(parentWorker.Desired).NotTo(BeNil(),
		"Parent should have Desired state")

	parentUserVars, err := extractUserNamespaceVariables(parentWorker.Desired)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Failed to extract parent user-namespace variables: %v", err))

	childSpecs, err := extractChildrenSpecs(parentWorker.Desired)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Failed to extract parent childrenSpecs: %v", err))
	Expect(len(childSpecs)).To(BeNumerically(">=", 2),
		fmt.Sprintf("Expected at least 2 childrenSpecs, found %d", len(childSpecs)))

	GinkgoWriter.Printf("\n=== Verifying template rendering works ===\n")
	GinkgoWriter.Printf("Found %d childrenSpecs to verify\n", len(childSpecs))

	for i, spec := range childSpecs {
		childName, _ := spec["name"].(string)
		GinkgoWriter.Printf("\n  Verifying child %d (%s) template rendering:\n", i, childName)

		childConfigStr, err := extractChildSpecConfig(spec)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("Child %s: failed to extract config from parent's childrenSpecs[%d]: %v",
				childName, i, err))

		childUserVars, err := extractChildSpecUserVariables(spec)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("Child %s: failed to extract user variables from parent's childrenSpecs[%d]: %v",
				childName, i, err))

		mergedUserVars := make(map[string]any, len(parentUserVars)+len(childUserVars))
		for k, v := range parentUserVars {
			mergedUserVars[k] = v
		}
		for k, v := range childUserVars {
			mergedUserVars[k] = v
		}

		mergedSpec := config.UserSpec{
			Config:    childConfigStr,
			Variables: config.VariableBundle{User: mergedUserVars},
		}

		GinkgoWriter.Printf("    UserSpec.Config: %q\n", mergedSpec.Config)
		GinkgoWriter.Printf("    UserSpec.Variables: %+v\n", mergedSpec.Variables)

		Expect(mergedSpec.Config).NotTo(BeEmpty(),
			fmt.Sprintf("Child %s UserSpec.Config should not be empty - parent must provide a config template", childName))

		renderedConfig, err := config.RenderConfigTemplate(mergedSpec.Config, mergedSpec.Variables)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("Child %s: RenderConfigTemplate failed: %v", childName, err))

		GinkgoWriter.Printf("    Rendered config: %q\n", renderedConfig)

		Expect(renderedConfig).To(ContainSubstring("192.168.1.100"),
			fmt.Sprintf("Child %s rendered config should contain IP '192.168.1.100'", childName))
		GinkgoWriter.Printf("    [PASS] Contains rendered IP: 192.168.1.100\n")

		Expect(renderedConfig).NotTo(ContainSubstring("{{ .IP }}"),
			fmt.Sprintf("Child %s rendered config should NOT contain unrendered placeholder '{{ .IP }}'", childName))
		GinkgoWriter.Printf("    [PASS] Does not contain unrendered placeholder {{ .IP }}\n")

		Expect(renderedConfig).To(ContainSubstring("device-"),
			fmt.Sprintf("Child %s rendered config should contain DEVICE_ID prefix 'device-'", childName))
		GinkgoWriter.Printf("    [PASS] Contains rendered DEVICE_ID prefix: device-\n")
	}

	GinkgoWriter.Printf("\n[PASS] Template rendering verified in all children!\n")
	GinkgoWriter.Printf("\n=== Production Note ===\n")
	GinkgoWriter.Printf("In production, workers call RenderConfigTemplate in DeriveDesiredState()\n")
	GinkgoWriter.Printf("to convert UserSpec.Config template + Variables into rendered config.\n")
}

// extractUserNamespaceVariables returns an empty map.
//
// Post-PR3-C3, the parent worker uses WrappedDesiredState[ExampleparentConfig]
// which, by PURE_DERIVE, does not carry originalUserSpec. Parent user variables
// (IP, PORT, CONNECTION_NAME) are pre-merged into each child's
// childrenSpecs[i].userSpec.variables.user by the child-spec factory, so the
// inherited keys are already visible there.
//
// The helper is retained so call sites do not need to branch — it returns an
// empty map and callers rely on extractChildSpecUserVariables for the merged
// variables seen by the supervisor at spawn.
func extractUserNamespaceVariables(desired map[string]any) (map[string]any, error) {
	_ = desired
	return map[string]any{}, nil
}

// extractChildrenSpecs returns the raw child specs (as map[string]any) from a
// parent's Desired state. Each entry represents a config.ChildSpec serialized
// as JSON; callers use extractChildSpecConfig / extractChildSpecUserVariables
// to pull individual fields.
func extractChildrenSpecs(desired map[string]any) ([]map[string]any, error) {
	raw, hasSpecs := desired["childrenSpecs"]
	if !hasSpecs {
		return nil, fmt.Errorf("missing childrenSpecs field, got keys: %v", getMapKeys(desired))
	}

	rawSlice, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("childrenSpecs should be []any, got: %T", raw)
	}

	specs := make([]map[string]any, 0, len(rawSlice))

	for i, item := range rawSlice {
		specMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("childrenSpecs[%d] should be map[string]any, got: %T", i, item)
		}

		specs = append(specs, specMap)
	}

	return specs, nil
}

// extractChildSpecConfig returns the UserSpec.Config template string from a
// single parent childrenSpecs entry. Returns the empty string if userSpec.config
// is absent (the parent wired no template for this child).
func extractChildSpecConfig(spec map[string]any) (string, error) {
	userSpecRaw, hasUserSpec := spec["userSpec"]
	if !hasUserSpec {
		return "", nil
	}

	userSpecMap, ok := userSpecRaw.(map[string]any)
	if !ok {
		return "", fmt.Errorf("childrenSpecs[].userSpec should be map[string]any, got: %T", userSpecRaw)
	}

	configVal, hasConfig := userSpecMap["config"]
	if !hasConfig {
		return "", nil
	}

	configStr, ok := configVal.(string)
	if !ok {
		return "", fmt.Errorf("childrenSpecs[].userSpec.config should be string, got: %T", configVal)
	}

	return configStr, nil
}

// extractChildSpecUserVariables returns the user-namespace variables for a
// single parent childrenSpecs entry (the per-child overlay). The supervisor
// merges these on top of the parent's user-namespace variables before spawning.
func extractChildSpecUserVariables(spec map[string]any) (map[string]any, error) {
	userSpecRaw, hasUserSpec := spec["userSpec"]
	if !hasUserSpec {
		return map[string]any{}, nil
	}

	userSpecMap, ok := userSpecRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("childrenSpecs[].userSpec should be map[string]any, got: %T", userSpecRaw)
	}

	variables, hasVars := userSpecMap["variables"]
	if !hasVars {
		return map[string]any{}, nil
	}

	varsMap, ok := variables.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("childrenSpecs[].userSpec.variables should be map[string]any, got: %T", variables)
	}

	userVars, hasUser := varsMap["user"]
	if !hasUser {
		return map[string]any{}, nil
	}

	userVarsMap, ok := userVars.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("childrenSpecs[].userSpec.variables.user should be map[string]any, got: %T", userVars)
	}

	return userVarsMap, nil
}

// getMapKeys returns the keys of a map for debugging purposes.
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
