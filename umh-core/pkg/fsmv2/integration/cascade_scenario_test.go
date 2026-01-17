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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/integration"
)

var _ = Describe("Cascade Scenario Integration", func() {
	It("should demonstrate child failure propagation to parent state", func() {
		By("Setting up test logger at DebugLevel")
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		By("Setting up context with 30s timeout")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		By("Setting up triangular store")
		store := setupTestStoreForScenario(testLogger.Logger)

		By("Creating scenario context with 25s duration")
		// 25s allows time for two complete failure cycles:
		// Cycle 1 (startup):
		// - Parent starts and creates children
		// - Children wait for ParentMappedState before starting (slight delay)
		// - Children fail (3 times each) - Parent stays in TryingToStart (expected)
		// - Children recover - Parent goes to Running
		// Cycle 2 (runtime):
		// - Children disconnect (after 2 healthy ticks)
		// - Children fail (3 times each) - Parent goes to Degraded
		// - Children recover - Parent returns to Running
		// Note: 15s was not enough for two full cycles with 100ms tick interval.
		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 25*time.Second)
		defer scenarioCancel()

		By("Running CascadeScenario with 100ms tick interval")
		result, err := examples.Run(scenarioCtx, examples.RunConfig{
			Scenario:     examples.CascadeScenario,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for scenario completion")
		<-result.Done

		// =====================================================================
		// Cascade Scenario Verifications
		// =====================================================================

		By("Verifying parent worker was created")
		verifyCascadeParentCreated(testLogger)

		By("Verifying children were created by parent")
		verifyCascadeChildrenCreated(testLogger)

		By("Verifying children experienced failures")
		verifyCascadeChildrenFailed(testLogger)

		By("Verifying children eventually recovered")
		verifyCascadeChildrenRecovered(testLogger)

		By("Verifying parent state transitions through degraded state")
		verifyCascadeParentStateTransitions(testLogger)

		By("Verifying multiple children fail independently")
		verifyCascadeMultipleChildrenIndependent(testLogger)

		By("Verifying all children must recover for parent to be healthy")
		verifyCascadeAllChildrenMustRecover(testLogger)

		By("Verifying each child fails expected number of times")
		verifyCascadeChildFailureCounts(testLogger)
	})
})

// verifyCascadeParentCreated checks that the cascade parent was created.
func verifyCascadeParentCreated(t *integration.TestLogger) {
	createdLogs := t.GetLogsMatching("worker_created")

	parentCreated := false

	for _, entry := range createdLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "cascade-parent") {
			parentCreated = true

			break
		}
	}

	// Also check for state transitions as alternative indicator
	if !parentCreated {
		stateTransitions := t.GetLogsMatching("state_transition")
		for _, entry := range stateTransitions {
			worker := ""

			for _, field := range entry.Context {
				if field.Key == "worker" {
					worker = field.String
				}
			}

			if strings.Contains(worker, "cascade-parent") {
				parentCreated = true

				break
			}
		}
	}

	Expect(parentCreated).To(BeTrue(),
		"Expected cascade-parent worker to be created")

	GinkgoWriter.Printf("✓ Cascade parent worker created\n")
}

// verifyCascadeChildrenCreated checks that children were created by the parent.
func verifyCascadeChildrenCreated(t *integration.TestLogger) {
	// Look for child worker creation or state transitions
	stateTransitions := t.GetLogsMatching("state_transition")

	childrenFound := make(map[string]bool)

	for _, entry := range stateTransitions {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}
		// Children are named child-0, child-1 under cascade-parent
		if strings.Contains(worker, "child-0") || strings.Contains(worker, "child-1") {
			childrenFound[worker] = true
		}
	}

	// Also check failure logs as indicator of child activity
	failureLogs := t.GetLogsMatching("connect_failed_simulated")
	for _, entry := range failureLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "child-") {
			childrenFound[worker] = true
		}
	}

	Expect(childrenFound).ToNot(BeEmpty(),
		fmt.Sprintf("Expected at least one child worker to be created, found: %v", childrenFound))

	GinkgoWriter.Printf("✓ Cascade children created: %d children\n", len(childrenFound))
}

// verifyCascadeChildrenFailed checks that children experienced failures.
func verifyCascadeChildrenFailed(t *integration.TestLogger) {
	failureLogs := t.GetLogsMatching("connect_failed_simulated")

	childFailures := 0

	for _, entry := range failureLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "child-") {
			childFailures++
		}
	}

	// Children should fail at least once to verify failure behavior
	// Note: The exact failure count depends on timing - children wait for
	// ParentMappedState before starting, which affects how many failures
	// occur within the test duration. The important thing is that failures DO happen.
	Expect(childFailures).To(BeNumerically(">=", 1),
		fmt.Sprintf("Expected children to fail at least once, got %d", childFailures))

	GinkgoWriter.Printf("✓ Children experienced failures: %d total failures\n", childFailures)
}

