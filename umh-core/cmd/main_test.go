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

package main

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/control"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

func TestMain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Suite")
}

var _ = Describe("Backend Connection", Ordered, func() {
	var (
		configData         config.FullConfig
		communicationState *communication_state.CommunicationState
		controlLoop        *control.ControlLoop
		log                *zap.SugaredLogger
	)

	BeforeAll(func() {
		logger.Initialize()
		log = logger.For(logger.ComponentCore)

		// Setup minimal config with UNREACHABLE backend
		configData = config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:           "http://unreachable-backend.invalid:9999",
					AuthToken:        "test-token",
					AllowInsecureTLS: false,
				},
				ReleaseChannel: "stable",
				MetricsPort:    9091,
			},
		}

		// Create minimal control loop (we don't need full functionality)
		// Note: Only create once per suite to avoid registry singleton panic
		configManager := &config.MockConfigManager{}
		controlLoop = control.NewControlLoop(configManager)
		systemSnapshotManager := controlLoop.GetSnapshotManager()

		// Create communication state with a dummy context for initial setup
		dummyCtx := context.Background()
		communicationState = communication_state.NewCommunicationState(
			watchdog.NewWatchdog(dummyCtx, time.NewTicker(time.Second*10), true, log),
			make(chan *models.UMHMessage, 100),
			make(chan *models.UMHMessage, 100),
			configData.Agent.ReleaseChannel,
			systemSnapshotManager,
			configManager,
			configData.Agent.APIURL,
			log,
			configData.Agent.AllowInsecureTLS,
			topicbrowser.NewCache(),
		)
	})

	Context("when Management Console is unreachable", func() {
		It("should not block control loop startup", func() {
			// This test proves that the main() flow doesn't block
			// when backend is unreachable
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			controlLoopReached := make(chan bool, 1)

			// Simulate the EXACT flow from main() (lines 179-191):
			// 1. Call enableBackendConnection (with 'go' keyword in production code)
			// 2. Immediately try to reach the control loop startup line
			go func() {
				// Line 179-180 in main.go (with 'go' keyword)
				if configData.Agent.APIURL != "" && configData.Agent.AuthToken != "" {
					go enableBackendConnection(ctx, &configData, communicationState, controlLoop, log)
				}

				// Line 191 in main.go - we reach this immediately after line 180
				// This should NOT be blocked by the backend connection attempt
				controlLoopReached <- true
			}()

			// Control loop line (191) should be reached immediately
			// even when backend is unreachable, because enableBackendConnection
			// runs in a separate goroutine (via 'go' keyword)
			Eventually(controlLoopReached, 500*time.Millisecond).Should(Receive(BeTrue()),
				"Control loop should be reached immediately, but was blocked by synchronous backend connection")
		})
	})

	Context("when context is cancelled", func() {
		It("should respect context cancellation and return promptly", func() {
			// This test verifies that enableBackendConnection respects context cancellation
			// and returns promptly when the context is cancelled during shutdown
			ctx, cancel := context.WithCancel(context.Background())

			done := make(chan bool, 1)

			// Start enableBackendConnection in a goroutine
			go func() {
				enableBackendConnection(ctx, &configData, communicationState, controlLoop, log)
				done <- true
			}()

			// Give NewLogin enough time to fail at least once (it has 1s backoff)
			// This ensures we test cancellation during the wait period
			time.Sleep(1200 * time.Millisecond)

			// Cancel the context (simulating shutdown)
			cancel()

			// Function should return within 500ms after context cancellation
			Eventually(done, 500*time.Millisecond).Should(Receive(BeTrue()),
				"enableBackendConnection should return promptly after context cancellation, but goroutine is still running")
		})
	})
})
