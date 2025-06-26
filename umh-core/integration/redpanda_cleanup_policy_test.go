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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Redpanda Cleanup Policy Integration Test", Ordered, Label("integration"), func() {
	const (
		topicName         = "dfc-cleanup-policy-test-topic"
		messagesPerSecond = 5
		testDuration      = 10 * time.Second
		postRestartWait   = 5 * time.Second
		compactionTime    = 30000 // 30 seconds in milliseconds
	)

	var lastOffset = -1
	var lastTimestamp time.Time
	var builder *DataFlowComponentBuilder

	BeforeAll(func() {
		By("Starting umh-core with Redpanda and a DFC that outputs to Kafka")
		builder = NewDataFlowComponentBuilder()
		builder.full.Internal.Redpanda.DesiredFSMState = "active"
		builder.full.Internal.Redpanda.Name = "redpanda"
		key := "some-key"
		builder.AddGeneratorDataFlowComponentToKafka("dfc-cleanup-policy", fmt.Sprintf("%dms", 1000/messagesPerSecond), topicName, &key)
		cfg := builder.BuildYAML()
		Expect(writeConfigFile(cfg)).To(Succeed())
		Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
		Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available after startup")
	})

	AfterEach(func() {
		// Reset the offset and timestamp
		lastOffset = -1
		lastTimestamp = time.Now()
	})

	AfterAll(func() {
		By("Stopping and cleaning up the container after the test")
		PrintLogsAndStopContainer()
		CleanupDockerBuildCache()
		if !CurrentSpecReport().Failed() {
			cleanupTmpDirs(getContainerName())
		}
	})

	It("should update Redpanda config and continue producing messages", func() {
		// Wait for initial messages to be produced
		Eventually(func() bool {
			newOffset, err := checkRPK(topicName, lastOffset, lastTimestamp, 0.1, 0.2, messagesPerSecond)
			lastOffset = newOffset
			lastTimestamp = time.Now()
			return err == nil && newOffset != -1
		}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Messages should be produced initially")

		By("Updating Redpanda configuration with new retention time")
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicRetentionMs = compactionTime
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicCleanupPolicy = "compact"
		cfg := builder.BuildYAML()
		GinkgoWriter.Printf("Updated config: %s\n", cfg)
		Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

		By("Checking if the config has been applied")
		Eventually(func() bool {
			redpandaConfig, err := getRedpandaConfig("log_retention_ms")
			cleanupPolicy, err2 := getRedpandaConfig("log_cleanup_policy")
			GinkgoWriter.Printf("Redpanda config: %s\n", redpandaConfig)
			GinkgoWriter.Printf("Cleanup policy: %s\n", cleanupPolicy)
			GinkgoWriter.Printf("Error: %v\n", err)
			return err == nil && err2 == nil && redpandaConfig == fmt.Sprintf("%d", compactionTime) && cleanupPolicy == "compact"
		}, 20*time.Second, 1*time.Second).Should(BeTrue(), "Redpanda config should be updated")

		By("Waiting for Redpanda to restart and apply new config")
		// Wait for metrics to become available again after restart
		Eventually(func() bool {
			resp, err := httpGetWithTimeout(GetMetricsURL(), 1*time.Second)
			return err == nil && resp == 200
		}, 20*time.Second, 1*time.Second).Should(BeTrue(), "Metrics endpoint should be healthy after config update")

		// Wait for messages to be produced again after restart
		Eventually(func() bool {
			newOffset, err := checkRPK(topicName, lastOffset, lastTimestamp, 0.1, 0.2, messagesPerSecond)
			lastOffset = newOffset
			lastTimestamp = time.Now()
			return err == nil && newOffset != -1
		}, 5*time.Second, 1*time.Second).Should(BeTrue(), "Messages should be produced after config update")

		By("Verifying messages continue to be produced after config update")
		failure := false
		for i := 0; i < 0; i++ {
			failure = false
			startTime := time.Now()
			for time.Since(startTime) < testDuration {
				time.Sleep(1 * time.Second)
				newOffset, err := checkRPK(topicName, lastOffset, lastTimestamp, 0.1, 0.2, messagesPerSecond)
				if err != nil {
					GinkgoWriter.Printf("Error: %v\n", err)
					failure = true
					break
				}
				lastOffset = newOffset
				lastTimestamp = time.Now()
			}
			if failure == false {
				break
			}
			time.Sleep(1 * time.Second)
		}

		By("Ensuring that the state of redpanda is healthy")
		redpandaState, err := checkRedpandaState(GetMetricsURL(), 1*time.Second)
		Expect(err).ToNot(HaveOccurred())
		Expect(redpandaState).To(BeNumerically("==", 3))

		By("Checking if the config has not been changed back")
		Eventually(func() bool {
			redpandaConfig, err := getRedpandaConfig("log_retention_ms")
			cleanupPolicy, err2 := getRedpandaConfig("log_cleanup_policy")
			GinkgoWriter.Printf("Redpanda config: %s\n", redpandaConfig)
			GinkgoWriter.Printf("Cleanup policy: %s\n", cleanupPolicy)
			GinkgoWriter.Printf("Error: %v\n", err)
			return err == nil && err2 == nil && redpandaConfig == fmt.Sprintf("%d", compactionTime) && cleanupPolicy == "compact"
		}, 5*time.Second, 1*time.Second).Should(BeTrue(), "Redpanda config should not be changed back")

		// Now we disable the producer, and wait for the compaction to happen (e.g waiting 1 minute)
		By("Stopping the producer")
		builder.StopDataFlowComponent("dfc-cleanup-policy")
		newYaml := builder.BuildYAML()

		Expect(writeConfigFile(newYaml, getContainerName())).To(Succeed())

		By("Waiting for the compaction to happen")

		Eventually(func() bool {
			messages, err := getRPKSample(topicName)
			GinkgoWriter.Printf("Messages: %v\n", messages)
			return err == nil && len(messages) == 1
		}, 120*time.Second, 1*time.Second).Should(BeTrue(), "Exactly 1 message should be produced")

	})
})
