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

package config_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

var _ = Describe("ParseVersion", func() {
	DescribeTable("accepts one-part and two-part keys",
		func(key string, wantMajor, wantMinor int) {
			v, err := config.ParseVersion(key)
			Expect(err).NotTo(HaveOccurred())
			Expect(v.Major).To(Equal(wantMajor))
			Expect(v.Minor).To(Equal(wantMinor))
		},
		Entry("bare v1", "v1", 1, 0),
		Entry("bare v10", "v10", 10, 0),
		Entry("two-part", "v1_2", 1, 2),
		Entry("minor zero", "v1_0", 1, 0),
		Entry("multi-digit both", "v10_23", 10, 23),
	)

	DescribeTable("rejects invalid keys",
		func(key string) {
			_, err := config.ParseVersion(key)
			Expect(err).To(HaveOccurred())
		},
		Entry("major zero", "v0"),
		Entry("major zero two-part", "v0_0"),
		Entry("empty minor", "v1_"),
		Entry("no v", "1_0"),
		Entry("empty", ""),
		Entry("three parts", "v1_0_1"),
		Entry("non-numeric", "vx"),
		Entry("negative", "v-1"),
		Entry("leading zero major", "v01"),
		Entry("leading zero minor", "v1_01"),
		Entry("leading zero major, multi-digit", "v007"),
	)

	DescribeTable("rejects with an actionable message",
		func(key string) {
			_, err := config.ParseVersion(key)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(key))
			Expect(err.Error()).To(ContainSubstring(`"v1"`))
			Expect(err.Error()).To(ContainSubstring(`"v1_2"`))
			Expect(err.Error()).To(ContainSubstring("major version starts at 1"))
			Expect(err.Error()).To(ContainSubstring("leading zero"))
		},
		Entry("major zero", "v0"),
		Entry("leading zero major", "v01"),
		Entry("empty minor", "v1_"),
		Entry("non-numeric", "vx"),
	)

	It("round-trips through String, canonical two-part (P1)", func() {
		for major := 1; major <= 12; major++ {
			for minor := range 13 {
				v := config.Version{Major: major, Minor: minor}
				got, err := config.ParseVersion(v.String())
				Expect(err).NotTo(HaveOccurred())
				Expect(got).To(Equal(v))
			}
		}
	})

	It("reads bare vN as vN_0 (P2)", func() {
		for major := 1; major <= 12; major++ {
			bare, err := config.ParseVersion(fmt.Sprintf("v%d", major))
			Expect(err).NotTo(HaveOccurred())
			twoPart, err := config.ParseVersion(fmt.Sprintf("v%d_0", major))
			Expect(err).NotTo(HaveOccurred())
			Expect(bare).To(Equal(twoPart))
		}
	})
})

var _ = Describe("Version.Compare", func() {
	It("orders by major first, then minor", func() {
		Expect(config.Version{Major: 1, Minor: 0}.Compare(config.Version{Major: 1, Minor: 1})).To(BeNumerically("<", 0))
		Expect(config.Version{Major: 2, Minor: 0}.Compare(config.Version{Major: 1, Minor: 9})).To(BeNumerically(">", 0))
		Expect(config.Version{Major: 10, Minor: 0}.Compare(config.Version{Major: 9, Minor: 9})).To(BeNumerically(">", 0))
		Expect(config.Version{Major: 1, Minor: 1}.Compare(config.Version{Major: 1, Minor: 1})).To(Equal(0))
	})
})

var _ = Describe("NextMinor", func() {
	DescribeTable("increments the minor of the highest major (P6)",
		func(keys []string, want string) {
			v, err := config.NextMinor(keys)
			Expect(err).NotTo(HaveOccurred())
			Expect(v.String()).To(Equal(want))
		},
		Entry("legacy single", []string{"v1"}, "v1_1"),
		Entry("legacy plus minor", []string{"v1", "v1_1"}, "v1_2"),
		Entry("two majors", []string{"v1", "v2"}, "v2_1"),
		Entry("new model", []string{"v1_0"}, "v1_1"),
		Entry("numeric major order", []string{"v9", "v10"}, "v10_1"),
		Entry("no versions", []string{}, "v1_0"),
		Entry("unordered input", []string{"v1_2", "v1", "v1_1"}, "v1_3"),
	)

	It("returns a version above every existing one and colliding with none (P6)", func() {
		keys := []string{"v1", "v1_1", "v2", "v2_3"}
		next, err := config.NextMinor(keys)
		Expect(err).NotTo(HaveOccurred())
		for _, k := range keys {
			existing, err := config.ParseVersion(k)
			Expect(err).NotTo(HaveOccurred())
			Expect(next.Compare(existing)).To(BeNumerically(">", 0))
			Expect(next.String()).NotTo(Equal(existing.String()))
		}
	})

	It("rejects an unparseable key rather than skipping it", func() {
		_, err := config.NextMinor([]string{"v1", "garbage"})
		Expect(err).To(HaveOccurred())
	})
})
