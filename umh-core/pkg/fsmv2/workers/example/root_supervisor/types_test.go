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

package root_supervisor_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	rootsupervisor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/root_supervisor"
)

func TestRootSupervisor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Root Supervisor Suite")
}

// Compile-time interface verification tests
// These ensure that our types satisfy the required fsmv2 interfaces

var _ = Describe("Interface Verification", func() {
	Describe("Compile-time interface checks", func() {
		It("ChildObservedState should implement fsmv2.ObservedState", func() {
			// Compile-time verification
			var _ fsmv2.ObservedState = (*rootsupervisor.ChildObservedState)(nil)
		})

		It("ChildDesiredState should implement fsmv2.DesiredState", func() {
			// Compile-time verification
			var _ fsmv2.DesiredState = (*rootsupervisor.ChildDesiredState)(nil)
		})
	})
})

var _ = Describe("ChildObservedState", func() {
	var (
		observed rootsupervisor.ChildObservedState
		now      time.Time
	)

	BeforeEach(func() {
		now = time.Now()
		observed = rootsupervisor.ChildObservedState{
			ID:          "child-1",
			CollectedAt: now,
			ChildDesiredState: rootsupervisor.ChildDesiredState{
				ShutdownRequested: false,
			},
			ConnectionStatus: "connected",
			ConnectAttempts:  1,
			ConnectionHealth: "healthy",
		}
	})

	Describe("GetTimestamp", func() {
		It("should return the CollectedAt timestamp", func() {
			timestamp := observed.GetTimestamp()
			Expect(timestamp).To(Equal(now))
		})
	})

	Describe("GetObservedDesiredState", func() {
		It("should return the embedded ChildDesiredState", func() {
			desired := observed.GetObservedDesiredState()
			Expect(desired).NotTo(BeNil())
			Expect(desired.IsShutdownRequested()).To(BeFalse())
		})
	})
})

var _ = Describe("ChildDesiredState", func() {
	Describe("IsShutdownRequested", func() {
		It("should return false when shutdown is not requested", func() {
			desired := rootsupervisor.ChildDesiredState{
				ShutdownRequested: false,
			}
			Expect(desired.IsShutdownRequested()).To(BeFalse())
		})

		It("should return true when shutdown is requested", func() {
			desired := rootsupervisor.ChildDesiredState{
				ShutdownRequested: true,
			}
			Expect(desired.IsShutdownRequested()).To(BeTrue())
		})
	})

	Describe("ShouldBeRunning", func() {
		It("should return true when shutdown is not requested", func() {
			desired := rootsupervisor.ChildDesiredState{
				ShutdownRequested: false,
			}
			Expect(desired.ShouldBeRunning()).To(BeTrue())
		})

		It("should return false when shutdown is requested", func() {
			desired := rootsupervisor.ChildDesiredState{
				ShutdownRequested: true,
			}
			Expect(desired.ShouldBeRunning()).To(BeFalse())
		})
	})
})
