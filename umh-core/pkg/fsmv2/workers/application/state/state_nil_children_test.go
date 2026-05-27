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

package state_test

// P0 GUARD: Application states MUST emit nil Children (never []config.ChildSpec{}).
//
// At the cutover, the supervisor will use NextResult.Children as the authoritative
// children set. nil means "no opinion — fall back to legacy ChildrenSpecs path."
// A non-nil empty slice means "I am a parent and I want ZERO children right now,"
// which would despawn all dataflow components.
//
// DO NOT weaken these assertions to BeEmpty() — BeEmpty() passes for both nil and
// []config.ChildSpec{} and would mask the invariant violation.

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/state"
)

// buildApplicationSnap builds a minimal valid snapshot for application state tests.
func buildApplicationSnap(shutdownRequested bool) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[snapshot.ApplicationStatus]{},
		Desired: &fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]{
			BaseDesiredState: config.BaseDesiredState{ShutdownRequested: shutdownRequested},
			Config:           snapshot.ApplicationConfig{},
		},
	}
}

var _ = Describe("Application state nil-Children invariant (P0 guard)", func() {
	// Each application state must return nil Children (not an empty slice).
	// Violation here means the cutover would despawn ALL dataflow components.

	Describe("RunningState", func() {
		It("returns nil Children on healthy tick", func() {
			result := (&state.RunningState{}).Next(buildApplicationSnap(false))
			Expect(result.Children).To(BeNil())
		})

		It("returns nil Children on shutdown tick", func() {
			result := (&state.RunningState{}).Next(buildApplicationSnap(true))
			Expect(result.Children).To(BeNil())
		})
	})

	Describe("DegradedState", func() {
		It("returns nil Children on degraded tick", func() {
			result := (&state.DegradedState{}).Next(buildApplicationSnap(false))
			Expect(result.Children).To(BeNil())
		})

		It("returns nil Children on shutdown tick", func() {
			result := (&state.DegradedState{}).Next(buildApplicationSnap(true))
			Expect(result.Children).To(BeNil())
		})
	})

	Describe("StoppedState", func() {
		It("returns nil Children on stopped tick", func() {
			result := (&state.StoppedState{}).Next(buildApplicationSnap(false))
			Expect(result.Children).To(BeNil())
		})

		It("returns nil Children on shutdown tick", func() {
			result := (&state.StoppedState{}).Next(buildApplicationSnap(true))
			Expect(result.Children).To(BeNil())
		})
	})
})
