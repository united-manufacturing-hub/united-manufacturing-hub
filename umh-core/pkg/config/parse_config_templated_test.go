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

package config

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseConfigTemplated", func() {
	Context("when the config is valid", func() {
		It("should parse the config correctly", func() {
			cfg, err := os.ReadFile("../../examples/example-config-dataflow-templated.yaml")
			Expect(err).To(BeNil())

			parsedConfig, err := parseConfig(cfg)
			Expect(err).To(BeNil())

			Expect(parsedConfig.DataFlow).To(HaveLen(2))
			Expect(parsedConfig.DataFlow[0].Name).To(Equal("data-flow-hello-world"))

			generatedConfigGeneratePart := parsedConfig.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.Input["generate"]
			Expect(generatedConfigGeneratePart).To(Equal(map[string]any{
				"mapping":  "root = \"{{ .customVariables.greeting }}\"",
				"interval": "1s",
				"count":    0,
			}))
		})
	})
})
