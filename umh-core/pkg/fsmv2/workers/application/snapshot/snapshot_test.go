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
var _ fsmv2.ObservedState = (*ApplicationObservedState)(nil)
var _ fsmv2.DesiredState = (*ApplicationDesiredState)(nil)

var _ = Describe("ApplicationObservedState", func() {
	It("should implement fsmv2.ObservedState interface", func() {
		obs := &ApplicationObservedState{
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
		obs := &ApplicationObservedState{
			CollectedAt: now,
		}
		Expect(obs.GetTimestamp()).To(Equal(now))
	})
})

var _ = Describe("ApplicationDesiredState", func() {
	It("should implement fsmv2.DesiredState interface", func() {
		desired := &ApplicationDesiredState{
			Name: "test-root",
		}

		// IsShutdownRequested should return false by default.
		Expect(desired.IsShutdownRequested()).To(BeFalse())
	})

	It("should return true when shutdown is requested", func() {
		desired := &ApplicationDesiredState{}
		desired.SetShutdownRequested(true)
		Expect(desired.IsShutdownRequested()).To(BeTrue())
	})

	It("should return children specs from named field", func() {
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

		// New flat structure - ChildrenSpecs is a direct field, not embedded
		desired := &ApplicationDesiredState{
			Name:          "test-root",
			ChildrenSpecs: children,
		}

		specs := desired.GetChildrenSpecs()
		Expect(specs).To(HaveLen(2))
		Expect(specs[0].Name).To(Equal("child-1"))
		Expect(specs[1].Name).To(Equal("child-2"))
	})
})
