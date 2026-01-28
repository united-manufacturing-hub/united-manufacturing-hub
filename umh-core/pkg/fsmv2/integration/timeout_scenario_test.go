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

var _ = Describe("Timeout Scenario Integration", func() {
	It("should demonstrate timeout and retry behavior patterns", func() {
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		store := setupTestStoreForScenario(testLogger.Logger)

		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 10*time.Second)
		defer scenarioCancel()

		result, err := examples.Run(scenarioCtx, examples.RunConfig{
			Scenario:     examples.TimeoutScenario,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		<-result.Done

		verifyTimeoutQuickWorker(testLogger)
		verifyTimeoutSlowWorker(testLogger)
		verifyTimeoutRetryWorker(testLogger)
		verifyTimeoutCombinedWorker(testLogger)
	})
})

// verifyTimeoutQuickWorker checks that the quick worker completes immediately.
func verifyTimeoutQuickWorker(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	quickConnected := false

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

		if strings.Contains(worker, "timeout-quick") && toState == "Connected" {
			quickConnected = true

			break
		}
	}

	Expect(quickConnected).To(BeTrue(),
		"Expected timeout-quick worker to reach Connected state")

	GinkgoWriter.Printf("✓ Quick worker connected immediately\n")
}

// verifyTimeoutSlowWorker checks that the slow worker completes after delay.
func verifyTimeoutSlowWorker(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	slowConnected := false

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

		if strings.Contains(worker, "timeout-slow") && toState == "Connected" {
			slowConnected = true

			break
		}
	}

	Expect(slowConnected).To(BeTrue(),
		"Expected timeout-slow worker to reach Connected state after delay")

	GinkgoWriter.Printf("✓ Slow worker connected after delay\n")
}

// verifyTimeoutRetryWorker checks that the retry worker demonstrates retry pattern.
func verifyTimeoutRetryWorker(t *integration.TestLogger) {
	failureLogs := t.GetLogsMatching("connect_failed_simulated")

	retryFailures := 0

	for _, entry := range failureLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "timeout-retry") {
			retryFailures++
		}
	}

	// Should have 3 failures before success
	Expect(retryFailures).To(BeNumerically(">=", 3),
		fmt.Sprintf("Expected timeout-retry worker to fail at least 3 times, got %d", retryFailures))

	// Check for eventual success
	successLogs := t.GetLogsMatching("connect_succeeded_after_failures")
	retryRecovered := false

	for _, entry := range successLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "timeout-retry") {
			retryRecovered = true

			break
		}
	}

	Expect(retryRecovered).To(BeTrue(),
		"Expected timeout-retry worker to eventually succeed")

	GinkgoWriter.Printf("✓ Retry worker demonstrated retry pattern (%d failures before success)\n", retryFailures)
}

// verifyTimeoutCombinedWorker checks the combined slow + retry pattern.
func verifyTimeoutCombinedWorker(t *integration.TestLogger) {
	failureLogs := t.GetLogsMatching("connect_failed_simulated")

	combinedFailures := 0

	for _, entry := range failureLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "timeout-combined") {
			combinedFailures++
		}
	}

	// Should have 5 failures before success (max_failures: 5)
	Expect(combinedFailures).To(BeNumerically(">=", 5),
		fmt.Sprintf("Expected timeout-combined to fail at least 5 times, got %d", combinedFailures))

	// Check for eventual success
	successLogs := t.GetLogsMatching("connect_succeeded_after_failures")
	combinedRecovered := false

	for _, entry := range successLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "timeout-combined") {
			combinedRecovered = true

			break
		}
	}

	Expect(combinedRecovered).To(BeTrue(),
		"Expected timeout-combined worker to eventually succeed")

	GinkgoWriter.Printf("✓ Combined worker demonstrated failure + recovery pattern (%d failures)\n", combinedFailures)
}
