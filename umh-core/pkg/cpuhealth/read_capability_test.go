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

// diagnosis.Environment is a set keyed by the raw string, so a Capability
// constant is not a label on a fact — it IS the join key between what
// DeriveEnvironment observed and what an instrument requires. Two constants
// sharing a string silently merge two unrelated facts: a copy-paste that gave
// HasPressureStats the string "cpuhealth.HasLimit" makes a PSI-present box with
// no quota answer true to env.Has(HasLimit), so buildDetails stamps
// LimitApplies and the message renders limit mode on a box that has no
// limit. Nothing else in the package can catch that — every instrument and every derivation is written
// in terms of the constants, so all of them agree with the typo.
package cpuhealth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the CPU capability constants", func() {
	It("should be three distinct join keys, each spelled as itself", func() {
		Expect([]diagnosis.Capability{HasVirtualization, HasLimit, HasPressureStats}).
			To(ConsistOf(
				diagnosis.Capability("cpuhealth.HasVirtualization"),
				diagnosis.Capability("cpuhealth.HasLimit"),
				diagnosis.Capability("cpuhealth.HasPressureStats"),
			), "each capability must key on its own string: a shared string merges two unrelated host facts into one set entry")

		// Pinned individually as well, so a failure names the constant that
		// moved rather than reporting that the set as a whole no longer matches.
		Expect(HasVirtualization).To(Equal(diagnosis.Capability("cpuhealth.HasVirtualization")))
		Expect(HasLimit).To(Equal(diagnosis.Capability("cpuhealth.HasLimit")))
		Expect(HasPressureStats).To(Equal(diagnosis.Capability("cpuhealth.HasPressureStats")))
	})
})
