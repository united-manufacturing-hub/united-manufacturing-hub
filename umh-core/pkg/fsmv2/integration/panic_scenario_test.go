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

var _ = Describe("Panic Scenario Integration", func() {
	It("should demonstrate panic recovery and logging", func() {
		By("Setting up test logger at DebugLevel")
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		By("Setting up context with 30s timeout")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		By("Setting up triangular store")
		store := setupTestStoreForScenario(testLogger.Logger)

		By("Creating scenario context with 5s duration")
		// Short duration - panic should be caught quickly
		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 5*time.Second)
		defer scenarioCancel()

		By("Running PanicScenario with 100ms tick interval")
		result, err := examples.Run(scenarioCtx, examples.RunConfig{
			Scenario:     examples.PanicScenario,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for scenario completion")
		<-result.Done

		// =====================================================================
		// Panic Scenario Verifications
		// =====================================================================

		By("Verifying panic worker was created")
		verifyPanicWorkerCreated(testLogger)

		By("Verifying panic was caught and logged")
		verifyPanicWasCaught(testLogger)

		By("Verifying worker never reached Connected state")
		verifyPanicWorkerNeverConnected(testLogger)
	})
})

// verifyPanicWorkerCreated checks that the panic worker was created.
func verifyPanicWorkerCreated(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	workerFound := false

	for _, entry := range stateTransitions {
		worker := ""

		for _, field := range entry.Context {
			if field.Key == "worker" {
				worker = field.String
			}
		}

		if strings.Contains(worker, "panic-worker") {
			workerFound = true

			break
		}
	}

	Expect(workerFound).To(BeTrue(),
		"Expected panic-worker to be created")
	GinkgoWriter.Printf("✓ Panic worker was created\n")
}

// verifyPanicWasCaught checks that the panic was caught and logged.
func verifyPanicWasCaught(t *integration.TestLogger) {
	panicLogs := t.GetLogsMatching("action_panic")

	Expect(panicLogs).ToNot(BeEmpty(),
		"Expected at least one action_panic log entry")

	// Verify panic message contains expected text
	panicFound := false

	for _, entry := range panicLogs {
		for _, field := range entry.Context {
			if field.Key == "panic" {
				panicStr := field.String
				if panicStr == "" {
					// Try interface value
					if interfaceVal, ok := field.Interface.(string); ok {
						panicStr = interfaceVal
					}
				}

				if strings.Contains(panicStr, "simulated panic") ||
					strings.Contains(panicStr, "panic") {
					panicFound = true
				}
			}
		}
	}

	Expect(panicFound).To(BeTrue(),
		"Expected panic log to contain panic-related message")
	GinkgoWriter.Printf("✓ Panic was caught and logged (%d panic entries)\n", len(panicLogs))
}

// verifyPanicWorkerNeverConnected checks that the panic worker never reached Connected.
func verifyPanicWorkerNeverConnected(t *integration.TestLogger) {
	stateTransitions := t.GetLogsMatching("state_transition")

	reachedConnected := false

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

		if strings.Contains(worker, "panic-worker") && toState == "Connected" {
			reachedConnected = true

			break
		}
	}

	Expect(reachedConnected).To(BeFalse(),
		"Expected panic-worker to never reach Connected state (panic prevents it)")
	GinkgoWriter.Printf("✓ Panic worker never reached Connected (as expected)\n")
}
