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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
)

func init() {
	// Use corev1 encoder for best performance and compatibility
	encoding.ChooseEncoder(encoding.EncodingCorev1)
}

var _ = Describe("UMH Core E2E Communication", Ordered, Label("e2e"), func() {
	var (
		mockServer    *MockAPIServer
		containerName string
		metricsPort   int
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(context.Background(), 10*time.Minute)

		By("Starting mock API server")
		mockServer = NewMockAPIServer()
		Expect(mockServer.Start()).To(Succeed())

		By("Starting UMH Core container with mock API")
		containerName, metricsPort = startUMHCoreWithMockAPI(mockServer)

		By("Waiting for container to be healthy and connected")
		Eventually(func() bool {
			return isContainerHealthy(metricsPort)
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Container should be healthy")

		By("Starting background subscription sender")
		go startPeriodicSubscription(testCtx, mockServer, 10*time.Second)

		DeferCleanup(func() {
			By("Cleaning up container and server")
			if containerName != "" {
				stopAndRemoveContainer(containerName)
			}
			if mockServer != nil {
				mockServer.Stop()
			}
			By("Stopping test context")
			testCancel()

		})
	})

	Context("Component Status Verification", func() {
		It("should verify component health status from status messages", func() {
			// Give umh-core time to process the initial subscription
			time.Sleep(5 * time.Second)

			// Use the modular health check function
			waitForComponentsHealthy(mockServer)
		})
	})

	Context("Bridge Creation", func() {
		It("should create a bridge with benthos generate and UNS output", func() {
			testBridgeCreation(mockServer)
		})
	})

	Context("Data Model Lifecycle", func() {
		It("should create, edit, and delete a data model with automatic data contract creation", func() {
			testDataModelLifecycle(mockServer)
		})
	})
})
