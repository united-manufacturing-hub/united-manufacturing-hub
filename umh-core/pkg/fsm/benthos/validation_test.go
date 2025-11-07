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

package benthos_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

var _ = Describe("Benthos FSM Path Traversal Protection (ENG-3869)", func() {
	Describe("Defense Layer 1: YAML Unmarshaling Validation", func() {
		Context("ProtocolConverterConfig unmarshaling", func() {
			It("should reject path traversal with ../../../etc/passwd", func() {
				yamlData := `
name: "../../../etc/passwd"
desiredState: active
protocolConverterServiceConfig: {}
`
				var cfg config.ProtocolConverterConfig
				err := yaml.Unmarshal([]byte(yamlData), &cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should reject path traversal with ../../data/secrets", func() {
				yamlData := `
name: "../../data/secrets"
desiredState: active
protocolConverterServiceConfig: {}
`
				var cfg config.ProtocolConverterConfig
				err := yaml.Unmarshal([]byte(yamlData), &cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should reject names with uppercase letters", func() {
				yamlData := `
name: "Bridge-One"
desiredState: active
protocolConverterServiceConfig: {}
`
				var cfg config.ProtocolConverterConfig
				err := yaml.Unmarshal([]byte(yamlData), &cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should reject names with special characters", func() {
				yamlData := `
name: "bridge@special"
desiredState: active
protocolConverterServiceConfig: {}
`
				var cfg config.ProtocolConverterConfig
				err := yaml.Unmarshal([]byte(yamlData), &cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should accept valid lowercase names with hyphens", func() {
				yamlData := `
name: "bridge-1"
desiredState: active
protocolConverterServiceConfig: {}
`
				var cfg config.ProtocolConverterConfig
				err := yaml.Unmarshal([]byte(yamlData), &cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Name).To(Equal("bridge-1"))
			})
		})

		Context("BenthosConfig unmarshaling", func() {
			It("should reject path traversal with ../../../etc/passwd", func() {
				yamlData := `
name: "../../../etc/passwd"
desiredState: active
benthosServiceConfig: {}
`
				var cfg config.BenthosConfig
				err := yaml.Unmarshal([]byte(yamlData), &cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should reject path traversal with ./sensitive", func() {
				yamlData := `
name: "./sensitive"
desiredState: active
benthosServiceConfig: {}
`
				var cfg config.BenthosConfig
				err := yaml.Unmarshal([]byte(yamlData), &cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should accept valid lowercase names with hyphens", func() {
				yamlData := `
name: "bridge-1"
desiredState: active
benthosServiceConfig: {}
`
				var cfg config.BenthosConfig
				err := yaml.Unmarshal([]byte(yamlData), &cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Name).To(Equal("bridge-1"))
			})
		})
	})

	Describe("Defense Layer 2: Manager getName Validation", func() {
		Context("BenthosConfig validation via config.ValidateComponentName", func() {
			It("should reject path traversal with ../../../etc/passwd", func() {
				err := config.ValidateComponentName("../../../etc/passwd")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should reject path traversal with ../../data/secrets", func() {
				err := config.ValidateComponentName("../../data/secrets")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should reject path traversal with ./sensitive", func() {
				err := config.ValidateComponentName("./sensitive")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should reject names with uppercase letters", func() {
				err := config.ValidateComponentName("Bridge-One")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should reject names with special characters", func() {
				err := config.ValidateComponentName("bridge@special")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only lowercase letters"))
			})

			It("should accept names with underscores", func() {
				err := config.ValidateComponentName("bridge_one")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should accept names starting with numbers", func() {
				err := config.ValidateComponentName("1bridge")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should accept valid lowercase names with hyphens", func() {
				err := config.ValidateComponentName("bridge-1")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should accept simple lowercase names", func() {
				err := config.ValidateComponentName("mybridge")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should accept names with multiple hyphens", func() {
				err := config.ValidateComponentName("my-test-bridge-123")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
