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

// wronglyTypedSamplerKey is the mistake this guards against: two keys agree on
// the name and disagree on the type, which is what happens when the side that
// hands a dependency in and the side that reads it declare their own keys.
var wronglyTypedSamplerKey = config.NewDependencyKey[string]("test.sampler")

var _ = Describe("DependencyKey", func() {
	It("reads back the value that was put under the same key", func() {
		m := map[string]any{}
		config.PutDependency(m, samplerKey, sampler(fixedSampler{value: 7}))

		got, ok := config.GetDependency(m, samplerKey)
		Expect(ok).To(BeTrue())
		Expect(got.Sample()).To(Equal(7))
	})

	It("reports absent when nothing was put under the key", func() {
		got, ok := config.GetDependency(map[string]any{}, samplerKey)
		Expect(ok).To(BeFalse())
		Expect(got).To(BeNil())
	})

	It("reports absent when the stored value is not the key's type", func() {
		m := map[string]any{}
		config.PutDependency(m, wronglyTypedSamplerKey, "not a sampler")

		got, ok := config.GetDependency(m, samplerKey)
		Expect(ok).To(BeFalse())
		Expect(got).To(BeNil())
	})
})
