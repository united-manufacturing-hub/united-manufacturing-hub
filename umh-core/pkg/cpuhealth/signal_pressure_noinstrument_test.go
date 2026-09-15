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

// A host whose kernel never reported PSI must not offer the pressure
// instrument at all: the pressure instrument Requires HasPressureStats, so
// selection resolves a no-PSI host to NoInstrument. The effect is the
// availability label only — neither NoInstrument nor the AllAbsent an
// ever-absent reading would otherwise report (released immediately by
// ReleaseOnAbsent) arms a hold.
package cpuhealth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the pressure instrument on a host that never reported PSI", func() {
	It("should not offer the pressure instrument on a host whose kernel never reported PSI", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())

		// A kernel without CONFIG_PSI never publishes cpu.pressure: the sticky
		// PsiAvailable is false, so the environment this Sample yields carries no
		// HasPressureStats. The pressure instrument Requires it, so selection
		// resolves the signal to NoInstrument.
		s := Sample{
			PsiAvailable: false,
			Pressure:     diagnosis.Reading{},
		}
		env := DeriveEnvironment(s)

		_, _, _, availability := engine.Select(signalNamed(cpuTable(4, 2.0), "pressure"), env)
		Expect(availability).To(Equal(diagnosis.NoInstrument),
			"a no-PSI host must be NoInstrument, not AllAbsent")
	})

	It("should offer the pressure instrument on the positive path — a host with PSI is pressurable", func() {
		// The twin guard to the spec above: DeriveEnvironment(Sample{
		// PsiAvailable: true}) must carry HasPressureStats, and the pressure
		// signal must resolve past NoInstrument on it. Without this the lone
		// no-PSI spec cannot distinguish a correct PSI-aware derivation from a
		// broken no-op one (both resolve NoInstrument), so it would certify the
		// missing wiring as correct. This is the assertion that breaks first if
		// the DeriveEnvironment append is removed.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())

		s := Sample{
			PsiAvailable: true,
			Pressure:     diagnosis.Known(0),
		}
		env := DeriveEnvironment(s)
		Expect(env.Has(HasPressureStats)).To(BeTrue(), "a PSI-present host must declare HasPressureStats")

		_, _, _, availability := engine.Select(signalNamed(cpuTable(4, 2.0), "pressure"), env)
		Expect(availability).NotTo(Equal(diagnosis.NoInstrument),
			"the pressure signal must be offered on a PSI-present host")
	})
})
