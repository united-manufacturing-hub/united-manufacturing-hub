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
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var _ = Describe("canonicalize", func() {
	// The gate is an opt-out kill switch, so the walk is what production runs and
	// what every other spec here is about. A default flipped to false would leave
	// the optimization shipped but never taken, and a misspelled variable name would
	// leave it impossible to turn off.
	//
	// Asserted against the environment these specs set rather than against
	// useCanonicalizeFast, which is fixed at process start: running the suite with
	// USE_CANONICALIZE_FAST=0 is a supported way to check the fallback, and must
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
			} else {
				Expect(os.Unsetenv("USE_CANONICALIZE_FAST")).To(Succeed())
			}

			// Left unresolved so the next spec reads the environment it sets.
			gateOnce = sync.Once{}
		})

		// Drives the real path: set the variable, then let roundTrip resolve it. Both
		// halves are asserted at once, since a value that falls back without saying so
		// is the failure this guards against.
		resolve := func(value string, isSet bool) (bool, *observer.ObservedLogs) {
			if isSet {
				Expect(os.Setenv("USE_CANONICALIZE_FAST", value)).To(Succeed())
			} else {
				Expect(os.Unsetenv("USE_CANONICALIZE_FAST")).To(Succeed())
			}

			gateOnce = sync.Once{}

			core, logs := observer.New(zapcore.WarnLevel)
			defer zap.ReplaceGlobals(zap.New(core))()

			canonicalize(BenthosServiceConfig{Input: map[string]interface{}{"a": "b"}})

			return useCanonicalizeFast, logs
		}

		It("takes the walk when unset", func() {
			fast, logs := resolve("", false)

			Expect(fast).To(BeTrue(), "the gate must default to on")
			Expect(logs.Len()).To(BeZero())
		})

		DescribeTable("reads 0 and 1, and says nothing about them",
			func(value string, expected bool) {
				fast, logs := resolve(value, true)

				Expect(fast).To(Equal(expected))
				Expect(logs.Len()).To(BeZero(), "%q is valid and must not warn", value)
			},
			Entry("1 keeps the walk", "1", true),
			Entry("0 is the kill switch", "0", false),
			// A trailing space survives a compose file; that is a typo in the
			// whitespace, not an unclear intent.
			Entry("trailing space", "0 ", false),
			Entry("leading space", " 1", true),
		)

		// Whoever sets this was reaching for the off switch, so a value we cannot use
		// turns the walk off rather than leaving the default on - and says so, or the
		// operator sees no change and concludes the walk is not the cause.
		DescribeTable("falls back and warns about anything else",
			func(value string) {
				fast, logs := resolve(value, true)

				Expect(fast).To(BeFalse(), "%q left the walk on", value)
				Expect(logs.Len()).To(Equal(1), "%q resolved silently", value)

				line := logs.All()[0].Message
				Expect(line).To(ContainSubstring("USE_CANONICALIZE_FAST"))
				Expect(line).To(ContainSubstring(`"`+value+`"`), "the warning must name the value")
				Expect(line).To(ContainSubstring("0 nor 1"), "and say what is accepted")
			},
			Entry("a word", "disabled"),
			Entry("true, which is not accepted", "true"),
			Entry("false, which is not accepted", "false"),
			Entry("any other digit", "2"),
			Entry("set but empty", ""),
		)

		It("warns once, not on every tick", func() {
			Expect(os.Setenv("USE_CANONICALIZE_FAST", "disabled")).To(Succeed())

			gateOnce = sync.Once{}

			core, logs := observer.New(zapcore.WarnLevel)
			defer zap.ReplaceGlobals(zap.New(core))()

			for range 5 {
				canonicalize(BenthosServiceConfig{Input: map[string]interface{}{"a": "b"}})
			}

			Expect(logs.Len()).To(Equal(1))
		})

		// canonicalizeMap is reachable without going through canonicalize, and an
		// unresolved gate reads as false - which would silently disable the walk.
		It("resolves on a section canonicalized on its own", func() {
			Expect(os.Unsetenv("USE_CANONICALIZE_FAST")).To(Succeed())

			gateOnce = sync.Once{}
			useCanonicalizeFast = false

			canonicalizeMap(map[string]interface{}{"a": "b"})

			Expect(useCanonicalizeFast).To(BeTrue(), "roundTrip did not resolve the gate")
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
			gateOnce = sync.Once{}
		})

		withGate := func(on bool, cfg BenthosServiceConfig) BenthosServiceConfig {
			pinGate(on)

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
