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

var _ = FDescribe("Redpanda Config Change Test", Ordered, Label("integration"), func() {
	const (
		topicName            = "compression-test-topic"
		messagesPerSecond    = 5
		testDuration         = 1 * time.Minute
		compressionWaitTime  = 10 * time.Second
		lossToleranceWarning = 0.1 // 10% message loss
		lossToleranceFail    = 0.2 // 20% message loss
	)

	var lastOffset = -1
	var lastTimestamp time.Time

	BeforeAll(func() {
		By("Starting umh-core with Redpanda and a DFC that outputs to Kafka")
		builder := NewDataFlowComponentBuilder()
		builder.full.Internal.Redpanda.DesiredFSMState = "active"
		builder.full.Internal.Redpanda.Name = "redpanda"
		builder.AddGeneratorDataFlowComponentToKafka("dfc-compression", fmt.Sprintf("%dms", 1000/messagesPerSecond), topicName)
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

	It("should continue processing messages after changing compression type to lz4", func() {
		By("Waiting for initial messages to be produced")
		Eventually(func() bool {
			newOffset, err := checkRPK(topicName, lastOffset, lastTimestamp, lossToleranceWarning, lossToleranceFail, messagesPerSecond)
			lastOffset = newOffset
			lastTimestamp = time.Now()
			return err == nil && newOffset != -1
		}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Messages should be produced initially")

		By("Changing compression type to lz4")
		builder := NewDataFlowComponentBuilder()
		builder.full.Internal.Redpanda.DesiredFSMState = "active"
		builder.full.Internal.Redpanda.Name = "redpanda"
		builder.full.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicCompressionType = "lz4"
		builder.AddGeneratorDataFlowComponentToKafka("dfc-compression", fmt.Sprintf("%dms", 1000/messagesPerSecond), topicName)
		cfg := builder.BuildYAML()
		Expect(writeConfigFile(cfg)).To(Succeed())

		By("Waiting for compression change to take effect")
		time.Sleep(compressionWaitTime)

		By("Verifying messages continue to be processed after compression change")
		startTime := time.Now()
		lastTimestamp = time.Now()
		for time.Since(startTime) < testDuration {
			time.Sleep(1 * time.Second)
			newOffset, err := checkRPK(topicName, lastOffset, lastTimestamp, lossToleranceWarning, lossToleranceFail, messagesPerSecond)
			Expect(err).ToNot(HaveOccurred(), "Should continue processing messages after compression change")
			lastOffset = newOffset
			lastTimestamp = time.Now()
		}
	})
})
