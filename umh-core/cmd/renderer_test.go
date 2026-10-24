// Copyright 2026 UMH Systems GmbH
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

package main

import (
	"bytes"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
)

var _ = Describe("renderSupervisorChildrenYAML", func() {
	type decodedChildren struct {
		Children []fsmv2config.ChildSpec `yaml:"children"`
	}

	// render fails the spec on a render error rather than asserting against an
	// empty document: an empty document means a supervisor with no children at
	// all, which no assertion below would distinguish from a correct render.
	render := func(registered []fsmv2config.ChildSpec) []fsmv2config.ChildSpec {
		doc, err := renderSupervisorChildrenYAML(registered)
		Expect(err).To(Succeed(), "rendering the supervisor children must not fail")

		var parsed decodedChildren
		Expect(yaml.Unmarshal([]byte(doc), &parsed)).To(Succeed())

		return parsed.Children
	}

	It("checks that only persistence is rendered when nothing is registered", func() {
		// The persistence child and its deps registration are both
		// unconditional, so no read of UseFSMv2MemoryCleanup may remain in
		// cmd/main.go, or persistence could spawn without its deps.
		src, err := os.ReadFile("main.go")
		Expect(err).To(Succeed())
		Expect(bytes.Contains(src, []byte("UseFSMv2MemoryCleanup"))).To(BeFalse(),
			"the memory-cleanup gate is still read in cmd/main.go")

		children := render(nil)
		Expect(children).To(HaveLen(1))
		Expect(children[0].Name).To(Equal("persistence"))
		Expect(children[0].WorkerType).To(Equal("persistence"))
	})

	It("checks that the registered children follow persistence, in order, with their config unchanged", func() {
		// The config holds characters that inline string formatting would
		// mangle, so it must round-trip through the YAML document unchanged.
		const hostile = "relay: \"x\\y\"\nline two"
		a := fsmv2config.ChildSpec{Name: "a", WorkerType: "type-a", UserSpec: fsmv2config.UserSpec{Config: hostile}, Enabled: true}
		b := fsmv2config.ChildSpec{Name: "b", WorkerType: "type-b", Enabled: true}

		children := render([]fsmv2config.ChildSpec{a, b})
		Expect(children).To(HaveLen(3))
		Expect(children[0].Name).To(Equal("persistence"))
		Expect(children[1].Name).To(Equal("a"))
		Expect(children[1].WorkerType).To(Equal("type-a"))
		Expect(children[1].UserSpec.Config).To(Equal(hostile))
		Expect(children[2].Name).To(Equal("b"))
		Expect(children[2].WorkerType).To(Equal("type-b"))
	})
})
