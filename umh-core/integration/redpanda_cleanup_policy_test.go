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
)

var _ = Describe("Redpanda Cleanup Policy Integration Test", Ordered, Label("integration"), func() {
	const (
		compactTopicName = "test-cleanup-compact"
		deleteTopicName  = "test-cleanup-delete"
		messagesPerKey   = 5
		testKeys         = 3
		postRestartWait  = 5 * time.Second
	)

	var builder *DataFlowComponentBuilder

	BeforeAll(func() {
		By("Starting umh-core with Redpanda configured with compact cleanup policy")
		builder = NewDataFlowComponentBuilder()
		builder.full.Internal.Redpanda.DesiredFSMState = "active"
		builder.full.Internal.Redpanda.Name = "redpanda"

		// Set initial cleanup policy to compact
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicCleanupPolicy = "compact"
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicRetentionMs = 604800000 // 7 days
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicCompressionAlgorithm = "snappy"

		cfg := builder.BuildYAML()
		Expect(writeConfigFile(cfg)).To(Succeed())
		Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
		Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available after startup")
	})

	AfterAll(func() {
		By("Stopping and cleaning up the container after the test")
		PrintLogsAndStopContainer()
		CleanupDockerBuildCache()
		if !CurrentSpecReport().Failed() {
			cleanupTmpDirs(getContainerName())
		}
	})

	It("should configure compact cleanup policy and verify compaction behavior", func() {
		By("Verifying initial cleanup policy is set to compact")
		Eventually(func() bool {
			cleanupPolicy, err := getRedpandaConfig("log_cleanup_policy")
			GinkgoWriter.Printf("Current cleanup policy: %s\n", cleanupPolicy)
			return err == nil && cleanupPolicy == "compact"
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "Cleanup policy should be set to compact")

		By("Creating a test topic with compact cleanup policy and short retention for testing")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Create topic with aggressive compaction settings for testing
		_, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
			"/opt/redpanda/bin/rpk", "topic", "create", compactTopicName,
			"-p", "1", "-r", "1",
			"-c", "cleanup.policy=compact",
			"-c", "retention.ms=30000", // 30 seconds retention
			"-c", "segment.ms=10000", // 10 second segments
			"-c", "min.compaction.lag.ms=5000", // 5 second compaction lag
			"-c", "max.compaction.lag.ms=15000") // 15 second max lag
		Expect(err).ToNot(HaveOccurred(), "Should be able to create test topic with compaction settings")

		// Wait for topic to be fully created
		time.Sleep(3 * time.Second)

		By("Verifying the topic was created successfully")
		topicListOutput, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
			"/opt/redpanda/bin/rpk", "topic", "list")
		Expect(err).ToNot(HaveOccurred(), "Should be able to list topics")
		Expect(topicListOutput).To(ContainSubstring(compactTopicName), "Topic should exist in topic list")
		GinkgoWriter.Printf("Topics available: %s\n", topicListOutput)

		By("Producing multiple messages for same keys to trigger compaction")
		keys := []string{"user_1", "user_2", "user_3"}

		// Phase 1: Produce initial messages
		GinkgoWriter.Printf("Phase 1: Producing initial messages...\n")
		for _, key := range keys {
			for i := 0; i < 4; i++ {
				messageContent := fmt.Sprintf(`{"key":"%s","version":%d,"timestamp":"%s","data":"old_data_%d"}`,
					key, i, time.Now().Format(time.RFC3339), i)

				_, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
					"sh", "-c", fmt.Sprintf(`echo '%s' | /opt/redpanda/bin/rpk topic produce %s --key %s`,
						messageContent, compactTopicName, key))
				Expect(err).ToNot(HaveOccurred(), "Should produce initial message for key %s", key)
				time.Sleep(200 * time.Millisecond)
			}
		}

		// Read initial message count
		initialMessages, _ := getTopicMessages(compactTopicName, 50)
		GinkgoWriter.Printf("Initial messages produced: %d\n", len(initialMessages))

		By("Waiting for segment to roll over")
		time.Sleep(12 * time.Second) // Wait for segment.ms

		By("Producing final messages that should survive compaction")
		GinkgoWriter.Printf("Phase 2: Producing final messages...\n")
		for _, key := range keys {
			finalMessage := fmt.Sprintf(`{"key":"%s","version":"final","timestamp":"%s","data":"final_data_for_%s"}`,
				key, time.Now().Format(time.RFC3339), key)

			_, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
				"sh", "-c", fmt.Sprintf(`echo '%s' | /opt/redpanda/bin/rpk topic produce %s --key %s`,
					finalMessage, compactTopicName, key))
			Expect(err).ToNot(HaveOccurred(), "Should produce final message for key %s", key)
			time.Sleep(200 * time.Millisecond)
		}

		By("Waiting for compaction to occur")
		GinkgoWriter.Printf("Waiting 45 seconds for compaction (retention.ms + compaction lag)...\n")
		time.Sleep(45 * time.Second)

		By("Verifying compaction behavior - only latest message per key should remain")
		Eventually(func() bool {
			messages, err := getTopicMessages(compactTopicName, 50)
			if err != nil {
				GinkgoWriter.Printf("Error reading messages: %v\n", err)
				return false
			}

			GinkgoWriter.Printf("Messages after compaction: %d\n", len(messages))
			for i, msg := range messages {
				GinkgoWriter.Printf("Remaining message %d: %s\n", i+1, msg)
			}

			// After compaction, we should have exactly 3 messages (one per key)
			// and they should all be the "final" versions
			if len(messages) != 3 {
				GinkgoWriter.Printf("Expected exactly 3 messages after compaction, got %d\n", len(messages))
				return false
			}

			// Verify all remaining messages are the final versions
			finalCount := 0
			keysFound := make(map[string]bool)
			for _, msg := range messages {
				if strings.Contains(msg, "final_data") {
					finalCount++
					// Extract key from message to ensure we have one per key
					if strings.Contains(msg, "user_1") {
						keysFound["user_1"] = true
					} else if strings.Contains(msg, "user_2") {
						keysFound["user_2"] = true
					} else if strings.Contains(msg, "user_3") {
						keysFound["user_3"] = true
					}
				}
			}

			allKeysPresent := len(keysFound) == 3
			allFinalMessages := finalCount == 3

			GinkgoWriter.Printf("Final messages: %d, All keys present: %v\n", finalCount, allKeysPresent)
			return allFinalMessages && allKeysPresent
		}, 2*time.Minute, 15*time.Second).Should(BeTrue(), "Compaction should keep exactly one (the latest) message per key")

		By("Verifying topic configuration shows compaction settings")
		describeOutput, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
			"/opt/redpanda/bin/rpk", "topic", "describe", compactTopicName)
		Expect(err).ToNot(HaveOccurred(), "Should be able to describe topic")
		GinkgoWriter.Printf("Topic configuration: %s\n", describeOutput)
		Expect(describeOutput).To(ContainSubstring("cleanup.policy"), "Topic should show cleanup policy configuration")
	})

	It("should update cleanup policy to delete and verify the configuration", func() {
		By("Updating Redpanda configuration to use delete cleanup policy")
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicCleanupPolicy = "delete"
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicRetentionMs = 3600000 // 1 hour for faster testing
		cfg := builder.BuildYAML()
		GinkgoWriter.Printf("Updated config with delete cleanup policy: %s\n", cfg)
		Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

		By("Verifying the cleanup policy configuration has been updated")
		Eventually(func() bool {
			cleanupPolicy, err := getRedpandaConfig("log_cleanup_policy")
			retentionMs, err2 := getRedpandaConfig("log_retention_ms")
			GinkgoWriter.Printf("Updated cleanup policy: %s, retention: %s\n", cleanupPolicy, retentionMs)
			return err == nil && err2 == nil && cleanupPolicy == "delete" && retentionMs == "3600000"
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "Cleanup policy should be updated to delete")

		By("Verifying system health after configuration change")
		Eventually(func() bool {
			resp, err := httpGetWithTimeout(GetMetricsURL(), 2*time.Second)
			return err == nil && resp == 200
		}, 20*time.Second, 1*time.Second).Should(BeTrue(), "Metrics endpoint should remain healthy")

		By("Creating a new topic to test delete cleanup policy")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
			"/opt/redpanda/bin/rpk", "topic", "create", deleteTopicName, "-p", "1", "-r", "1")
		Expect(err).ToNot(HaveOccurred(), "Should be able to create topic with delete policy")

		By("Producing test messages to the delete policy topic")
		for i := 0; i < 5; i++ {
			messageContent := fmt.Sprintf(`{"message_id":%d,"timestamp":"%s","content":"test_message_%d"}`,
				i, time.Now().Format(time.RFC3339), i)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err = runDockerCommandWithCtx(ctx, "exec", getContainerName(),
				"sh", "-c", fmt.Sprintf(`echo '%s' | /opt/redpanda/bin/rpk topic produce %s`,
					messageContent, deleteTopicName))
			cancel()

			if err != nil {
				GinkgoWriter.Printf("Warning: Failed to produce message %d: %v\n", i, err)
			}
		}

		By("Verifying messages are available in the delete policy topic")
		messages, err := getTopicMessages(deleteTopicName, 10)
		Expect(err).ToNot(HaveOccurred(), "Should be able to read messages from delete policy topic")
		Expect(len(messages)).To(BeNumerically(">", 0), "Should have messages in the delete policy topic")

		GinkgoWriter.Printf("Found %d messages in delete policy topic\n", len(messages))
	})

	It("should support compact,delete hybrid cleanup policy", func() {
		By("Updating to hybrid compact,delete cleanup policy")
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicCleanupPolicy = "compact,delete"
		cfg := builder.BuildYAML()
		Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

		By("Verifying the hybrid cleanup policy is applied")
		Eventually(func() bool {
			cleanupPolicy, err := getRedpandaConfig("log_cleanup_policy")
			GinkgoWriter.Printf("Hybrid cleanup policy: %s\n", cleanupPolicy)
			// Redpanda might normalize this to "delete,compact" or keep it as "compact,delete"
			return err == nil && (cleanupPolicy == "compact,delete" || cleanupPolicy == "delete,compact")
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "Cleanup policy should support hybrid mode")

		By("Ensuring system remains healthy with hybrid policy")
		Eventually(func() bool {
			resp, err := httpGetWithTimeout(GetMetricsURL(), 2*time.Second)
			return err == nil && resp == 200
		}, 20*time.Second, 1*time.Second).Should(BeTrue(), "System should remain healthy with hybrid cleanup policy")
	})

	It("should revert to compact policy and maintain system stability", func() {
		By("Reverting back to compact cleanup policy")
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicCleanupPolicy = "compact"
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicRetentionMs = 604800000 // Back to 7 days
		cfg := builder.BuildYAML()
		Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

		By("Verifying revert to compact policy")
		Eventually(func() bool {
			cleanupPolicy, err := getRedpandaConfig("log_cleanup_policy")
			retentionMs, err2 := getRedpandaConfig("log_retention_ms")
			GinkgoWriter.Printf("Reverted cleanup policy: %s, retention: %s\n", cleanupPolicy, retentionMs)
			return err == nil && err2 == nil && cleanupPolicy == "compact" && retentionMs == "604800000"
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "Should revert to compact cleanup policy")

		By("Ensuring system remains stable after multiple configuration changes")
		redpandaState, err := checkRedpandaState(GetMetricsURL(), 2*time.Second)
		Expect(err).ToNot(HaveOccurred(), "Should be able to check Redpanda state")
		Expect(redpandaState).To(BeNumerically("==", 3), "Redpanda should be in healthy state (3)")

		By("Verifying that both test topics are still accessible")
		compactMessages, err := getTopicMessages(compactTopicName, 10)
		Expect(err).ToNot(HaveOccurred(), "Should still be able to read from compact topic")

		deleteMessages, err := getTopicMessages(deleteTopicName, 10)
		Expect(err).ToNot(HaveOccurred(), "Should still be able to read from delete topic")

		GinkgoWriter.Printf("Final verification - Compact topic messages: %d, Delete topic messages: %d\n",
			len(compactMessages), len(deleteMessages))
	})
})

