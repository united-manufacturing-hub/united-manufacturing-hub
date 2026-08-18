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

package benthosserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("canonicalize", func() {
	It("fast path matches the round trip for a representative config", func() {
		cfg := NewNormalizer().NormalizeConfig(bridgeConfig(10))

		fast, ok := canonicalizeFast(cfg)
		Expect(ok).To(BeTrue())

		Expect(fast).To(Equal(canonicalizeSlow(cfg)))
	})

	It("falls back to the round trip for a value the walk declines, and still matches it", func() {
		cfg := NewNormalizer().NormalizeConfig(bridgeConfig(10))
		cfg.Input["s7comm"].(map[string]interface{})["preamble"] = "\nSELECT 1;\n"

		_, ok := canonicalizeFast(cfg)
		Expect(ok).To(BeFalse())

		Expect(canonicalize(cfg)).To(Equal(canonicalizeSlow(cfg)))
	})

	It("leaves an empty section untouched instead of walking it", func() {
		cfg := BenthosServiceConfig{Input: map[string]interface{}{}, Output: map[string]interface{}{}}

		fast, ok := canonicalizeFast(cfg)
		Expect(ok).To(BeTrue())
		Expect(fast.Input).To(Equal(cfg.Input))
		Expect(fast.Output).To(Equal(cfg.Output))
	})
})
