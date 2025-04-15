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

package nmapserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Nmap YAML Normalizer", func() {
	Describe("NormalizeConfig", func() {

		It("should preserve existing values", func() {
			config := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			}

			normalizer := NewNormalizer()
			normalizedConfig := normalizer.NormalizeConfig(config)
			normalizedTarget := normalizedConfig.Target
			normalizedPort := normalizedConfig.Port

			Expect(normalizedTarget).To(Equal("127.0.0.1"))
			Expect(normalizedPort).To(Equal(443))
		})
	})

	// Test the package-level function
	Describe("NormalizeNmapConfig package function", func() {
		It("should use the default normalizer", func() {
			config := NmapServiceConfig{}

			// Use package-level function
			normalizedConfig1 := NormalizeBenthosConfig(config)

			// Use normalizer directly
			normalizer := NewNormalizer()
			normalizedConfig2 := normalizer.NormalizeConfig(config)

			// Results should be the same
			Expect(normalizedConfig1.Target).To(Equal(normalizedConfig2.Target))
			Expect(normalizedConfig1.Port).To(Equal(normalizedConfig2.Port))
		})
	})
})
