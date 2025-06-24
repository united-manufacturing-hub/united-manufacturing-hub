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

		By("Creating a test topic with compact cleanup policy")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Create topic explicitly to ensure it uses the current default settings
		_, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
			"/opt/redpanda/bin/rpk", "topic", "create", compactTopicName, "-p", "1", "-r", "1")
		Expect(err).ToNot(HaveOccurred(), "Should be able to create test topic")

		By("Producing multiple messages with the same keys to test compaction")
		// Produce multiple messages for each key
		for key := 0; key < testKeys; key++ {
			for msg := 0; msg < messagesPerKey; msg++ {
				messageContent := fmt.Sprintf(`{"key":"%d","message_id":%d,"timestamp":"%s","content":"message_%d_for_key_%d"}`,
					key, msg, time.Now().Format(time.RFC3339), msg, key)

				// Use echo to pipe message content to rpk
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
					"sh", "-c", fmt.Sprintf(`echo '%s' | /opt/redpanda/bin/rpk topic produce %s --key key_%d`,
						messageContent, compactTopicName, key))
				cancel()

				// Don't fail immediately on individual message failures, but log them
				if err != nil {
					GinkgoWriter.Printf("Warning: Failed to produce message %d for key %d: %v\n", msg, key, err)
				}

				// Small delay between messages to ensure different timestamps
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Wait a bit for compaction to potentially occur
		time.Sleep(5 * time.Second)

		By("Verifying messages are available in the topic")
		messages, err := getTopicMessages(compactTopicName, 50)
		Expect(err).ToNot(HaveOccurred(), "Should be able to read messages from topic")
		Expect(len(messages)).To(BeNumerically(">", 0), "Should have messages in the topic")

		GinkgoWriter.Printf("Found %d messages in compact topic\n", len(messages))
		for i, msg := range messages {
			GinkgoWriter.Printf("Message %d: %s\n", i, msg)
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(),
		"/opt/redpanda/bin/rpk", "topic", "consume", topic,
		"--offset", "start", "-n", fmt.Sprintf("%d", maxMessages),
		"--format", "%v")

	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	messages := make([]string, 0, len(lines))

	for _, line := range lines {
		if line != "" {
			messages = append(messages, line)
		}
	}

	return messages, nil
}
