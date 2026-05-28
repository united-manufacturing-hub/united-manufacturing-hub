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

package exampleparent_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fsmconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
)

// RenderChildren must not emit "{{" in any rendered Config: that path no
// longer template-expands (DEVICE_ID switched to loop index). Catches a
// future regression that re-introduces template strings silently.
var _ = Describe("exampleparent RenderChildren", func() {
	var cfg exampleparent.ExampleparentConfig

	BeforeEach(func() {
		cfg = exampleparent.ExampleparentConfig{
			ChildrenCount: 3,
		}
	})

	It("returns one spec per child in ChildrenCount", func() {
		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(HaveLen(3))
	})

	It("names children child-0, child-1, ... by loop index", func() {
		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs[0].Name).To(Equal("child-0"))
		Expect(specs[1].Name).To(Equal("child-1"))
		Expect(specs[2].Name).To(Equal("child-2"))
	})

	It("produces no leftover Go template markers in rendered child config (DEVICE_ID gate)", func() {
		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		for _, spec := range specs {
			Expect(strings.Contains(spec.UserSpec.Config, "{{")).To(BeFalse(),
				"child %q config must not contain unresolved template markers", spec.Name)
		}
	})

	It("sets Enabled=true on each spec when enabled=true", func() {
		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		for _, spec := range specs {
			Expect(spec.Enabled).To(BeTrue(), "spec %q must be enabled", spec.Name)
		}
	})

	It("sets Enabled=false on each spec when enabled=false", func() {
		specs, err := exampleparent.RenderChildren(cfg, false)
		Expect(err).ToNot(HaveOccurred())
		for _, spec := range specs {
			Expect(spec.Enabled).To(BeFalse(), "spec %q must be disabled", spec.Name)
		}
	})

	It("attaches ChildStartStates to each spec", func() {
		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		for _, spec := range specs {
			Expect(spec.ChildStartStates).To(ConsistOf("TryingToStart", "Running"),
				"spec %q must have correct ChildStartStates", spec.Name)
		}
	})

	It("parses each child UserSpec back to BaseUserSpec without error", func() {
		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		for _, spec := range specs {
			_, parseErr := fsmconfig.ParseUserSpec[fsmconfig.BaseUserSpec](spec.UserSpec)
			Expect(parseErr).ToNot(HaveOccurred(),
				"child %q config must be valid YAML parseable as BaseUserSpec", spec.Name)
		}
	})

	It("returns an empty slice for ChildrenCount=0", func() {
		emptyCfg := exampleparent.ExampleparentConfig{ChildrenCount: 0}
		specs, err := exampleparent.RenderChildren(emptyCfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(BeEmpty())
	})
})
