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

// Cutover parity: exampleparent ChildConfig path equivalence.
//
// After the cutover, RenderChildren matches legacy DeriveDesiredState semantics:
// cfg.ChildConfig is threaded verbatim as each child's UserSpec.Config, and a
// per-child DEVICE_ID variable (device-0, device-1, …) is added.
//
// One deliberate divergence: when cfg.ChildConfig == "", RenderChildren produces
// empty Config, while legacy DeriveDesiredState produced a default template
// ("address: {{ .IP }}:{{ .PORT }}\ndevice: {{ .DEVICE_ID }}"). This is safe:
// ExamplechildConfig embeds only BaseUserSpec — no address/device typed fields exist,
// so the default template was never parsed into a typed value. No shipped scenario
// (simple, configerror) relies on the template; those two do not set child_config.
//
// Tests:
//  1. RenderChildren threads cfg.ChildConfig verbatim (effective-config equivalence).
//  2. Per-child DEVICE_ID matches legacy DeriveDesiredState's "device-N" pattern exactly.
//  3. Empty cfg.ChildConfig → empty child Config (no fallback template; safe residual divergence).
var _ = Describe("exampleparent cutover parity — ChildConfig equivalence", func() {
	It("RenderChildren threads cfg.ChildConfig to child UserSpec.Config", func() {
		// Both legacy DeriveDesiredState and new RenderChildren thread ChildConfig verbatim.
		// This is the load-bearing equivalence: the cascade scenario uses child_config to
		// pass should_fail/max_failures to examplefailing children.
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
		// Legacy: childVariables = {DEVICE_ID: fmt.Sprintf("device-%d", i)}
		// New:    same — key DEVICE_ID, value device-0, device-1, …
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

	It("empty cfg.ChildConfig threads as empty string — legacy fallback template is not replicated", func() {
		// Threading test, empty-input case. ChildConfig IS threaded (same as above) —
		// the result is just an empty string when the input is empty. This also guards
		// the design decision NOT to replicate the legacy default-fallback template
		// ("address: {{ .IP }}:{{ .PORT }}\ndevice: {{ .DEVICE_ID }}") that legacy
		// DeriveDesiredState injected when ChildConfig was unset. That default is
		// intentionally absent: ExamplechildConfig has no typed address/device fields so
		// the template was never consumed. No active scenario (simple, configerror) relies
		// on it; both omit child_config.
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
