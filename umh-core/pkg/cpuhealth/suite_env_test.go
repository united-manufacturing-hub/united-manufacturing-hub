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

// The suite's environment must grant every capability the CPU table asks for.
// A signal whose Requires is missing from it resolves NoInstrument in all six
// scenarios, so the suite reports on a signal it never measured — and it says
// so quietly: the scenario count stays at 30/24 either way, because an outcome
// is emitted per signal x case whatever the case concluded. The spec that
// counts scenarios therefore cannot see this, and neither can a reader.
package cpuhealth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the environment the generated suite runs in", func() {
	It("should grant every capability the CPU table's instruments require", func() {
		env := suiteEnvironment()

		// Both shapes of the table: cpuTable omits container-limit-full entirely at
		// quota 0, so the quota-0 table can carry a requirement the quota-2
		// table does not, and vice versa.
		for _, t := range []diagnosis.Table[Sample]{cpuTable(4, 2.0), cpuTable(4, 0)} {
			for _, tableSignal := range t.Signals {
				for _, inst := range tableSignal.Instruments {
					for _, req := range inst.Requires {
						Expect(env.Has(req)).To(BeTrue(),
							"signal %q instrument %q requires %q, which the suite environment does not grant — that signal is NoInstrument in every scenario and the suite tests nothing about it",
							tableSignal.Name, inst.Name, req)
					}
				}
			}
		}

		// The converse is deliberately not asserted. HasVirtualization and
		// HasLimit gate several instruments each, and a capability granted but
		// unrequired is a widened environment, not a skipped signal — the
		// failure mode this file exists for only runs in one direction.
	})
})
