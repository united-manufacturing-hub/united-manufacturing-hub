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
// integration_test.go

package integration_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Redpanda Extended Tests (burst)", Ordered, Label("redpanda-extended"), func() {

	AfterAll(func() {
		// Always stop container after the entire suite
		PrintLogsAndStopContainer()
		CleanupDockerBuildCache()

		// Keep temp dirs for debugging if the test failed
		if !CurrentSpecReport().Failed() {
			cleanupTmpDirs(containerName)
		}
	})

	Context("with redpanda and 15 high-frequency benthos producers", func() {

		// Add 15 benthos services that will produce messages to the "test-throughput" topic
		const messagesPerSecond = 100 // Each benthos instance will produce 100 messages per second
		const testDuration = 10 * time.Minute
		const monitorHealthInterval = 30 * time.Second
		const lossToleranceWarning = 0.025 // Warn on 2.5% message loss
		const lossToleranceFail = 0.05     // Fail on 5% message loss
		const producers = 15
		BeforeAll(func() {
			By("Starting with an empty configuration")
			cfg := NewBuilder().BuildYAML()
			// Write the empty config and start the container
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		AfterAll(func() {
			By("Stopping the container after the redpanda-benthos throughput test")
			PrintLogsAndStopContainer()
		})

		It("should produce messages at high frequency and maintain system stability", func() {
			By("Configuring redpanda and benthos producers")
			// Set up redpanda with benthos producers
			builder := NewRedpandaBuilder()
			builder.AddGoldenRedpanda()
			testTopic := "test-throughput"

			for i := range producers {
				builder.AddBenthosProducer(fmt.Sprintf("benthos-%d", i), fmt.Sprintf("%dms", 1000/messagesPerSecond), testTopic)
			}

			// Apply configuration
			cfg := builder.BuildYAML()
			Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

			By("Waiting for services to stabilize")
			// Allow time for redpanda and benthos services to start
			time.Sleep(30 * time.Second)

			By("Monitoring system health during test run")
			startTime := time.Now()
			testEndTime := startTime.Add(testDuration)

			// Monitor the health of the system at regular intervals
			lastLoopOffset := -1
			lastLoopTimestamp := time.Now()
			for time.Now().Before(testEndTime) {
				// Print status update
				elapsedDuration := time.Since(startTime).Round(time.Second)
				remainingDuration := time.Until(testEndTime).Round(time.Second)

				GinkgoWriter.Printf(
					"\n=== Redpanda-Benthos Throughput Test Status ===\n"+
						"Elapsed time: %v\n"+
						"Remaining time: %v\n",
					elapsedDuration, remainingDuration)

				// Check system health
				err := reportOnMetricsHealthIssue(false, true)
				if err != nil {
					Fail(fmt.Sprintf("System is unstable: %v", err))
				}
				newOffset, err := checkRPK(testTopic, lastLoopOffset, lastLoopTimestamp, lossToleranceWarning, lossToleranceFail, producers*messagesPerSecond)
				if err != nil {
					Fail(fmt.Sprintf("RPK check failed: %v", err))
				}
				lastLoopOffset = newOffset
				lastLoopTimestamp = time.Now()
				time.Sleep(monitorHealthInterval)
			}

			By("Verifying message count with rpk")
			messageCount, err := checkRPK(testTopic, lastLoopOffset, lastLoopTimestamp, lossToleranceWarning, lossToleranceFail, producers*messagesPerSecond)
			if err != nil {
				Fail(fmt.Sprintf("RPK check failed: %v", err))
			}

			// Calculate expected message count the given loss tolerance
			totalSeconds := int(time.Since(startTime).Seconds())
			expectedMessagesPerSecond := producers * messagesPerSecond // 15 producers * 10 messages per second
			expectedMessages := totalSeconds * expectedMessagesPerSecond
			minimumExpectedMessages := int(float64(expectedMessages) * (1 - lossToleranceWarning))

			GinkgoWriter.Printf("\n=== Message Count Results ===\n")
			GinkgoWriter.Printf("Test duration: %v (%d seconds)\n", testDuration, totalSeconds)
			GinkgoWriter.Printf("Producers: %d, each sending %d messages per second\n", producers, messagesPerSecond)
			GinkgoWriter.Printf("Expected messages: %d\n", expectedMessages)
			GinkgoWriter.Printf("Minimum expected messages (%d%%): %d\n", 100-int(lossToleranceFail*100), minimumExpectedMessages)
			GinkgoWriter.Printf("Actual messages received: %d\n", messageCount)

			if messageCount >= minimumExpectedMessages {
				GinkgoWriter.Printf("SUCCESS: Received sufficient messages (%d >= %d)\n",
					messageCount, minimumExpectedMessages)
			} else {
				GinkgoWriter.Printf("WARNING: Received fewer messages than expected (%d < %d)\n",
					messageCount, minimumExpectedMessages)
			}

			// Assert that we received at least the minimum expected messages
			Expect(messageCount).To(BeNumerically(">=", minimumExpectedMessages),
				fmt.Sprintf("Should receive at least %d messages (%d percent of expected %d)",
					minimumExpectedMessages, 100-int(lossToleranceFail*100), expectedMessages))
		})
	})
})
