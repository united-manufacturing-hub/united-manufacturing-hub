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
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DataFlowComponent Restart Integration Test", Ordered, Label("integration"), func() {
	const (
		topicName            = "dfc-restart-test-topic"
		messagesPerSecond    = 5
		testDuration         = 1 * time.Minute
		postRestartWait      = 5 * time.Second
		containerDownWait    = 30 * time.Second
		lossToleranceWarning = 0.1 // 10% message loss
		lossToleranceFail    = 0.2 // 20% message loss
	)

	type restartMode string
	const (
		gracefulRestart restartMode = "graceful"
		killRestart     restartMode = "kill"
	)

	restartActions := map[restartMode]func(){
		gracefulRestart: func() {
			By("Stopping the container (graceful)")
			out, err := runDockerCommand("stop", getContainerName())
			GinkgoWriter.Printf("Stopped container: %s\n", out)
			Expect(err).ToNot(HaveOccurred())
		},
		killRestart: func() {
			By("Killing the container (SIGKILL)")
			out, err := runDockerCommand("kill", getContainerName())
			GinkgoWriter.Printf("Killed container: %s\n", out)
			Expect(err).ToNot(HaveOccurred())
		},
	}

	var lastOffset = -1
	var lastTimestamp time.Time

	BeforeAll(func() {
		By("Starting umh-core with Redpanda and a DFC that outputs to Kafka")
		builder := NewDataFlowComponentBuilder()
		builder.full.Internal.Redpanda.DesiredFSMState = "active"
		builder.full.Internal.Redpanda.Name = "redpanda"
		builder.AddGeneratorDataFlowComponentToKafka("dfc-restart", fmt.Sprintf("%dms", 1000/messagesPerSecond), topicName)
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

	DescribeTable("should produce messages, survive a restart, and produce messages again",
		func(mode restartMode) {
			// Wait for the first checkRPK to return a result
			Eventually(func() bool {
				newOffset, err := checkRPK(topicName, lastOffset, lastTimestamp, lossToleranceWarning, lossToleranceFail, messagesPerSecond)
				lastOffset = newOffset
				lastTimestamp = time.Now()
				return err == nil && newOffset != -1
			}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Messages should be produced after restart")

			By("Waiting for messages to be produced before restart")
			startTime := time.Now()
			lastTimestamp = time.Now()
			for time.Since(startTime) < testDuration {
				time.Sleep(1 * time.Second)
				newOffset, err := checkRPK(topicName, lastOffset, lastTimestamp, lossToleranceWarning, lossToleranceFail, messagesPerSecond)
				Expect(err).ToNot(HaveOccurred())
				lastOffset = newOffset
				lastTimestamp = time.Now()
			}

			restartActions[mode]()

			By("Validating that the container is stopped (via docker ps)")
			out, err := runDockerCommand("ps", "-a", "--format", "{{.Names}} {{.ID}} {{.Status}}")
			GinkgoWriter.Printf("Docker ps: %s\n", out)
			Expect(err).ToNot(HaveOccurred())
			lines := strings.Split(out, "\n")
			var containerLine string
			for _, line := range lines {
				if strings.Contains(line, getContainerName()) {
					containerLine = line
					break
				}
			}
			Expect(containerLine).To(ContainSubstring("Exited"))

			By(fmt.Sprintf("Waiting %s before restart", containerDownWait))
			for i := 0; i < int(containerDownWait/time.Second); i++ {
				time.Sleep(time.Second)
				GinkgoWriter.Printf("Waiting until we can restart: %d seconds\n", int(containerDownWait.Seconds())-i)
			}

			By("Starting the container again")
			out, err = runDockerCommand("start", getContainerName())
			GinkgoWriter.Printf("Started container: %s\n", out)
			Expect(err).ToNot(HaveOccurred())

			By(fmt.Sprintf("Waiting %s for container to become healthy", postRestartWait))
			time.Sleep(postRestartWait)
			Eventually(func() bool {
				resp, err := httpGetWithTimeout(GetMetricsURL(), 1*time.Second)
				return err == nil && resp == 200
			}, 20*time.Second, 1*time.Second).Should(BeTrue(), "Metrics endpoint should be healthy after restart")

			// Wait for the first checkRPK to return a result
			Eventually(func() bool {
				// We set the offset to -1, to prevent it from hard failing if no messages are yet produced
				// We also don't care about the timestamp, because we will check it later
				newOffset, err := checkRPK(topicName, -1, lastTimestamp, lossToleranceWarning, lossToleranceFail, messagesPerSecond)
				return err == nil && newOffset != -1
			}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Messages should be produced after restart")

			By("Validating messages are being produced again after restart")
			startTime = time.Now()
			lastTimestamp = time.Now()
			for time.Since(startTime) < testDuration {
				time.Sleep(1 * time.Second)
				newOffset, err := checkRPK(topicName, lastOffset, lastTimestamp, lossToleranceWarning, lossToleranceFail, messagesPerSecond)
				Expect(err).ToNot(HaveOccurred())
				lastOffset = newOffset
				lastTimestamp = time.Now()
			}

			By("Ensuring that the state of redpanda is healthy")
			redpandaState, err := checkRedpandaState(GetMetricsURL(), 1*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(redpandaState).To(BeNumerically("==", 3))

		},
		Entry("graceful restart (docker stop/start)", gracefulRestart),
		Entry("kill restart (docker kill/start)", killRestart),
	)
})

// httpGetWithTimeout is a helper for Eventually to check endpoint health
func httpGetWithTimeout(url string, timeout time.Duration) (int, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	err = resp.Body.Close()
	if err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}

func checkRedpandaState(url string, timeout time.Duration) (int, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = resp.Body.Close()
		return 0, err
	}
	err = resp.Body.Close()
	if err != nil {
		return 0, err
	}

	// Get the umh_core_service_current_state{component="redpanda_instance",instance="redpanda"}
	re := regexp.MustCompile(`umh_core_service_current_state{component="redpanda_instance",instance="redpanda"} (\d)`)
	matches := re.FindStringSubmatch(string(body))
	if len(matches) != 2 {
		return 0, fmt.Errorf("no match found for umh_core_service_current_state")
	}
	GinkgoWriter.Printf("Redpanda state: %s\n", matches[1])
	return strconv.Atoi(matches[1])
}
