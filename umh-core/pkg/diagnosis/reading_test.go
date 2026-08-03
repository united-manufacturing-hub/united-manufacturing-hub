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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Reading", func() {
	It("should represent a reading as either a value or an absence, and treat an absence as different from a zero", func() {
		// A known zero is present: zero is a legitimate value, not an absence.
		v, ok := Known(0).Get()
		Expect(ok).To(BeTrue(), "a known zero must be present, not an absence")
		Expect(v).To(Equal(0.0))

		// A nonzero known value comes back with its value and its presence.
		v, ok = Known(42).Get()
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal(42.0))

		// An absence reports a zero value but is NOT present: the value
		// field is zero, the presence is false, and that is what separates
		// "no reading" from "a reading of zero".
		v, ok = Unknown().Get()
		Expect(ok).To(BeFalse(), "an absence must not be present, even though its value is zero")
		Expect(v).To(Equal(0.0))
	})

	// The construction-encapsulation half of this spec — that a Reading is
	// reachable only through Known and Unknown — is unobservable from inside
	// the package (an internal test can reach the unexported fields directly),
	// and SPEC §9 R1 defers it to S2 R5, where the `cpu` package crosses the
	// boundary. What S1 can observe is the return contract: the value and its
	// presence arrive together for a known reading, and an absence arrives as
	// presence-false with no usable value.
	It("should build a reading only through Known and Unknown, and return the value and its presence together or not at all", func() {
		v, ok := Known(7).Get()
		Expect(v).To(Equal(7.0))
		Expect(ok).To(BeTrue(), "a known reading returns its value and presence together")

		_, ok = Unknown().Get()
		Expect(ok).To(BeFalse(), "an absence returns no presence at all")
	})
})
