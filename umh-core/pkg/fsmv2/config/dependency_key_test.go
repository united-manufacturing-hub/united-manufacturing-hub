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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// A sampler stands in for the kind of thing a scenario hands a worker: an
// interface the worker calls, with a real implementation in production and a
// controllable one under test.
type sampler interface {
	Sample() int
}

type fixedSampler struct{ value int }

func (s fixedSampler) Sample() int { return s.value }

var samplerKey = config.NewDependencyKey[sampler]("test.sampler")

var _ = Describe("DependencyKey", func() {
	It("reads back the value that was put under the same key", func() {
		m := map[string]any{}
		config.SetDependency(m, samplerKey, sampler(fixedSampler{value: 7}))

		got, ok := config.LookupDependency(m, samplerKey)
		Expect(ok).To(BeTrue())
		Expect(got.Sample()).To(Equal(7))
	})

	It("reports absent when nothing was put under the key", func() {
		got, ok := config.LookupDependency(map[string]any{}, samplerKey)
		Expect(ok).To(BeFalse())
		Expect(got).To(BeNil())
	})

	It("rejects a name declared again with a different type and allows the same type again", func() {
		// A name persists for the life of the test binary, so no other spec may
		// use these names.
		Expect(func() {
			config.NewDependencyKey[string]("test.redeclare.mismatch")
			config.NewDependencyKey[int]("test.redeclare.mismatch")
		}).To(PanicWith(And(
			ContainSubstring("test.redeclare.mismatch"),
			ContainSubstring("string"),
			ContainSubstring("int"),
		)))

		first := config.NewDependencyKey[string]("test.redeclare.same")
		second := config.NewDependencyKey[string]("test.redeclare.same")
		Expect(first).To(Equal(second))
	})

	It("rejects a name declared as an interface and then as a type that implements it", func() {
		Expect(func() {
			config.NewDependencyKey[sampler]("test.redeclare.interface")
			config.NewDependencyKey[fixedSampler]("test.redeclare.interface")
		}).To(PanicWith(ContainSubstring("test.redeclare.interface")))
	})

	It("reports absent when the stored value is not the key's type", func() {
		// A key cannot store a wrong type, but code that writes the map by name
		// can, and LookupDependency must still read that as absent.
		m := map[string]any{"test.sampler": "not a sampler"}

		got, ok := config.LookupDependency(m, samplerKey)
		Expect(ok).To(BeFalse())
		Expect(got).To(BeNil())
	})
})