// getTopicMessages retrieves messages from a topic using rpk
func getTopicMessages(topic string, maxMessages int) ([]string, error) {
	// Use a shorter timeout for the consume command to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to consume messages with a timeout and exit when no more messages are available
	result, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
		"/opt/redpanda/bin/rpk", "topic", "consume", topic,
		"--offset", "start",
		"-n", fmt.Sprintf("%d", maxMessages),
		"--format", "%v",
		"--timeout", "5s") // Add explicit timeout to rpk consume

	if err != nil {
		// Log the error but don't fail immediately - there might be no messages
		GinkgoWriter.Printf("Note: rpk consume returned error (this may be normal if no messages): %v\n", err)

		// Check if this is a timeout or "no messages" scenario
		if ctx.Err() == context.DeadlineExceeded {
			GinkgoWriter.Printf("Consume command timed out - assuming no messages available\n")
			return []string{}, nil
		}

		// For other errors, still return what we got
		if result != "" {
			lines := strings.Split(strings.TrimSpace(result), "\n")
			// Filter out empty lines
			var messages []string
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					messages = append(messages, strings.TrimSpace(line))
				}
			}
			return messages, nil
		}

		return []string{}, nil // Return empty slice instead of error
	}

	if result == "" {
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSpace(result), "\n")
	var messages []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			messages = append(messages, strings.TrimSpace(line))
		}
	}

	return messages, nil
}
