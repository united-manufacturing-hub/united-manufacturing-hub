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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
)

// Pins three RenderChildren behaviors against the legacy DeriveDesiredState path:
//  1. cfg.ChildConfig flows verbatim into each child's UserSpec.Config.
//  2. Per-child DEVICE_ID is "device-N" (index-based).
//  3. Empty cfg.ChildConfig produces empty child Config — no fallback template.
//
// Item 3 is deliberate: legacy DeriveDesiredState injected an "address:..."
// template that was never parsed (ExamplechildConfig has no typed address field).
// The shipped scenarios (simple, configerror) don't set child_config, so dropping
// the template changes no observed behavior.
var _ = Describe("exampleparent RenderChildren ChildConfig equivalence", func() {
	It("RenderChildren threads cfg.ChildConfig to child UserSpec.Config", func() {
		cfg := exampleparent.ExampleparentConfig{
			ChildrenCount: 2,
			ChildConfig:   "should_fail: true\nmax_failures: 3",
		}

		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(HaveLen(2))

		for i, spec := range specs {
			Expect(spec.UserSpec.Config).To(Equal("should_fail: true\nmax_failures: 3"),
				fmt.Sprintf("child-%d UserSpec.Config must equal cfg.ChildConfig verbatim", i))
		}
	})

	It("per-child DEVICE_ID matches legacy DeriveDesiredState device-N pattern exactly", func() {
		cfg := exampleparent.ExampleparentConfig{ChildrenCount: 3}

		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(HaveLen(3))

		for i, spec := range specs {
			deviceID, ok := spec.UserSpec.Variables.User["DEVICE_ID"]
			Expect(ok).To(BeTrue(),
				fmt.Sprintf("child-%d must have DEVICE_ID variable", i))
			Expect(deviceID).To(Equal(fmt.Sprintf("device-%d", i)),
				fmt.Sprintf("child-%d DEVICE_ID must match legacy device-N pattern", i))
		}
	})

	It("empty cfg.ChildConfig produces empty child Config — no fallback template", func() {
		// See file header: legacy fallback template was never parsed and is intentionally dropped.
		cfg := exampleparent.ExampleparentConfig{ChildrenCount: 1}

		specs, err := exampleparent.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(HaveLen(1))

		Expect(specs[0].UserSpec.Config).To(BeEmpty(),
			"empty ChildConfig threads as empty string — no legacy fallback template injected")
		Expect(strings.Contains(specs[0].UserSpec.Config, "{{")).To(BeFalse(),
			"no template markers must appear — legacy fallback template intentionally not replicated")
	})
})
