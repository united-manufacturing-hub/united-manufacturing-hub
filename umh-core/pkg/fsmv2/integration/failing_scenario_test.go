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

var _ = Describe("Failing Scenario Integration", func() {
	It("should demonstrate recovery vs permanent failure patterns", func() {
		By("Setting up test logger at DebugLevel")
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		By("Setting up context with 30s timeout")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		By("Setting up triangular store")
		store := setupTestStoreForScenario(testLogger.Logger)

		By("Creating scenario context with 8s duration")
		// 8s is enough to see:
		// - Recovery worker: 3 failures + 1 success (~4s with 1s tick)
		// - Permanent worker: several failures showing it stays stuck
		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 8*time.Second)
		defer scenarioCancel()

		By("Running FailingScenario with 100ms tick interval")
		result, err := examples.Run(scenarioCtx, examples.RunConfig{
			Scenario:     examples.FailingScenario,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for scenario completion")
		<-result.Done

		// =====================================================================
		// Failing Scenario Verifications
		// =====================================================================

		By("Verifying recovery worker eventually succeeds")
		verifyRecoveryWorkerSucceeds(testLogger)

		By("Verifying permanent failure worker stays stuck")
		verifyPermanentWorkerStaysStuck(testLogger)

		By("Verifying action failures are logged with attempt counts")
		verifyActionFailuresLogged(testLogger)

		By("Verifying recovery worker transitions to Connected")
		verifyRecoveryWorkerReachesConnected(testLogger)

		By("Verifying permanent worker never reaches Connected")
		verifyPermanentWorkerNeverConnected(testLogger)
	})
})

// verifyRecoveryWorkerSucceeds checks that the recovery worker eventually succeeds after failures.
func verifyRecoveryWorkerSucceeds(t *integration.TestLogger) {
	succeededLogs := t.GetLogsMatching("connect_succeeded_after_failures")

	// Filter to only recovery worker
	recoverySucceeded := false
	for _, entry := range succeededLogs {
		worker := ""
		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}
		if strings.Contains(worker, "failing-worker-recovery") {
			recoverySucceeded = true
			break
		}
	}

	Expect(recoverySucceeded).To(BeTrue(),
		"Expected recovery worker to log 'connect_succeeded_after_failures'")

	GinkgoWriter.Printf("✓ Recovery worker succeeded after failures\n")
}

// verifyPermanentWorkerStaysStuck checks that the permanent failure worker keeps failing.
func verifyPermanentWorkerStaysStuck(t *integration.TestLogger) {
	failedLogs := t.GetLogsMatching("connect_failed_simulated")

	// Count failures for permanent worker
	permanentFailures := 0
	for _, entry := range failedLogs {
		worker := ""
		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}
		if strings.Contains(worker, "failing-worker-permanent") {
			permanentFailures++
		}
	}

	// Should have multiple failures (at least 3-4 in 8s with ~1s between retries)
	Expect(permanentFailures).To(BeNumerically(">=", 3),
		fmt.Sprintf("Expected permanent worker to fail at least 3 times, got %d", permanentFailures))

	// Should NOT have succeeded
	succeededLogs := t.GetLogsMatching("connect_succeeded_after_failures")
	for _, entry := range succeededLogs {
		worker := ""
		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}
		Expect(strings.Contains(worker, "failing-worker-permanent")).To(BeFalse(),
			"Permanent worker should NEVER succeed")
	}

	GinkgoWriter.Printf("✓ Permanent worker stayed stuck (%d failures, no success)\n", permanentFailures)
}

// verifyActionFailuresLogged checks that action failures include attempt counts.
func verifyActionFailuresLogged(t *integration.TestLogger) {
	failedLogs := t.GetLogsMatching("connect_failed_simulated")

	Expect(len(failedLogs)).To(BeNumerically(">=", 1),
		"Expected at least one connect_failed_simulated log")

	// Verify logs have attempt and remaining fields
	for _, entry := range failedLogs {
		hasAttempt := false
		hasRemaining := false
		for _, field := range entry.Context {
			if field.Key == "attempt" {
				hasAttempt = true
			}
			if field.Key == "remaining" {
				hasRemaining = true
			}
		}
		Expect(hasAttempt).To(BeTrue(),
			"connect_failed_simulated should have 'attempt' field")
		Expect(hasRemaining).To(BeTrue(),
			"connect_failed_simulated should have 'remaining' field")
	}

	GinkgoWriter.Printf("✓ Action failures logged with attempt counts (%d failures)\n", len(failedLogs))
}

// verifyRecoveryWorkerReachesConnected checks that recovery worker transitions to Connected.
func verifyRecoveryWorkerReachesConnected(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	recoveryReachedConnected := false
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
		if strings.Contains(worker, "failing-worker-recovery") && toState == "Connected" {
			recoveryReachedConnected = true
			break
		}
	}

	Expect(recoveryReachedConnected).To(BeTrue(),
		"Expected recovery worker to reach Connected state")

	GinkgoWriter.Printf("✓ Recovery worker reached Connected state\n")
}

// verifyPermanentWorkerNeverConnected checks that permanent worker never reaches Connected.
func verifyPermanentWorkerNeverConnected(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

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
		if strings.Contains(worker, "failing-worker-permanent") && toState == "Connected" {
			Fail("Permanent worker should NEVER reach Connected state")
		}
	}

	GinkgoWriter.Printf("✓ Permanent worker never reached Connected (stays in TryingToConnect)\n")
}
