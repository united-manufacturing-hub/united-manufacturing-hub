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

package diagnosis

import (
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// This is the structural spec: it makes a readability fact or flag unwritable
// in the latch, and it is written BY HAND rather than driven red-green, because
// a flag-driven latch passes every other spec here and a break of the
// structural guarantee fails silently. The guarantee is by signature: there is
// no readability argument, no readability field, and no route by which one
// reaches the latch. If a bool ever joins Coverage, or a readability parameter
// ever joins Update, this spec has been broken. The leading string on Update
// is not readability: it is the name of the instrument whose reduction is being
// judged, stamped beside the marks when the latch fires.
var _ = Describe("Latch signature", func() {
	It("should derive its state from the reduction and the window's extent, and from no readability fact of any kind", func() {
		// Coverage is exactly two time.Duration fields (span, covered) and
		// nothing else, no bool, no Reading. The clear arm and re-fire arm are
		// gated on these; a window that has already engaged its freeze (a second
		// consecutive failed read) still spans its full duration, which is why
		// the fields carry no fact about whether THIS tick's read succeeded.
		ct := reflect.TypeOf(Coverage{})
		Expect(ct.NumField()).To(Equal(2),
			"Coverage must carry exactly two durations — a readability field smuggled in here is F1 rebuilt")
		for i := range ct.NumField() {
			Expect(ct.Field(i).Type).To(Equal(reflect.TypeOf(time.Duration(0))),
				"every Coverage field must be a time.Duration, never a bool or a Reading")
		}

		// Update's parameter list is fixed, checked as a function value so it
		// fails at COMPILE time the day a readability parameter is added: a
		// signature outranks any generated test.
		//
		// The explicit type IS the assertion, so it must not be inlined away.
		// staticcheck's QF1011 offers to drop it as redundant, and golangci-lint
		// --fix took that offer once: the line became `var _ = (&Latch{}).Update`,
		// which asserts only that the method exists. The suite stayed green,
		// because deleting a compile-time guard cannot fail a run.
		//nolint:staticcheck // QF1011: the written-out type is what this spec checks.
		var _ func(string, Reduced, Coverage, Marks, time.Time) = (&Latch{}).Update
	})
})
