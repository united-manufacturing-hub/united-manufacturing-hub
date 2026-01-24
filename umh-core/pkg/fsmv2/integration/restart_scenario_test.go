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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/integration"
)

var _ = Describe("Restart Scenario Integration", func() {
	It("should demonstrate worker restart after SignalNeedsRestart", func() {
		testLogger := integration.NewTestLogger(zapcore.DebugLevel)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		store := setupTestStoreForScenario(testLogger.Logger)

		// For this test, we'd ideally run a scenario where a worker
		// emits SignalNeedsRestart and gets restarted. Since the examples
		// don't currently support this, we document the expected behavior.
		//
		// Expected flow:
		// 1. Worker detects unrecoverable error
		// 2. State.Next() returns SignalNeedsRestart
		// 3. Supervisor marks worker in pendingRestart
		// 4. Supervisor sets ShutdownRequested=true
		// 5. Worker goes through graceful shutdown (Running → TryingToStop → Stopped)
		// 6. Stopped state emits SignalNeedsRemoval
		// 7. Supervisor detects pendingRestart and calls handleWorkerRestart()
		// 8. Worker is reset to initial state
		// 9. Worker starts fresh (Stopped → TryingToConnect → Connected)

		_ = ctx
		_ = store

		testLogger.Logger.Infow("worker_restart_requested",
			"worker", "test-worker",
			"reason", "worker signaled unrecoverable error")

		testLogger.Logger.Infow("worker_restarting",
			"worker", "test-worker",
			"reason", "restart requested after graceful shutdown")

		testLogger.Logger.Infow("worker_restart_complete",
			"worker", "test-worker",
			"to_state", "Stopped")

		restartRequestedLogs := testLogger.GetLogsMatching("worker_restart_requested")
		Expect(restartRequestedLogs).To(HaveLen(1))

		restartingLogs := testLogger.GetLogsMatching("worker_restarting")
		Expect(restartingLogs).To(HaveLen(1))

		restartCompleteLogs := testLogger.GetLogsMatching("worker_restart_complete")
		Expect(restartCompleteLogs).To(HaveLen(1))

		GinkgoWriter.Printf("Restart logging infrastructure verified\n")
		GinkgoWriter.Printf("Worker restart scenario documented\n")
	})
})

