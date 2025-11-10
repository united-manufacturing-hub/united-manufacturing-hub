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

package protocolconverterserviceconfig_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

var _ = Describe("ProtocolConverterServiceConfig Integration", func() {
	Context("debug_level flag", func() {
		It("should enable OPC_DEBUG when debug_level is true", func() {
			// Test the complete flow with debug_level: true
			yamlData := `
config:
  debug_level: true
  connection:
    name: "test-connection"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "test"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`

			// 1. Parse YAML to Spec
			var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

			err := yaml.Unmarshal([]byte(yamlData), &spec)
			Expect(err).NotTo(HaveOccurred())

			// 2. Verify debug_level parsed correctly
			Expect(spec.Config.DebugLevel).To(BeTrue())

			// 3. Convert Spec to Runtime
			runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
			Expect(err).NotTo(HaveOccurred())

			// 4. Verify debug_level preserved in Runtime
			Expect(runtime.DebugLevel).To(BeTrue())

			// 5. Extract DataflowComponentServiceConfigs
			readDFC := runtime.DataflowComponentReadServiceConfig
			writeDFC := runtime.DataflowComponentWriteServiceConfig

			// 6. Verify debug_level propagated to DataflowComponentServiceConfigs
			Expect(readDFC.DebugLevel).To(BeTrue())
			Expect(writeDFC.DebugLevel).To(BeTrue())

			// 7. Convert DataflowComponentServiceConfigs to BenthosServiceConfigs
			readBenthos := readDFC.GetBenthosServiceConfig()
			writeBenthos := writeDFC.GetBenthosServiceConfig()

			// 8. Verify LogLevel set to DEBUG in BenthosServiceConfigs
			Expect(readBenthos.DebugLevel).To(BeTrue())
			Expect(writeBenthos.DebugLevel).To(BeTrue())

			// 9. Generate S6 configs using BenthosService
			benthosService := benthos.NewDefaultBenthosService("integration-test")

			readS6, err := benthosService.GenerateS6ConfigForBenthos(&readBenthos, "test-read")
			Expect(err).NotTo(HaveOccurred())

			writeS6, err := benthosService.GenerateS6ConfigForBenthos(&writeBenthos, "test-write")
			Expect(err).NotTo(HaveOccurred())

			// 10. Verify OPC_DEBUG environment variable set in S6 configs
			Expect(readS6.Env["OPC_DEBUG"]).To(Equal("debug"))
			Expect(writeS6.Env["OPC_DEBUG"]).To(Equal("debug"))
		})

		It("should not set OPC_DEBUG when debug_level is false", func() {
			// Test the complete flow with debug_level: false
			yamlData := `
config:
  debug_level: false
  connection:
    name: "test-connection"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "test"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`

			// 1. Parse YAML to Spec
			var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

			err := yaml.Unmarshal([]byte(yamlData), &spec)
			Expect(err).NotTo(HaveOccurred())

			// 2. Verify debug_level parsed correctly
			Expect(spec.Config.DebugLevel).To(BeFalse())

			// 3. Convert Spec to Runtime
			runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
			Expect(err).NotTo(HaveOccurred())

			// 4. Verify debug_level preserved in Runtime
			Expect(runtime.DebugLevel).To(BeFalse())

			// 5. Extract DataflowComponentServiceConfigs
			readDFC := runtime.DataflowComponentReadServiceConfig
			writeDFC := runtime.DataflowComponentWriteServiceConfig

			// 6. Verify debug_level propagated to DataflowComponentServiceConfigs
			Expect(readDFC.DebugLevel).To(BeFalse())
			Expect(writeDFC.DebugLevel).To(BeFalse())

			// 7. Convert DataflowComponentServiceConfigs to BenthosServiceConfigs
			readBenthos := readDFC.GetBenthosServiceConfig()
			writeBenthos := writeDFC.GetBenthosServiceConfig()

			// 8. Verify LogLevel set to INFO (default) in BenthosServiceConfigs
			Expect(readBenthos.DebugLevel).To(BeFalse())
			Expect(writeBenthos.DebugLevel).To(BeFalse())

			// 9. Generate S6 configs using BenthosService
			benthosService := benthos.NewDefaultBenthosService("integration-test")

			readS6, err := benthosService.GenerateS6ConfigForBenthos(&readBenthos, "test-read")
			Expect(err).NotTo(HaveOccurred())

			writeS6, err := benthosService.GenerateS6ConfigForBenthos(&writeBenthos, "test-write")
			Expect(err).NotTo(HaveOccurred())

			// 10. Verify OPC_DEBUG environment variable NOT set in S6 configs
			_, readOpcDebugExists := readS6.Env["OPC_DEBUG"]
			_, writeOpcDebugExists := writeS6.Env["OPC_DEBUG"]

			Expect(readOpcDebugExists).To(BeFalse(), "OPC_DEBUG should not be set when LogLevel is INFO")
			Expect(writeOpcDebugExists).To(BeFalse(), "OPC_DEBUG should not be set when LogLevel is INFO")
		})

		It("should default to INFO when debug_level is omitted", func() {
			// Test the complete flow without debug_level field (should default to false/INFO)
			yamlData := `
config:
  connection:
    name: "test-connection"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "test"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`

			// 1. Parse YAML to Spec
			var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

			err := yaml.Unmarshal([]byte(yamlData), &spec)
			Expect(err).NotTo(HaveOccurred())

			// 2. Verify debug_level defaults to false
			Expect(spec.Config.DebugLevel).To(BeFalse())

			// 3. Convert Spec to Runtime
			runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
			Expect(err).NotTo(HaveOccurred())

			// 4. Verify debug_level preserved in Runtime
			Expect(runtime.DebugLevel).To(BeFalse())

			// 5. Extract DataflowComponentServiceConfigs
			readDFC := runtime.DataflowComponentReadServiceConfig
			writeDFC := runtime.DataflowComponentWriteServiceConfig

			// 6. Verify debug_level defaults to false in DataflowComponentServiceConfigs
			Expect(readDFC.DebugLevel).To(BeFalse())
			Expect(writeDFC.DebugLevel).To(BeFalse())

			// 7. Convert DataflowComponentServiceConfigs to BenthosServiceConfigs
			readBenthos := readDFC.GetBenthosServiceConfig()
			writeBenthos := writeDFC.GetBenthosServiceConfig()

			// 8. Verify LogLevel set to INFO (default) in BenthosServiceConfigs
			Expect(readBenthos.DebugLevel).To(BeFalse())
			Expect(writeBenthos.DebugLevel).To(BeFalse())

			// 9. Generate S6 configs using BenthosService
			benthosService := benthos.NewDefaultBenthosService("integration-test")

			readS6, err := benthosService.GenerateS6ConfigForBenthos(&readBenthos, "test-read")
			Expect(err).NotTo(HaveOccurred())

			writeS6, err := benthosService.GenerateS6ConfigForBenthos(&writeBenthos, "test-write")
			Expect(err).NotTo(HaveOccurred())

			// 10. Verify OPC_DEBUG environment variable NOT set in S6 configs
			_, readOpcDebugExists := readS6.Env["OPC_DEBUG"]
			_, writeOpcDebugExists := writeS6.Env["OPC_DEBUG"]

			Expect(readOpcDebugExists).To(BeFalse(), "OPC_DEBUG should not be set when LogLevel is INFO")
			Expect(writeOpcDebugExists).To(BeFalse(), "OPC_DEBUG should not be set when LogLevel is INFO")
		})
	})

	Context("integration chain verification", func() {
		var (
			spec    protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec
			runtime protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime
		)

		BeforeEach(func() {
			// Additional test to verify the complete integration chain explicitly
			yamlData := `
config:
  debug_level: true
  connection:
    name: "integration-test"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "integration"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`

			// Parse YAML
			err := yaml.Unmarshal([]byte(yamlData), &spec)
			Expect(err).NotTo(HaveOccurred())

			// Convert to Runtime
			runtime, err = protocolconverterserviceconfig.SpecToRuntime(spec)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("ReadDataflowComponent", func() {
			It("should propagate debug_level through the chain", func() {
				readDFC := runtime.DataflowComponentReadServiceConfig

				// Step 1: DebugLevel flag set
				Expect(readDFC.DebugLevel).To(BeTrue(), "DebugLevel should be true in DataflowComponentServiceConfig")

				// Step 2: GetBenthosServiceConfig() returns config with DEBUG LogLevel
				benthosConfig := readDFC.GetBenthosServiceConfig()
				Expect(benthosConfig.DebugLevel).To(BeTrue(), "LogLevel should be DEBUG in BenthosServiceConfig")

				// Step 3: GenerateS6ConfigForBenthos() sets OPC_DEBUG environment variable
				benthosService := benthos.NewDefaultBenthosService("integration-test")
				s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "read-test")
				Expect(err).NotTo(HaveOccurred())
				Expect(s6Config.Env["OPC_DEBUG"]).To(Equal("debug"), "OPC_DEBUG should be set to debug in S6ServiceConfig")
			})
		})

		Context("WriteDataflowComponent", func() {
			It("should propagate debug_level through the chain", func() {
				writeDFC := runtime.DataflowComponentWriteServiceConfig

				// Step 1: DebugLevel flag set
				Expect(writeDFC.DebugLevel).To(BeTrue(), "DebugLevel should be true in DataflowComponentServiceConfig")

				// Step 2: GetBenthosServiceConfig() returns config with DEBUG LogLevel
				benthosConfig := writeDFC.GetBenthosServiceConfig()
				Expect(benthosConfig.DebugLevel).To(BeTrue(), "LogLevel should be DEBUG in BenthosServiceConfig")

				// Step 3: GenerateS6ConfigForBenthos() sets OPC_DEBUG environment variable
				benthosService := benthos.NewDefaultBenthosService("integration-test")
				s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "write-test")
				Expect(err).NotTo(HaveOccurred())
				Expect(s6Config.Env["OPC_DEBUG"]).To(Equal("debug"), "OPC_DEBUG should be set to debug in S6ServiceConfig")
			})
		})
	})

	Context("log level mapping", func() {
		// Test explicit mapping between debug_level boolean and BenthosServiceConfig LogLevel string
		type testCase struct {
			name              string
			debugLevel        bool
			expectedDebugLevel bool
			expectOpcDebugSet bool
			expectedOpcDebug  string
		}

		DescribeTable("debug_level to LogLevel mapping",
			func(tc testCase) {
				yamlData := `
config:
  connection:
    name: "test-connection"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "test"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`
				// Parse base YAML
				var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

				err := yaml.Unmarshal([]byte(yamlData), &spec)
				Expect(err).NotTo(HaveOccurred())

				// Set debug_level programmatically
				spec.Config.DebugLevel = tc.debugLevel

				// Convert to Runtime
				runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
				Expect(err).NotTo(HaveOccurred())

				// Test read DFC
				readBenthos := runtime.DataflowComponentReadServiceConfig.GetBenthosServiceConfig()
				Expect(readBenthos.DebugLevel).To(Equal(tc.expectedDebugLevel))

				benthosService := benthos.NewDefaultBenthosService("integration-test")
				readS6, err := benthosService.GenerateS6ConfigForBenthos(&readBenthos, "test-read")
				Expect(err).NotTo(HaveOccurred())

				opcDebugValue, opcDebugExists := readS6.Env["OPC_DEBUG"]
				Expect(opcDebugExists).To(Equal(tc.expectOpcDebugSet), "OPC_DEBUG existence should match expected")

				if tc.expectOpcDebugSet {
					Expect(opcDebugValue).To(Equal(tc.expectedOpcDebug), "OPC_DEBUG value should match expected")
				}

				// Test write DFC
				writeBenthos := runtime.DataflowComponentWriteServiceConfig.GetBenthosServiceConfig()
				Expect(writeBenthos.DebugLevel).To(Equal(tc.expectedDebugLevel))

				writeS6, err := benthosService.GenerateS6ConfigForBenthos(&writeBenthos, "test-write")
				Expect(err).NotTo(HaveOccurred())

				opcDebugValue, opcDebugExists = writeS6.Env["OPC_DEBUG"]
				Expect(opcDebugExists).To(Equal(tc.expectOpcDebugSet), "OPC_DEBUG existence should match expected")

				if tc.expectOpcDebugSet {
					Expect(opcDebugValue).To(Equal(tc.expectedOpcDebug), "OPC_DEBUG value should match expected")
				}
			},
			Entry("debug_level true maps to LogLevel DEBUG",
				testCase{
					name:               "debug_level true maps to LogLevel DEBUG",
					debugLevel:         true,
					expectedDebugLevel: true,
					expectOpcDebugSet:  true,
					expectedOpcDebug:   "debug",
				}),
			Entry("debug_level false maps to LogLevel INFO",
				testCase{
					name:               "debug_level false maps to LogLevel INFO",
					debugLevel:         false,
					expectedDebugLevel: false,
					expectOpcDebugSet:  false,
					expectedOpcDebug:   "",
				}),
		)
	})
})
