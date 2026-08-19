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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("canonicalize", func() {
	// The gate is an opt-out kill switch, so the walk is what production runs and
	// what every other spec here is about. A default flipped to false would leave
	// the optimization shipped but never taken, and a misspelled variable name would
	// leave it impossible to turn off.
	//
	// Asserted against the environment these specs set rather than against
	// useCanonicalizeFast, which is fixed at process start: running the suite with
	// USE_CANONICALIZE_FAST=false is a supported way to check the fallback, and must
	// not fail the spec that describes the default.
	Describe("USE_CANONICALIZE_FAST", func() {
		var prev string

		var hadPrev bool

		BeforeEach(func() {
			prev, hadPrev = os.LookupEnv("USE_CANONICALIZE_FAST")
		})

		AfterEach(func() {
			if hadPrev {
				Expect(os.Setenv("USE_CANONICALIZE_FAST", prev)).To(Succeed())

				return
			}

			Expect(os.Unsetenv("USE_CANONICALIZE_FAST")).To(Succeed())
		})

		It("takes the walk when unset", func() {
			Expect(os.Unsetenv("USE_CANONICALIZE_FAST")).To(Succeed())

			Expect(canonicalizeFastEnabled()).To(BeTrue(), "the gate must default to on")
		})

		It("takes the round-trip when set to false", func() {
			Expect(os.Setenv("USE_CANONICALIZE_FAST", "false")).To(Succeed())

			Expect(canonicalizeFastEnabled()).To(BeFalse(), "the kill switch must work")
		})
	})

	// An empty section renders as a sequence rather than a map, so round-tripping it
	// would hand the comparator a shape it does not expect.
	Describe("empty sections", func() {
		It("returns an empty map untouched", func() {
			empty := map[string]interface{}{}

			Expect(canonicalizeMap(empty)).To(Equal(empty))
			Expect(canonicalizeMap(nil)).To(BeNil())
		})

		It("returns an empty resource list untouched", func() {
			empty := []map[string]interface{}{}

			Expect(canonicalizeResources(empty)).To(Equal(empty))
			Expect(canonicalizeResources(nil)).To(BeNil())
		})
	})

	Describe("USE_CANONICALIZE_FAST gate", func() {
		var prev bool

		BeforeEach(func() {
			prev = useCanonicalizeFast
		})

		AfterEach(func() {
			useCanonicalizeFast = prev
		})

		withGate := func(on bool, cfg BenthosServiceConfig) BenthosServiceConfig {
			useCanonicalizeFast = on

			return canonicalize(cfg)
		}

		It("produces the same config with the walk on as with it off", func() {
			cfg := NewNormalizer().NormalizeConfig(bridgeConfig(50))

			Expect(withGate(true, cfg)).To(Equal(withGate(false, cfg)),
				"the walk must be indistinguishable from the round-trip it replaces")
		})

		It("stays correct when one section declines and the rest do not", func() {
			cfg := NewNormalizer().NormalizeConfig(bridgeConfigDeclining(20))

			// The fallback is per section: only Input holds the shape the walk
			// refuses, so the other sections are still walked.
			_, inputOK := fastNormalize(cfg.Input)
			Expect(inputOK).To(BeFalse(), "fixture no longer triggers a fallback")

			_, outputOK := fastNormalize(cfg.Output)
			Expect(outputOK).To(BeTrue(), "only Input should decline")

			Expect(withGate(true, cfg)).To(Equal(withGate(false, cfg)),
				"a mixed walked/round-tripped config must match the all-round-trip one")
		})
	})
})
