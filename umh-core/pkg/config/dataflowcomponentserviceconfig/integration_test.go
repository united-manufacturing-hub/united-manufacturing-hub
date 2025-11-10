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

package dataflowcomponentserviceconfig_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

var _ = Describe("DataflowComponentServiceConfig Integration", func() {
	Context("debug_level flag", func() {
		It("should enable OPC_DEBUG when debug_level is true", func() {
			// Test standalone data flow with debug_level: true
			dfc := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: true,
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"generate": map[string]interface{}{
							"mapping": `root = "test"`,
						},
					},
					Output: map[string]interface{}{
						"drop": map[string]interface{}{},
					},
				},
			}

			// 1. Verify DebugLevel flag set
			Expect(dfc.DebugLevel).To(BeTrue())

			// 2. Convert to BenthosServiceConfig
			benthosConfig := dfc.GetBenthosServiceConfig()

			// 3. Verify LogLevel set to DEBUG
			Expect(benthosConfig.DebugLevel).To(BeTrue())

			// 4. Generate S6 config
			benthosService := benthos.NewDefaultBenthosService("integration-test")
			s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "test-dataflow")
			Expect(err).NotTo(HaveOccurred())

			// 5. Verify OPC_DEBUG environment variable set
			Expect(s6Config.Env["OPC_DEBUG"]).To(Equal("debug"))
		})

		It("should not set OPC_DEBUG when debug_level is false", func() {
			// Test standalone data flow with debug_level: false
			dfc := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: false,
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"generate": map[string]interface{}{
							"mapping": `root = "test"`,
						},
					},
					Output: map[string]interface{}{
						"drop": map[string]interface{}{},
					},
				},
			}

			// 1. Verify DebugLevel flag set to false
			Expect(dfc.DebugLevel).To(BeFalse())

			// 2. Convert to BenthosServiceConfig
			benthosConfig := dfc.GetBenthosServiceConfig()

			// 3. Verify LogLevel set to INFO (default)
			Expect(benthosConfig.DebugLevel).To(BeFalse())

			// 4. Generate S6 config
			benthosService := benthos.NewDefaultBenthosService("integration-test")
			s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "test-dataflow")
			Expect(err).NotTo(HaveOccurred())

			// 5. Verify OPC_DEBUG environment variable NOT set
			_, opcDebugExists := s6Config.Env["OPC_DEBUG"]
			Expect(opcDebugExists).To(BeFalse(), "OPC_DEBUG should not be set when LogLevel is INFO")
		})

		It("should default to INFO when debug_level is omitted", func() {
			// Test standalone data flow without debug_level field (should default to false/INFO)
			dfc := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"generate": map[string]interface{}{
							"mapping": `root = "test"`,
						},
					},
					Output: map[string]interface{}{
						"drop": map[string]interface{}{},
					},
				},
			}

			// 1. Verify DebugLevel defaults to false
			Expect(dfc.DebugLevel).To(BeFalse())

			// 2. Convert to BenthosServiceConfig
			benthosConfig := dfc.GetBenthosServiceConfig()

			// 3. Verify LogLevel set to INFO (default)
			Expect(benthosConfig.DebugLevel).To(BeFalse())

			// 4. Generate S6 config
			benthosService := benthos.NewDefaultBenthosService("integration-test")
			s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "test-dataflow")
			Expect(err).NotTo(HaveOccurred())

			// 5. Verify OPC_DEBUG environment variable NOT set
			_, opcDebugExists := s6Config.Env["OPC_DEBUG"]
			Expect(opcDebugExists).To(BeFalse(), "OPC_DEBUG should not be set when LogLevel is INFO")
		})
	})

	Context("LogLevel mapping", func() {
		DescribeTable("mapping debug_level boolean to LogLevel string",
			func(debugLevel bool, expectedDebugLevel bool, expectOpcDebugSet bool, expectedOpcDebug string) {
				// Create data flow component with programmatic debug_level
				dfc := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					DebugLevel: debugLevel,
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]interface{}{
							"generate": map[string]interface{}{
								"mapping": `root = "test"`,
							},
						},
						Output: map[string]interface{}{
							"drop": map[string]interface{}{},
						},
					},
				}

				// Convert to BenthosServiceConfig
				benthosConfig := dfc.GetBenthosServiceConfig()
				Expect(benthosConfig.DebugLevel).To(BeEquivalentTo(expectedDebugLevel))

				// Generate S6 config
				benthosService := benthos.NewDefaultBenthosService("integration-test")
				s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "test-dataflow")
				Expect(err).NotTo(HaveOccurred())

				// Verify OPC_DEBUG environment variable
				opcDebugValue, opcDebugExists := s6Config.Env["OPC_DEBUG"]
				Expect(opcDebugExists).To(Equal(expectOpcDebugSet), "OPC_DEBUG existence should match expected")

				if expectOpcDebugSet {
					Expect(opcDebugValue).To(Equal(expectedOpcDebug), "OPC_DEBUG value should match expected")
				}
			},
			Entry("debug_level true maps to LogLevel DEBUG",
				true, true, true, "debug"),
			Entry("debug_level false maps to LogLevel INFO",
				false, false, false, ""),
		)
	})
})
