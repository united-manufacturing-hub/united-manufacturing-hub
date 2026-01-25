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

var _ = Describe("Restart Scenario Integration", func() {
	It("should complete full worker restart cycle after SignalNeedsRestart", func() {
		By("Setting up test logger at DebugLevel")
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		By("Setting up context with 30s timeout")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		By("Setting up triangular store")
		store := setupTestStoreForScenario(testLogger.Logger)

		By("Creating scenario context with 10s duration")
		// 10s is enough to see the restart worker:
		// - 5 consecutive failures (at 100ms tick) triggers SignalNeedsRestart
		// - Graceful shutdown cycle completes
		// - Worker restarts and begins new attempt cycle
		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 10*time.Second)
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
		// Restart Scenario Verifications
		// =====================================================================
		// The FailingScenario includes "failing-worker-restart" configured with:
		//   restart_after_failures: 5
		// This triggers SignalNeedsRestart after 5 consecutive connect failures.
		//
		// Expected flow:
		// 1. Worker starts in Stopped state
		// 2. Parent signals children to run -> TryingToConnect
		// 3. ConnectAction fails 5 times (simulated failures)
		// 4. After 5th failure, TryingToConnectState.Next() returns SignalNeedsRestart
		// 5. Supervisor marks worker in pendingRestart, sets ShutdownRequested=true
		// 6. Worker transitions: TryingToConnect -> TryingToStop -> Stopped
		// 7. Stopped state emits SignalNeedsRemoval
		// 8. Supervisor detects pendingRestart, calls handleWorkerRestart()
		// 9. Worker resets to initial state (Stopped)
		// 10. Worker starts fresh cycle (Stopped -> TryingToConnect)
		// =====================================================================

		By("Verifying restart worker triggers SignalNeedsRestart")
		verifyRestartWorkerRequestsRestart(testLogger)

		By("Verifying restart worker completes full restart flow")
		verifyRestartWorkerCompletesRestart(testLogger)

		By("Verifying restart worker goes through TryingToStop during shutdown")
		verifyRestartWorkerGracefulShutdown(testLogger)

		By("Verifying restart worker begins new cycle after restart")
		verifyRestartWorkerBeginsNewCycle(testLogger)
	})
})

// verifyRestartWorkerRequestsRestart checks that the restart worker emits SignalNeedsRestart.
func verifyRestartWorkerRequestsRestart(t *integration.TestLogger) {
	restartLogs := t.GetLogsMatching("worker_restart_requested")

	restartWorkerTriggered := false

	for _, entry := range restartLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "failing-worker-restart") {
			restartWorkerTriggered = true

			break
		}
	}

	Expect(restartWorkerTriggered).To(BeTrue(),
		"Expected restart worker to trigger SignalNeedsRestart (worker_restart_requested log)")

	GinkgoWriter.Printf("Restart worker triggered SignalNeedsRestart\n")
}

// verifyRestartWorkerCompletesRestart checks the restart flow completes.
func verifyRestartWorkerCompletesRestart(t *integration.TestLogger) {
	completeLogs := t.GetLogsMatching("worker_restart_complete")

	found := false

	for _, entry := range completeLogs {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "failing-worker-restart") {
			found = true

			break
		}
	}

	Expect(found).To(BeTrue(),
		"Expected restart worker to complete restart flow (worker_restart_complete log)")

	GinkgoWriter.Printf("Restart worker completed full restart\n")
}

// verifyRestartWorkerGracefulShutdown checks the worker goes through TryingToStop.
func verifyRestartWorkerGracefulShutdown(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	sawTryingToStop := false

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

		if strings.Contains(worker, "failing-worker-restart") && toState == "TryingToStop" {
			sawTryingToStop = true

			break
		}
	}

	Expect(sawTryingToStop).To(BeTrue(),
		"Expected restart worker to transition to TryingToStop during graceful shutdown")

	GinkgoWriter.Printf("Restart worker went through TryingToStop (graceful shutdown)\n")
}

// verifyRestartWorkerBeginsNewCycle checks the worker starts a fresh cycle after restart.
func verifyRestartWorkerBeginsNewCycle(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	// Track sequence: we should see TryingToStop -> Stopped -> TryingToConnect
	// The second TryingToConnect indicates a new cycle began after restart
	tryingToConnectCount := 0
	sawStoppedAfterTryingToStop := false
	inShutdownPhase := false

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

		if !strings.Contains(worker, "failing-worker-restart") {
			continue
		}

		if toState == "TryingToConnect" {
			tryingToConnectCount++
		}

		if toState == "TryingToStop" {
			inShutdownPhase = true
		}

		if inShutdownPhase && fromState == "TryingToStop" && toState == "Stopped" {
			sawStoppedAfterTryingToStop = true
		}
	}

	// After restart, worker should attempt TryingToConnect again
	// Initial cycle: Stopped -> TryingToConnect (count=1)
	// After restart: Stopped -> TryingToConnect (count=2)
	Expect(tryingToConnectCount).To(BeNumerically(">=", 2),
		"Expected restart worker to begin new cycle (TryingToConnect at least twice)")

	Expect(sawStoppedAfterTryingToStop).To(BeTrue(),
		"Expected restart worker to complete graceful shutdown (TryingToStop -> Stopped)")

	GinkgoWriter.Printf("Restart worker began new cycle after restart (TryingToConnect count: %d)\n", tryingToConnectCount)
}
