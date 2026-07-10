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

package simple_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

var _ = Describe("Health", func() {
	Describe("Healthy", func() {
		It("reports a not-degraded verdict carrying the reason", func() {
			h := simple.Healthy("running (queue drained)")

			Expect(h.Degraded).To(BeFalse())
			Expect(h.Reason).To(Equal("running (queue drained)"))
		})
	})

	Describe("Degraded", func() {
		It("reports a degraded verdict carrying the reason", func() {
			h := simple.Degraded("port 502 unreachable")

			Expect(h.Degraded).To(BeTrue())
			Expect(h.Reason).To(Equal("port 502 unreachable"))
		})
	})
})
