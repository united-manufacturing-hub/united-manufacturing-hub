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

package snapshot_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

func TestSnapshot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Snapshot Suite")
}

var _ = Describe("ApplicationStatus", func() {
	It("reports no infrastructure issues for zero counts", func() {
		s := snapshot.ApplicationStatus{}
		Expect(s.HasInfrastructureIssues()).To(BeFalse())
		Expect(s.InfrastructureReason()).To(BeEmpty())
	})

	It("reports infrastructure issues when circuit breakers are open", func() {
		s := snapshot.ApplicationStatus{ChildrenCircuitOpen: 2}
		Expect(s.HasInfrastructureIssues()).To(BeTrue())
		Expect(s.InfrastructureReason()).To(ContainSubstring("circuit_open=2"))
		Expect(s.InfrastructureReason()).To(ContainSubstring("stale=0"))
	})

	It("reports infrastructure issues when children are stale", func() {
		s := snapshot.ApplicationStatus{ChildrenStale: 3}
		Expect(s.HasInfrastructureIssues()).To(BeTrue())
		Expect(s.InfrastructureReason()).To(ContainSubstring("stale=3"))
	})
})

var _ = Describe("ChildrenViewToStatus", func() {
	It("returns zero counts for nil view", func() {
		open, stale := snapshot.ChildrenViewToStatus(nil)
		Expect(open).To(Equal(0))
		Expect(stale).To(Equal(0))
	})

	It("returns zero counts for wrong type", func() {
		open, stale := snapshot.ChildrenViewToStatus("not a view")
		Expect(open).To(Equal(0))
		Expect(stale).To(Equal(0))
	})
})
