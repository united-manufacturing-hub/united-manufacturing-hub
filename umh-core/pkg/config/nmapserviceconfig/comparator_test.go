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

var _ = Describe("Benthos YAML Comparator", func() {
	Describe("ConfigsEqual", func() {
		It("should consider identical configs equal", func() {
			config1 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			}
			config2 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)

			Expect(equal).To(BeTrue())
		})

		It("should consider configs with different targets not equal", func() {
			config1 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			}
			config2 := NmapServiceConfig{
				Target: "127.0.0.2",
				Port:   443,
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("Target:"))
			Expect(diff).To(ContainSubstring("Want: 127.0.0.1"))
			Expect(diff).To(ContainSubstring("Have: 127.0.0.2"))
		})

		It("should consider configs with different outputs not equal", func() {

			config1 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			}
			config2 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   444,
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("Port:"))
			Expect(diff).To(ContainSubstring("Want: 443"))
			Expect(diff).To(ContainSubstring("Have: 444"))
		})
	})

	// Test package-level functions
	Describe("Package-level functions", func() {
		It("ConfigsEqual should use default comparator", func() {

			config1 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			}
			config2 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			}

			// Use package-level function
			equal1 := ConfigsEqual(config1, config2)

			// Use comparator directly
			comparator := NewComparator()
			equal2 := comparator.ConfigsEqual(config1, config2)

			Expect(equal1).To(Equal(equal2))
		})

		It("ConfigDiff should use default comparator", func() {

			config1 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			}
			config2 := NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   444,
			}

			// Use package-level function
			diff1 := ConfigDiff(config1, config2)

			// Use comparator directly
			comparator := NewComparator()
			diff2 := comparator.ConfigDiff(config1, config2)

			Expect(diff1).To(Equal(diff2))
		})
	})
})
