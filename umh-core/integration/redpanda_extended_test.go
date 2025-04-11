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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Redpanda Extended Tests", Ordered, Label("redpanda-extended"), func() {

	AfterAll(func() {
		// Always stop container after the entire suite
		PrintLogsAndStopContainer()
		CleanupDockerBuildCache()

		//Keep temp dirs for debugging if the test failed
		if !CurrentSpecReport().Failed() {
			cleanupTmpDirs(containerName)
		}
	})

	Context("with redpanda and 10 high-frequency benthos producers", func() {

		// Add 10 benthos services that will produce messages to the "test-throughput" topic
		const messagesPerSecond = 10 // Each benthos instance will produce 10 messages per second
		const testDuration = 5 * time.Minute
		const monitorHealthInterval = 30 * time.Second
		const maxErrorsInARow = 10
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

			for i := 0; i < 10; i++ {
				builder.AddBenthosProducer(fmt.Sprintf("benthos-%d", i), fmt.Sprintf("%ds", 1+i%3), testTopic)
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
			errorsInARow := 0
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
				err := reportOnMetricsHealthIssue()
				if err != nil {
					errorsInARow++
					GinkgoWriter.Printf("[%d] Error: %v\n", errorsInARow, err)
				} else {
					errorsInARow = 0
				}
				if errorsInARow >= maxErrorsInARow {
					Fail("System is unstable, too many errors in a row")
				}

				time.Sleep(monitorHealthInterval)
			}

			By("Verifying message count with rpk")
			// First check if the topic exists
			out, err := runDockerCommand("exec", getContainerName(), "/opt/redpanda/bin/rpk", "topic", "list")
			Expect(err).NotTo(HaveOccurred(), "Should be able to list topics with rpk")
			GinkgoWriter.Printf("Available topics:\n%s\n", out)

			// Count the messages by using rpk to consume all messages and pipe through wc -l
			out, err = runDockerCommand("exec", getContainerName(), "bash", "-c",
				fmt.Sprintf("/opt/redpanda/bin/rpk topic consume %s --offset earliest -n -1 2>/dev/null | wc -l", testTopic))
			Expect(err).NotTo(HaveOccurred(), "Should be able to count messages with rpk")

			// Parse message count
			messageCount := 0
			_, err = fmt.Sscanf(strings.TrimSpace(out), "%d", &messageCount)
			Expect(err).NotTo(HaveOccurred(), "Should be able to parse message count")

			// Calculate expected message count with a tolerance of 20% loss
			totalSeconds := int(testDuration.Seconds())
			expectedMessagesPerSecond := 10 * messagesPerSecond // 10 producers * 10 messages per second
			expectedMessages := totalSeconds * expectedMessagesPerSecond
			minimumExpectedMessages := int(float64(expectedMessages) * 0.8) // Allow for 20% message loss

			GinkgoWriter.Printf("\n=== Message Count Results ===\n")
			GinkgoWriter.Printf("Test duration: %v (%d seconds)\n", testDuration, totalSeconds)
			GinkgoWriter.Printf("Producers: 10, each sending %d messages per second\n", messagesPerSecond)
			GinkgoWriter.Printf("Expected messages: %d\n", expectedMessages)
			GinkgoWriter.Printf("Minimum expected messages (80%%): %d\n", minimumExpectedMessages)
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
				fmt.Sprintf("Should receive at least %d messages (80%% of expected %d)",
					minimumExpectedMessages, expectedMessages))
		})
	})
})
