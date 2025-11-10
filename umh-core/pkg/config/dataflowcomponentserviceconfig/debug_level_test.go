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
	"gopkg.in/yaml.v3"
)

var _ = Describe("DataflowComponentServiceConfig DebugLevel", func() {
	Describe("ParseYAML", func() {
		Context("when debug_level is true", func() {
			It("should parse DebugLevel as true", func() {
				yamlData := `
debug_level: true
benthos:
  input:
    generate:
      mapping: 'root = ""'
  output:
    stdout: {}
`

				var config dataflowcomponentserviceconfig.DataflowComponentServiceConfig

				err := yaml.Unmarshal([]byte(yamlData), &config)
				Expect(err).NotTo(HaveOccurred())
				Expect(config.DebugLevel).To(BeTrue())
			})
		})

		Context("when debug_level is false", func() {
			It("should parse DebugLevel as false", func() {
				yamlData := `
debug_level: false
benthos:
  input:
    generate:
      mapping: 'root = ""'
  output:
    stdout: {}
`

				var config dataflowcomponentserviceconfig.DataflowComponentServiceConfig

				err := yaml.Unmarshal([]byte(yamlData), &config)
				Expect(err).NotTo(HaveOccurred())
				Expect(config.DebugLevel).To(BeFalse())
			})
		})

		Context("when debug_level is omitted", func() {
			It("should default DebugLevel to false", func() {
				yamlData := `
benthos:
  input:
    generate:
      mapping: 'root = ""'
  output:
    stdout: {}
`

				var config dataflowcomponentserviceconfig.DataflowComponentServiceConfig

				err := yaml.Unmarshal([]byte(yamlData), &config)
				Expect(err).NotTo(HaveOccurred())
				Expect(config.DebugLevel).To(BeFalse())
			})
		})
	})

	Describe("GetBenthosServiceConfig", func() {
		Context("when DebugLevel is true", func() {
			It("should set LogLevel to DEBUG", func() {
				cfg := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					DebugLevel: true,
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]any{
							"generate": map[string]any{
								"mapping": `root = ""`,
							},
						},
						Output: map[string]any{
							"stdout": map[string]any{},
						},
					},
				}

				benthosConfig := cfg.GetBenthosServiceConfig()
				Expect(benthosConfig.LogLevel).To(Equal("DEBUG"))
			})
		})

		Context("when DebugLevel is false", func() {
			It("should set LogLevel to INFO", func() {
				cfg := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					DebugLevel: false,
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]any{
							"generate": map[string]any{
								"mapping": `root = ""`,
							},
						},
						Output: map[string]any{
							"stdout": map[string]any{},
						},
					},
				}

				benthosConfig := cfg.GetBenthosServiceConfig()
				Expect(benthosConfig.LogLevel).To(Equal("INFO"))
			})
		})
	})
})
