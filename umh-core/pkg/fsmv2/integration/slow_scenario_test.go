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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/integration"
)

var _ = Describe("Slow Scenario Integration", func() {
	It("should demonstrate long-running action handling and context cancellation", func() {
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		store := setupTestStoreForScenario(testLogger.Logger)

		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 8*time.Second)
		defer scenarioCancel()

		result, err := examples.Run(scenarioCtx, examples.RunConfig{
			Scenario:     examples.SlowScenario,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		<-result.Done

		verifySloWorkerAttemptsConnect(testLogger)
		verifySloWorkerDelay(testLogger)
		verifySloWorkerConnected(testLogger)
		verifySloWorkerStateTransitions(testLogger)
		verifySloWorkerNoCancellation(testLogger)
	})
})

// verifySloWorkerAttemptsConnect checks that the slow worker logs its connection attempt.
func verifySloWorkerAttemptsConnect(t *integration.TestLogger) {
	attemptLogs := t.GetLogsMatching("connect_attempting")

	connectAttempted := false

	for _, entry := range attemptLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "slow-worker-1") {
			connectAttempted = true

			break
		}
	}

	Expect(connectAttempted).To(BeTrue(),
		"Expected slow-worker-1 to log 'connect_attempting'")

	GinkgoWriter.Printf("✓ Slow worker attempted connection with delay\n")
}

// verifySloWorkerDelay checks that the slow worker experienced the expected delay.
func verifySloWorkerDelay(t *integration.TestLogger) {
	delayCompletedLogs := t.GetLogsMatching("Connect delay completed successfully")

	delayCompleted := false

	for _, entry := range delayCompletedLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "slow-worker-1") {
			delayCompleted = true

			break
		}
	}

	Expect(delayCompleted).To(BeTrue(),
		"Expected slow-worker-1 to log 'Connect delay completed successfully'")

	GinkgoWriter.Printf("✓ Slow worker delay completed as expected\n")
}

// verifySloWorkerConnected checks that the slow worker reaches Connected state.
func verifySloWorkerConnected(t *integration.TestLogger) {
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

		if strings.Contains(worker, "slow-worker-1") && toState == "Connected" {
			slowConnected = true

			break
		}
	}

	Expect(slowConnected).To(BeTrue(),
		"Expected slow-worker-1 to reach Connected state after delay")

	GinkgoWriter.Printf("✓ Slow worker reached Connected state\n")
}

// verifySloWorkerStateTransitions checks the expected state progression.
func verifySloWorkerStateTransitions(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	stoppedToTrying := false
	tryingToConnected := false

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

		if !strings.Contains(worker, "slow-worker-1") {
			continue
		}

		if fromState == "Stopped" && toState == "TryingToConnect" {
			stoppedToTrying = true
		}

		if fromState == "TryingToConnect" && toState == "Connected" {
			tryingToConnected = true
		}
	}

	Expect(stoppedToTrying).To(BeTrue(),
		"Expected state transition: Stopped → TryingToConnect")
	Expect(tryingToConnected).To(BeTrue(),
		"Expected state transition: TryingToConnect → Connected")

	GinkgoWriter.Printf("✓ State transitions follow expected pattern: Stopped → TryingToConnect → Connected\n")
}

// verifySloWorkerNoCancellation checks that no cancellation occurred.
func verifySloWorkerNoCancellation(t *integration.TestLogger) {
	cancellationLogs := t.GetLogsMatching("Connect action cancelled during delay")

	for _, entry := range cancellationLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "slow-worker-1") {
			Fail("Slow worker should NOT be cancelled during the 8-second scenario duration")
		}
	}

	GinkgoWriter.Printf("✓ No unexpected cancellations occurred\n")
}
