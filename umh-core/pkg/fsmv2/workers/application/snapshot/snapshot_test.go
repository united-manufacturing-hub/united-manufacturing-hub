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

package snapshot

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

func TestRoot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Root Suite")
}

var _ = Describe("ApplicationStatus", func() {
	Describe("HasInfrastructureIssues", func() {
		It("returns false when no issues", func() {
			s := ApplicationStatus{}
			Expect(s.HasInfrastructureIssues()).To(BeFalse())
		})

		It("returns true when circuit open", func() {
			s := ApplicationStatus{ChildrenCircuitOpen: 1}
			Expect(s.HasInfrastructureIssues()).To(BeTrue())
		})

		It("returns true when stale", func() {
			s := ApplicationStatus{ChildrenStale: 1}
			Expect(s.HasInfrastructureIssues()).To(BeTrue())
		})
	})

	Describe("InfrastructureReason", func() {
		It("returns empty string when no issues", func() {
			s := ApplicationStatus{}
			Expect(s.InfrastructureReason()).To(BeEmpty())
		})

		It("returns non-empty string when issues exist", func() {
			s := ApplicationStatus{ChildrenCircuitOpen: 2, ChildrenStale: 1}
			Expect(s.InfrastructureReason()).To(ContainSubstring("circuit_open=2"))
			Expect(s.InfrastructureReason()).To(ContainSubstring("stale=1"))
		})
	})

	Describe("ChildrenViewToStatus", func() {
		It("returns zeroed counts for nil view", func() {
			c, s := ChildrenViewToStatus(nil)
			Expect(c).To(Equal(0))
			Expect(s).To(Equal(0))
		})

		It("returns zeroed counts for wrong type", func() {
			c, s := ChildrenViewToStatus("not a ChildrenView")
			Expect(c).To(Equal(0))
			Expect(s).To(Equal(0))
		})

		It("counts circuit-open and stale children from a ChildrenView", func() {
			view := config.NewChildrenView([]config.ChildInfo{
				{Name: "a", IsCircuitOpen: true},
				{Name: "b", IsStale: true},
				{Name: "c"},
			})
			c, s := ChildrenViewToStatus(view)
			Expect(c).To(Equal(1))
			Expect(s).To(Equal(1))
		})
	})
})
