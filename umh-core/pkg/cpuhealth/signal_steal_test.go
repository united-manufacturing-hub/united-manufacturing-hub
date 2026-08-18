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

package cpuhealth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the steal arms share one mark pair", func() {
	It("keeps every sigSteal instrument on the same Marks, so an episode fired by one arm stays releasable once selection hands over to the other", func() {
		sig := stealSignal()
		Expect(sig.Instruments).To(HaveLen(2),
			"this spec covers exactly the two known steal arms; add any new arm here too")

		want := sig.Instruments[0].Marks
		for _, in := range sig.Instruments {
			Expect(in.Marks).To(Equal(want),
				"sigSteal's %q instrument carries different Marks than %q; a fired latch releases only against the mark pair its episode fired under, so a divergent pair would hold a mean-fired episode forever once selection hands over to p95 at twenty samples",
				in.Name, sig.Instruments[0].Name)
		}
	})
})
