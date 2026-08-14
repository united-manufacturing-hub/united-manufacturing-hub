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

// A test in package diagnosis can reach Reduced.v and Reduced.state, which
// would make the "no number without its outcome" property vacuous. This file
// stays in package diagnosis_test, where .v and .state are unexported and the
// window is driven only through the exported interface (NewWindow, Known,
// Observe, Reduce, Get).
package diagnosis_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("Reduced access", func() {
	It("should not expose the reduced number without its outcome", func() {
		w, _ := diagnosis.NewWindow(time.Hour, 60*time.Second, diagnosis.Last, false)
		w.Observe(diagnosis.Known(5), diagnosis.Unknown(), time.Unix(1_000_000, 0))

		n, s := w.Reduce().Get()

		Expect(s).To(Equal(diagnosis.StateValue))
		Expect(n).To(Equal(5.0),
			"the value and its outcome are returned together, and only together")
	})
})
