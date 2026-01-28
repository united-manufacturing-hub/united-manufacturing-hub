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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
)

var _ = Describe("ExamplefailingObservedState", func() {
	Describe("GetTimestamp", func() {
		It("should return the CollectedAt timestamp", func() {
			now := time.Now()
			observed := snapshot.ExamplefailingObservedState{
				CollectedAt: now,
			}

			Expect(observed.GetTimestamp()).To(Equal(now))
		})
	})

	Describe("GetObservedDesiredState", func() {
		It("should return a non-nil desired state", func() {
			observed := snapshot.ExamplefailingObservedState{
				CollectedAt: time.Now(),
				State:       "running_connected",
			}

			desired := observed.GetObservedDesiredState()
			Expect(desired).NotTo(BeNil())
		})
	})
})

var _ = Describe("ExamplefailingDesiredState", func() {
	Describe("ShutdownRequested", func() {
		DescribeTable("should correctly report shutdown status",
			func(shutdown bool, want bool) {
				desired := &snapshot.ExamplefailingDesiredState{}
				desired.SetShutdownRequested(shutdown)

				Expect(desired.IsShutdownRequested()).To(Equal(want))
			},
			Entry("not requested", false, false),
			Entry("requested", true, true),
		)
	})
})

var _ = Describe("ExamplefailingObservedState.IsStopRequired", func() {
	DescribeTable("should correctly determine stop requirement",
		func(shutdownRequested bool, parentMappedState string, want bool) {
			obs := snapshot.ExamplefailingObservedState{
				ExamplefailingDesiredState: snapshot.ExamplefailingDesiredState{
					ParentMappedState: parentMappedState,
				},
			}
			obs.ExamplefailingDesiredState.SetShutdownRequested(shutdownRequested)

			Expect(obs.IsStopRequired()).To(Equal(want))
		},
		Entry("returns true when shutdown requested", true, config.DesiredStateRunning, true),
		Entry("returns true when parent mapped state is stopped", false, config.DesiredStateStopped, true),
		Entry("returns true when parent mapped state is empty", false, "", true),
		Entry("returns false when running and not shutdown requested", false, config.DesiredStateRunning, false),
		Entry("returns true when both shutdown requested and parent stopped", true, config.DesiredStateStopped, true),
	)
})
