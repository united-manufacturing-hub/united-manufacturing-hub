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

package connectionserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
)

var _ = Describe("Nmap YAML Generator", func() {
	type testCase struct {
		config      *ConnectionServiceConfig
		expected    []string
		notExpected []string
	}

	DescribeTable("generator rendering",
		func(tc testCase) {
			generator := NewGenerator()
			yamlStr, err := generator.RenderConfig(*tc.config)
			Expect(err).NotTo(HaveOccurred())

			// Check for expected strings
			for _, exp := range tc.expected {
				Expect(yamlStr).To(ContainSubstring(exp))
			}

			// Check for strings that should not be present
			for _, notExp := range tc.notExpected {
				Expect(yamlStr).NotTo(ContainSubstring(notExp))
			}
		},
		Entry("should render empty port correctly",
			testCase{
				config: &ConnectionServiceConfig{
					NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
						Target: "127.0.0.1",
					},
				},
				expected: []string{
					"nmap:",
					"  target:",
				},
				notExpected: []string{
					"  port",
				},
			}),
		Entry("should render empty target correctly",
			testCase{
				config: &ConnectionServiceConfig{
					NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
						Port: 443,
					},
				},
				expected: []string{
					"nmap:",
					"  port: 443",
				},
				notExpected: []string{
					"  target:",
				},
			}),
	)

	// Add package-level function test
	Describe("RenderNmapYAML package function", func() {
		It("should produce the same output as the Generator", func() {
			// Setup test config
			target := "127.0.0.1"
			port := 443
			nmapConfig := nmapserviceconfig.NmapServiceConfig{
				Target: target,
				Port:   uint16(port),
			}

			// Use package-level function
			yamlStr1, err := RenderConnectionYAML(
				nmapConfig,
			)
			Expect(err).NotTo(HaveOccurred())

			// Use Generator directly
			cfg := ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: target,
					Port:   uint16(port),
				},
			}
			generator := NewGenerator()
			yamlStr2, err := generator.RenderConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Both should produce the same result
			Expect(yamlStr1).To(Equal(yamlStr2))
		})
	})
})
