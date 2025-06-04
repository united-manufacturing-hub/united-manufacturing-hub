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

package variables_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

var _ = Describe("VariableBundle Normalizer", func() {
	var (
		normalizer *variables.Normalizer
	)

	BeforeEach(func() {
		normalizer = variables.NewNormalizer()
	})

	Describe("NormalizeConfig", func() {
		It("should return the same config for a complete config", func() {
			config := variables.VariableBundle{
				User: map[string]any{
					"key1": "value1",
					"key2": 42,
				},
				Global: map[string]any{
					"global1": "value1",
					"global2": true,
				},
				Internal: map[string]any{
					"internal1": "value1",
					"internal2": 3.14,
				},
			}

			normalized := normalizer.NormalizeConfig(config)
			Expect(normalized).To(Equal(config))
		})

		It("should handle empty maps", func() {
			config := variables.VariableBundle{}

			normalized := normalizer.NormalizeConfig(config)
			Expect(normalized).To(Equal(config))
		})

		It("should handle nil maps", func() {
			config := variables.VariableBundle{
				User:     nil,
				Global:   nil,
				Internal: nil,
			}

			normalized := normalizer.NormalizeConfig(config)
			Expect(normalized).To(Equal(config))
		})

		It("should handle complex nested structures", func() {
			config := variables.VariableBundle{
				User: map[string]any{
					"nested": map[string]any{
						"key1": []any{1, 2, 3},
						"key2": map[string]any{
							"subkey": "value",
						},
					},
				},
			}

			normalized := normalizer.NormalizeConfig(config)
			Expect(normalized).To(Equal(config))
		})
	})
})
