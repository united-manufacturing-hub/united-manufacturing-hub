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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"gopkg.in/yaml.v3"
)

var _ = Describe("ProtocolConverterServiceConfigSpec DebugLevel", func() {
	Describe("ParseYAML", func() {
		Context("when debug_level is true", func() {
			It("should parse DebugLevel as true", func() {
				yamlData := `
config:
  debug_level: true
  connection:
    name: "test-connection"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
  dataflowcomponent_write:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
`

				var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

				err := yaml.Unmarshal([]byte(yamlData), &spec)
				Expect(err).NotTo(HaveOccurred())
				Expect(spec.Config.DebugLevel).To(BeTrue())
			})
		})

		Context("when debug_level is false", func() {
			It("should parse DebugLevel as false", func() {
				yamlData := `
config:
  debug_level: false
  connection:
    name: "test-connection"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
  dataflowcomponent_write:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
`

				var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

				err := yaml.Unmarshal([]byte(yamlData), &spec)
				Expect(err).NotTo(HaveOccurred())
				Expect(spec.Config.DebugLevel).To(BeFalse())
			})
		})

		Context("when debug_level is omitted", func() {
			It("should default DebugLevel to false", func() {
				yamlData := `
config:
  connection:
    name: "test-connection"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
  dataflowcomponent_write:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
`

				var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

				err := yaml.Unmarshal([]byte(yamlData), &spec)
				Expect(err).NotTo(HaveOccurred())
				Expect(spec.Config.DebugLevel).To(BeFalse())
			})
		})
	})
})
