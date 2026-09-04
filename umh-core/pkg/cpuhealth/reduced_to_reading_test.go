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

// The table test for readingFromReduced, the three-state to two-state
// narrowing. The rule and its reason are stated at readingFromReduced in
// decide.go; the untrusted state is declared at diagnosis.StateUntrusted.

package cpuhealth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the reduced-to-reading conversion", func() {
	It("should keep only a StateValue reduction as a present Reading, with StateAbsent and StateUntrusted both narrowing to an absent one", func() {
		// Each arm carries its own non-zero number, including the arms
		// that must drop it. A conversion that answered Known(0) differs
		// from the correct one only in these absent arms. So does one that
		// published the partial number an untrusted window formed. Absence
		// is asserted through Reading's second return, never by comparing
		// the number to 0.
		cases := []struct {
			state   diagnosis.State
			value   float64
			present bool
			want    float64
		}{
			{state: diagnosis.StateValue, value: 3.5, present: true, want: 3.5},
			{state: diagnosis.StateAbsent, value: 7.25, present: false},
			{state: diagnosis.StateUntrusted, value: 1.25, present: false},
		}
		for _, tc := range cases {
			reading := readingFromReduced(tc.value, tc.state)
			v, ok := reading.Get()
			Expect(ok).To(Equal(tc.present), "a reduction in state %d must narrow to a present Reading only when it is StateValue", tc.state)
			if tc.present {
				Expect(v).To(Equal(tc.want), "a StateValue reduction must keep its own number")
			}
		}
	})
})
