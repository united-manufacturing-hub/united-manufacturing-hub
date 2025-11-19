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

package root

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

func TestRoot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Root Suite")
}

// Compile-time interface verification.
var _ fsmv2.ObservedState = (*PassthroughObservedState)(nil)
var _ fsmv2.DesiredState = (*PassthroughDesiredState)(nil)

var _ = Describe("PassthroughObservedState", func() {
	It("should implement fsmv2.ObservedState interface", func() {
		obs := &PassthroughObservedState{
			CollectedAt: time.Now(),
			Name:        "test-root",
		}

		// GetTimestamp should return the collected time.
		Expect(obs.GetTimestamp()).NotTo(BeZero())

		// GetObservedDesiredState should return the deployed desired state.
		desired := obs.GetObservedDesiredState()
		Expect(desired).NotTo(BeNil())
	})

	It("should return the correct timestamp", func() {
		now := time.Now()
		obs := &PassthroughObservedState{
			CollectedAt: now,
		}
		Expect(obs.GetTimestamp()).To(Equal(now))
	})
})

var _ = Describe("PassthroughDesiredState", func() {
	It("should implement fsmv2.DesiredState interface", func() {
		desired := &PassthroughDesiredState{
			DesiredState: config.DesiredState{
				State: "running",
			},
			Name: "test-root",
		}

		// IsShutdownRequested should return false for "running" state.
		Expect(desired.IsShutdownRequested()).To(BeFalse())
	})

	It("should return true for shutdown state", func() {
		desired := &PassthroughDesiredState{
			DesiredState: config.DesiredState{
				State: "shutdown",
			},
		}
		Expect(desired.IsShutdownRequested()).To(BeTrue())
	})

	It("should return children specs from embedded DesiredState", func() {
		children := []config.ChildSpec{
			{
				Name:       "child-1",
				WorkerType: "example-child",
			},
			{
				Name:       "child-2",
				WorkerType: "example-child",
			},
		}

		desired := &PassthroughDesiredState{
			DesiredState: config.DesiredState{
				State:         "running",
				ChildrenSpecs: children,
			},
		}

		specs := desired.GetChildrenSpecs()
		Expect(specs).To(HaveLen(2))
		Expect(specs[0].Name).To(Equal("child-1"))
		Expect(specs[1].Name).To(Equal("child-2"))
	})
})