// verifyCascadeChildrenRecovered checks that children eventually recovered.
func verifyCascadeChildrenRecovered(t *integration.TestLogger) {
	recoveryLogs := t.GetLogsMatching("connect_succeeded_after_failures")

	childRecoveries := 0

	for _, entry := range recoveryLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "child-") {
			childRecoveries++
		}
	}

	Expect(childRecoveries).To(BeNumerically(">=", 1),
		fmt.Sprintf("Expected at least one child to recover, got %d", childRecoveries))

	GinkgoWriter.Printf("✓ Children recovered: %d children\n", childRecoveries)
}

// verifyCascadeParentStateTransitions checks parent state transitions through degraded state.
func verifyCascadeParentStateTransitions(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	parentToHealthy := false
	parentToDegraded := false
	parentRecoveredToHealthy := false

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

		if strings.Contains(worker, "cascade-parent") {
			if strings.Contains(toState, "healthy") || toState == "Running" {
				if parentToDegraded {
					parentRecoveredToHealthy = true
				} else {
					parentToHealthy = true
				}
			}

			if strings.Contains(toState, "degraded") || strings.Contains(toState, "Degraded") {
				parentToDegraded = true
			}
		}
	}

	// Assert parent transitions through degraded state
	// This verifies the cascade behavior: when children become unhealthy,
	// parent should detect it and transition to Degraded state.
	Expect(parentToDegraded).To(BeTrue(),
		"Expected parent to transition to Degraded when children fail")
	Expect(parentRecoveredToHealthy).To(BeTrue(),
		"Expected parent to recover to Running after children heal")

	GinkgoWriter.Printf("✓ Parent state transitions verified: healthy=%v, degraded=%v, recovered=%v\n",
		parentToHealthy, parentToDegraded, parentRecoveredToHealthy)
}

// verifyCascadeMultipleChildrenIndependent checks that multiple children fail independently.
func verifyCascadeMultipleChildrenIndependent(t *integration.TestLogger) {
	failureLogs := t.GetLogsMatching("connect_failed_simulated")

	child0Failures := 0
	child1Failures := 0

	for _, entry := range failureLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "child-0") {
			child0Failures++
		}

		if strings.Contains(worker, "child-1") {
			child1Failures++
		}
	}

	// Both children should fail independently
	Expect(child0Failures).To(BeNumerically(">=", 1),
		"Expected child-0 to experience failures")
	Expect(child1Failures).To(BeNumerically(">=", 1),
		"Expected child-1 to experience failures")

	GinkgoWriter.Printf("✓ Children failed independently: child-0=%d, child-1=%d failures\n",
		child0Failures, child1Failures)
}

// verifyCascadeAllChildrenMustRecover checks that all children must recover for parent to be healthy.
func verifyCascadeAllChildrenMustRecover(t *integration.TestLogger) {
	recoveryLogs := t.GetLogsMatching("connect_succeeded_after_failures")

	child0Recovered := false
	child1Recovered := false

	for _, entry := range recoveryLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "child-0") {
			child0Recovered = true
		}

		if strings.Contains(worker, "child-1") {
			child1Recovered = true
		}
	}

	// Both children must recover for cascade test to complete successfully
	Expect(child0Recovered).To(BeTrue(),
		"Expected child-0 to recover")
	Expect(child1Recovered).To(BeTrue(),
		"Expected child-1 to recover")

	GinkgoWriter.Printf("✓ All children recovered: child-0=%v, child-1=%v\n",
		child0Recovered, child1Recovered)
}

// verifyCascadeChildFailureCounts checks that each child fails expected number of times.
func verifyCascadeChildFailureCounts(t *integration.TestLogger) {
	failureLogs := t.GetLogsMatching("connect_failed_simulated")

	childFailures := make(map[string]int)

	for _, entry := range failureLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "child-") {
			childFailures[worker]++
		}
	}

	// Each child should fail exactly 6 times (max_failures: 3 * failure_cycles: 2 = 6)
	for child, count := range childFailures {
		Expect(count).To(Equal(6),
			fmt.Sprintf("Expected %s to fail exactly 6 times (3 per cycle * 2 cycles), got %d", child, count))
	}

	GinkgoWriter.Printf("✓ Child failure counts: %v\n", childFailures)
}
