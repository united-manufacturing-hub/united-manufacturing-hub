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
	"context"
	"fmt"
	"strconv"
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
		const testDuration = 60 * time.Minute
		const monitorHealthInterval = 30 * time.Second
		const lossTolerance = 0.05 // Allow for 5% message loss
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
				builder.AddBenthosProducer(fmt.Sprintf("benthos-%d", i), "1s", testTopic)
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
				newOffset, err := checkRPK(testTopic, lastLoopOffset, lastLoopTimestamp, lossTolerance, messagesPerSecond)
				if err != nil {
					Fail(fmt.Sprintf("RPK check failed: %v", err))
				}
				lastLoopOffset = newOffset
				lastLoopTimestamp = time.Now()
				time.Sleep(monitorHealthInterval)
			}

			By("Verifying message count with rpk")
			messageCount, err := checkRPK(testTopic, lastLoopOffset, lastLoopTimestamp, lossTolerance, messagesPerSecond)
			if err != nil {
				Fail(fmt.Sprintf("RPK check failed: %v", err))
			}

			// Calculate expected message count with a tolerance of 20% loss
			totalSeconds := int(testDuration.Seconds())
			expectedMessagesPerSecond := 10 * messagesPerSecond // 10 producers * 10 messages per second
			expectedMessages := totalSeconds * expectedMessagesPerSecond
			minimumExpectedMessages := int(float64(expectedMessages) * (1 - lossTolerance))

			GinkgoWriter.Printf("\n=== Message Count Results ===\n")
			GinkgoWriter.Printf("Test duration: %v (%d seconds)\n", testDuration, totalSeconds)
			GinkgoWriter.Printf("Producers: 10, each sending %d messages per second\n", messagesPerSecond)
			GinkgoWriter.Printf("Expected messages: %d\n", expectedMessages)
			GinkgoWriter.Printf("Minimum expected messages (%d%%): %d\n", 100-int(lossTolerance*100), minimumExpectedMessages)
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
				fmt.Sprintf("Should receive at least %d messages (%d%% of expected %d)",
					minimumExpectedMessages, 100-int(lossTolerance*100), expectedMessages))
		})
	})
})

func checkRPK(topic string, lastLoopOffset int, lastLoopTimestamp time.Time, lossToleranceWarning float64, messagesPerSecond int) (newOffset int, err error) {
	// Fetch the latest Messages and validate that:
	// 1) The timestamp is within reason (+-1m)
	// 2) The offsets are increasing (e.g the last offset of the batch is higher then the current one)

	messages, err := getRPKSample(topic)
	if err != nil {
		return 0, err
	}

	var lastTimestamp int64
	if len(messages) > 0 {
		newOffset = int(messages[len(messages)-1].Offset)
		lastTimestamp = messages[len(messages)-1].Timestamp
	} else {
		Fail("❌ No messages received")
	}

	// 1. Check timestamp

	// Parse lastTimestamp from unix milli
	lastTime := time.UnixMilli(lastTimestamp)
	// Redpanda logs in UTC, so we need to convert to local time
	lastTime = lastTime.UTC()
	currentNow := time.Now().UTC()
	timeDifference := lastTime.Sub(currentNow)
	if lastTime.Before(currentNow.Add(-1 * time.Minute)) {
		Fail(fmt.Sprintf("❌ Timestamp is too old: %s (%dms)", lastTime, lastTime.Sub(currentNow).Milliseconds()))
	} else if lastTime.After(currentNow.Add(1 * time.Minute)) {
		Fail(fmt.Sprintf("❌ Timestamp is too new: %s (%dms)", lastTime, lastTime.Sub(currentNow).Milliseconds()))
	}

	GinkgoWriter.Printf("✅ Timestamp is within reason: %s (time difference: %s)\n", lastTime, timeDifference)

	// 2. Check offset
	if newOffset <= lastLoopOffset {
		Fail(fmt.Sprintf("Offset is not increasing: %d <= %d", newOffset, lastLoopOffset))
	}

	GinkgoWriter.Printf("✅ Offset is increasing: %d > %d\n", newOffset, lastLoopOffset)

	// 3. Calculate msg per sec based on now and lastlooptimestamp
	elapsedTime := time.Since(lastLoopTimestamp)
	msgPerSec := float64(newOffset-lastLoopOffset) / elapsedTime.Seconds()

	if msgPerSec <= 0 {
		Fail("❌ Msg per sec is not positive")
	}
	if msgPerSec < 9 {
		Fail(fmt.Sprintf("❌ Msg per sec is too low: %f\n", msgPerSec))
	} else {
		// Let's warn (but not fail) if we are below the loss tolerance (use a nice warning signal)
		if msgPerSec < float64(messagesPerSecond)*(1-lossToleranceWarning) {
			GinkgoWriter.Printf("⚠️ Msg per sec is below the loss tolerance: %f\n", msgPerSec)
		} else {
			GinkgoWriter.Printf("✅ Msg per sec: %f\n", msgPerSec)
		}
	}

	return newOffset, nil
}

type Message struct {
	Timestamp int64
	Topic     string
	Offset    int64
}

// getRPKSample connects to the redpanda cluster, and returns the last 10 messages from the given topic
func getRPKSample(topic string) ([]Message, error) {
	ctx, cncl := context.WithTimeout(context.Background(), 1*time.Second)
	defer cncl()
	out, err := runDockerCommandWithCtx(ctx, "exec", getContainerName(), "/opt/redpanda/bin/rpk", "topic", "consume", topic, "--offset", "-10", "-n", "10", "--format", "%d:%t:%o\n")
	if err != nil {
		return nil, err
	}
	/*
		ddc913c11978:/# /opt/redpanda/bin/rpk topic consume hello-world-topic --offset -10 -n 10 --format "%d:%t:%o\n"
		1744631802928:hello-world-topic:661
		1744631802943:hello-world-topic:662
		1744631803929:hello-world-topic:663
		1744631803946:hello-world-topic:664
		1744631804933:hello-world-topic:665
		1744631804949:hello-world-topic:666
		1744631805938:hello-world-topic:667
		1744631805949:hello-world-topic:668
		1744631806940:hello-world-topic:669
		1744631806950:hello-world-topic:670
	*/

	// Parse the output
	lines := strings.Split(out, "\n")

	messages := make([]Message, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}

		splits := strings.Split(line, ":")
		if len(splits) != 3 {
			return nil, fmt.Errorf("invalid line: %s", line)
		}

		timestamp, err := strconv.ParseInt(splits[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp: %s", splits[0])
		}

		offset, err := strconv.ParseInt(splits[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid offset: %s", splits[2])
		}

		messages = append(messages, Message{
			Timestamp: timestamp,
			Topic:     splits[1],
			Offset:    offset,
		})
	}

	return messages, nil
}
