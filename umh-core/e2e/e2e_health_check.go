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

package e2e_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:ST1001
	. "github.com/onsi/gomega"    //nolint:ST1001
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// getLogger returns a logger for e2e tests
func getLogger() *zap.SugaredLogger {
	// Create a new logger for testing
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

// waitForComponentsHealthy waits for all UMH components to become healthy
func waitForComponentsHealthy(mockServer *MockAPIServer) {
	By("Waiting for umh-core to send status messages")
	Eventually(func() bool {
		pushedMessages := mockServer.DrainPushedMessages()

		for _, msg := range pushedMessages {
			// Only check messages for our test email
			if msg.Email != "e2e-test@example.com" {
				continue
			}

			// Decode the base64 encoded message content
			content, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
			if err != nil {
				getLogger().Errorf("Failed to decode message content: %v", err)
				continue
			}

			if content.MessageType == models.Status {
				// Try to parse the status message payload
				if statusMsg := parseStatusMessageFromPayload(content.Payload); statusMsg != nil {
					getLogger().Info("Received status message:")
					getLogger().Infof("  Core: %s (message: %s)",
						safeGetHealthState(statusMsg.Core.Health), safeGetHealthMessage(statusMsg.Core.Health))
					getLogger().Infof("  Agent: %s (message: %s)",
						safeGetHealthState(statusMsg.Core.Agent.Health), safeGetHealthMessage(statusMsg.Core.Agent.Health))
					getLogger().Infof("  Redpanda: %s (message: %s)",
						safeGetHealthState(statusMsg.Core.Redpanda.Health), safeGetHealthMessage(statusMsg.Core.Redpanda.Health))
					getLogger().Infof("  TopicBrowser: %s (message: %s)",
						safeGetHealthState(statusMsg.Core.TopicBrowser.Health), safeGetHealthMessage(statusMsg.Core.TopicBrowser.Health))
					return true
				}
			}
		}
		return false
	}, 60*time.Second, 2*time.Second).Should(BeTrue(), "Should receive at least one status message")

	By("Verifying all components are in healthy states")
	Eventually(func() bool {
		var latestStatusMessage *models.StatusMessage
		pushedMessages := mockServer.DrainPushedMessages()

		for _, msg := range pushedMessages {
			if msg.Email != "e2e-test@example.com" {
				continue
			}

			content, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
			if err != nil {
				continue
			}

			if content.MessageType == models.Status {
				if statusMsg := parseStatusMessageFromPayload(content.Payload); statusMsg != nil {
					latestStatusMessage = statusMsg
				}
			}
		}

		if latestStatusMessage == nil {
			return false
		}

		// Check Core health
		if latestStatusMessage.Core.Health == nil || !isHealthActiveOrAcceptable(latestStatusMessage.Core.Health, "active", "running") {
			getLogger().Infof("Core not ready: %s", safeGetHealthState(latestStatusMessage.Core.Health))
			return false
		}

		// Check Agent health
		if latestStatusMessage.Core.Agent.Health == nil || !isHealthActiveOrAcceptable(latestStatusMessage.Core.Agent.Health, "active", "running") {
			getLogger().Infof("Agent not ready: %s", safeGetHealthState(latestStatusMessage.Core.Agent.Health))
			return false
		}

		// Check Redpanda health (active or idle)
		if latestStatusMessage.Core.Redpanda.Health == nil || !isHealthActiveOrAcceptable(latestStatusMessage.Core.Redpanda.Health, "active", "idle", "running") {
			getLogger().Infof("Redpanda not ready: %s", safeGetHealthState(latestStatusMessage.Core.Redpanda.Health))
			return false
		}

		// Check TopicBrowser health (active or idle)
		if latestStatusMessage.Core.TopicBrowser.Health == nil || !isHealthActiveOrAcceptable(latestStatusMessage.Core.TopicBrowser.Health, "active", "idle", "running") {
			getLogger().Infof("TopicBrowser not ready: %s", safeGetHealthState(latestStatusMessage.Core.TopicBrowser.Health))
			return false
		}

		getLogger().Info("All components are healthy!")
		return true
	}, 10*time.Second, 1*time.Second).Should(BeTrue(), "All components should eventually be in healthy states")
}

// startPeriodicSubscription sends subscription messages periodically to maintain the subscription
func startPeriodicSubscription(ctx context.Context, mockServer *MockAPIServer, interval time.Duration) {
	// Send initial subscription immediately
	getLogger().Info("Sending initial subscription message")
	subscribeMessage := createSubscriptionMessage()
	mockServer.AddMessageToPullQueue(subscribeMessage)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			getLogger().Info("Stopping periodic subscription sender")
			return
		case <-ticker.C:
			// Send resubscription message
			getLogger().Info("Sending periodic resubscription message")
			resubscribeMessage := createResubscriptionMessage()
			mockServer.AddMessageToPullQueue(resubscribeMessage)
		}
	}
}

// parseStatusMessageFromPayload converts payload to StatusMessage
func parseStatusMessageFromPayload(payload interface{}) *models.StatusMessage {
	// First try direct type assertion
	if statusMsg, ok := payload.(*models.StatusMessage); ok {
		return statusMsg
	}

	if statusMsg, ok := payload.(models.StatusMessage); ok {
		return &statusMsg
	}

	// If payload is a map, try to unmarshal it
	if payloadMap, ok := payload.(map[string]interface{}); ok {
		jsonBytes, err := json.Marshal(payloadMap)
		if err != nil {
			return nil
		}

		var statusMsg models.StatusMessage
		if err := json.Unmarshal(jsonBytes, &statusMsg); err != nil {
			return nil
		}

		return &statusMsg
	}

	return nil
}

// isHealthActiveOrAcceptable checks if health state matches any of the acceptable states
func isHealthActiveOrAcceptable(health *models.Health, acceptableStates ...string) bool {
	if health == nil {
		return false
	}

	// Check the ObservedState field
	for _, acceptable := range acceptableStates {
		if health.ObservedState == acceptable {
			return true
		}
	}

	return false
}

// safeGetHealthState safely extracts the health state, returning a fallback if nil
func safeGetHealthState(health *models.Health) string {
	if health == nil {
		return "<nil>"
	}
	if health.ObservedState == "" {
		return "<empty>"
	}
	return health.ObservedState
}

// safeGetHealthMessage safely extracts the health message, returning a fallback if nil
func safeGetHealthMessage(health *models.Health) string {
	if health == nil {
		return "<nil>"
	}
	if health.Message == "" {
		return "<empty>"
	}
	return health.Message
}
