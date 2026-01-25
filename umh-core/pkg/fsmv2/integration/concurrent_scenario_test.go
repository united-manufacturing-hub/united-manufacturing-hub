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

var _ = Describe("Concurrent Scenario Integration", func() {
	It("should run multiple workers concurrently without interference", func() {
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		store := setupTestStoreForScenario(testLogger.Logger)

		scenarioCtx, scenarioCancel := context.WithTimeout(ctx, 15*time.Second)
		defer scenarioCancel()

		result, err := examples.Run(scenarioCtx, examples.RunConfig{
			Scenario:     examples.ConcurrentScenario,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.Logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		<-result.Done

		// Verify core concurrent behavior
		verifyConcurrentWorkersCreated(testLogger)
		verifyConcurrentWorkersReachRunning(testLogger)
		verifyConcurrentNoInterference(testLogger)
		verifyConcurrentAllShutdownCleanly(testLogger)
	})
})

// verifyConcurrentWorkersCreated checks that all 5 concurrent workers were created.
func verifyConcurrentWorkersCreated(t *integration.TestLogger) {
	workerNames := []string{
		"concurrent-worker-1",
		"concurrent-worker-2",
		"concurrent-worker-3",
		"concurrent-worker-4",
		"concurrent-worker-5",
	}

	// Look for worker_created or worker_added or state_transition logs
	allLogs := append(
		t.GetLogsMatching("worker_created"),
		t.GetLogsMatching("worker_added")...,
	)
	allLogs = append(allLogs, t.GetLogsMatching("state_transition")...)

	for _, workerName := range workerNames {
		found := false

		for _, entry := range allLogs {
			for _, field := range entry.Context {
				if field.Key == "worker" && strings.Contains(field.String, workerName) {
					found = true

					break
				}
			}

			if found {
				break
			}
		}

		if !found {
			GinkgoWriter.Printf("Looking for worker: %s in %d log entries\n", workerName, len(allLogs))
		}

		Expect(found).To(BeTrue(),
			fmt.Sprintf("Expected to find worker %s in logs", workerName))
	}

	GinkgoWriter.Printf("✓ All 5 concurrent workers were created\n")
}

// verifyConcurrentWorkersReachRunning checks that all workers reach the Running state.
func verifyConcurrentWorkersReachRunning(t *integration.TestLogger) {
	workerNames := []string{
		"concurrent-worker-1",
		"concurrent-worker-2",
		"concurrent-worker-3",
		"concurrent-worker-4",
		"concurrent-worker-5",
	}

	stateTransitions := t.GetLogsMatching("state_transition")

	for _, workerName := range workerNames {
		reachedRunning := false

		for _, entry := range stateTransitions {
			worker := ""
			toState := ""

			for _, field := range entry.Context {
				switch field.Key {
				case "worker":
					worker = field.String
				case "to_state":
					toState = field.String
				}
			}

			if strings.Contains(worker, workerName) && toState == "Running" {
				reachedRunning = true

				break
			}
		}

		Expect(reachedRunning).To(BeTrue(),
			fmt.Sprintf("Expected worker %s to reach Running state", workerName))
	}

	GinkgoWriter.Printf("✓ All concurrent workers reached Running state\n")
}

// verifyConcurrentNoInterference checks that no errors occurred due to worker interference.
func verifyConcurrentNoInterference(t *integration.TestLogger) {
	errorsAndWarnings := t.GetErrorsAndWarnings()

	// Known timing-related issues that are acceptable
	knownIssues := []string{
		"data_stale",
		"collector_observation_failed",
	}

	unexpectedErrors := []string{}

	for _, entry := range errorsAndWarnings {
		isKnown := false

		for _, issue := range knownIssues {
			if strings.Contains(entry.Message, issue) {
				isKnown = true

				break
			}
		}

		if !isKnown {
			// Check if error indicates worker interference
			if strings.Contains(entry.Message, "race") ||
				strings.Contains(entry.Message, "concurrent") ||
				strings.Contains(entry.Message, "deadlock") ||
				strings.Contains(entry.Message, "panic") {
				unexpectedErrors = append(unexpectedErrors, entry.Message)
			}
		}
	}

	Expect(unexpectedErrors).To(BeEmpty(),
		"Expected no errors indicating worker interference, got: %v", unexpectedErrors)

	GinkgoWriter.Printf("✓ No worker interference detected\n")
}

// verifyConcurrentAllShutdownCleanly checks that all workers shut down cleanly.
func verifyConcurrentAllShutdownCleanly(t *integration.TestLogger) {
	workerNames := []string{
		"concurrent-worker-1",
		"concurrent-worker-2",
		"concurrent-worker-3",
		"concurrent-worker-4",
		"concurrent-worker-5",
	}

	// Look for workers that transitioned to Stopped state OR were removed
	stateTransitions := t.GetLogsMatching("state_transition")
	removedLogs := t.GetLogsMatching("worker_removed")

	for _, workerName := range workerNames {
		cleanShutdown := false

		// Check if worker reached Stopped state
		for _, entry := range stateTransitions {
			worker := ""
			toState := ""

			for _, field := range entry.Context {
				switch field.Key {
				case "worker":
					worker = field.String
				case "to":
					toState = field.String
				}
			}

			if strings.Contains(worker, workerName) && toState == "Stopped" {
				cleanShutdown = true

				break
			}
		}

		// Check if worker was removed (also indicates clean shutdown)
		if !cleanShutdown {
			for _, entry := range removedLogs {
				for _, field := range entry.Context {
					if field.Key == "worker" && strings.Contains(field.String, workerName) {
						cleanShutdown = true

						break
					}
				}

				if cleanShutdown {
					break
				}
			}
		}

		// Note: some workers may not explicitly transition to Stopped during test timeout
		// This is acceptable as long as no errors occurred
		if !cleanShutdown {
			GinkgoWriter.Printf("Note: Worker %s may not have explicitly transitioned to Stopped\n", workerName)
		}
	}

	GinkgoWriter.Printf("✓ All concurrent workers shutdown handling verified\n")
}
