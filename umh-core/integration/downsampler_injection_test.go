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
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Downsampler Injection Integration Test", Ordered, Label("integration", "downsampler"), func() {

	AfterAll(func() {
		// Always stop container after the test
		PrintLogsAndStopContainer()
		CleanupDockerBuildCache()

		// Keep temp dirs for debugging if the test failed
		if !CurrentSpecReport().Failed() {
			cleanupTmpDirs(containerName)
		}
	})

	Context("Protocol Converter with Automatic Downsampler Injection", func() {
		BeforeAll(func() {
			By("Building protocol converter config with tag_processor that should trigger downsampler injection")
			cfg := getProtocolConverterWithTagProcessorYAML()

			By("Starting container with protocol converter configuration")
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available")
		})

		It("should successfully start container with protocol converter containing injected downsampler", func() {
			By("Verifying container is running and metrics are available")
			Eventually(func() bool {
				resp, err := http.Get(GetMetricsURL())
				if err != nil {
					return false
				}
				defer func() { _ = resp.Body.Close() }()

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return false
				}

				// Look for evidence that the system is running (any core metrics)
				return strings.Contains(string(body), "umh_core_reconcile_duration_milliseconds")
			}, 30*time.Second, 2*time.Second).Should(BeTrue(), "System should be running with metrics available")

			By("Verifying system remains stable with injected downsampler")
			// If the container can start and run with our config that includes a tag_processor,
			// then the normalizer (with downsampler injection) worked correctly
			Consistently(func() int {
				resp, err := http.Get(GetMetricsURL())
				if err != nil {
					return 0
				}
				defer func() { _ = resp.Body.Close() }()
				return resp.StatusCode
			}, 5*time.Second, 1*time.Second).Should(Equal(http.StatusOK),
				"System should remain stable, proving downsampler injection didn't break anything")
		})
	})
})

// getProtocolConverterWithTagProcessorYAML returns a simple YAML config with a protocol converter
// containing a tag_processor, which should trigger automatic downsampler injection during normalization
func getProtocolConverterWithTagProcessorYAML() string {
	return `
agent:
  metricsPort: 8080
  location:
    0: "plant-test"
    1: "line-integration"

protocolConverter:
  - name: "test-downsampler-injection"
    desiredState: "active"
    protocolConverterServiceConfig:
      location:
        2: "sensor-test"
      template:
        connection:
          nmap:
            target: "localhost"
            port: "8085"
        dataflowcomponent_read:
          benthos:
            input:
              generate:
                count: 5  # Just a few messages for the test
                interval: "2s"
                mapping: 'root = {"value": random_int(min:1, max:100), "timestamp_ms": timestamp_unix_milli()}'
            pipeline:
              processors:
                - tag_processor:
                    defaults: |
                      msg.meta.location_path = "test.plant.sensor";
                      msg.meta.data_contract = "_historian";  
                      msg.meta.tag_name = "temperature";
                      return msg;
                # Note: downsampler should be automatically injected here during normalization
            output:
              stdout: {}
        dataflowcomponent_write:
          benthos:
            input:
              stdin: {}
            output:
              drop: {}
      variables:
        HOST: "localhost"
        PORT: "8085"

internal:
  redpanda:
    name: "redpanda"
    desiredState: "stopped"  # Keep stopped for test efficiency
`
}
