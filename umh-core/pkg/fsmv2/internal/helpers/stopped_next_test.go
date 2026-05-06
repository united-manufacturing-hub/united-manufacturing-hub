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

package helpers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

// stoppedNextStubConfig is a minimal TConfig that embeds BaseUserSpec so it
// satisfies the GetState() interface used by WorkerSnapshot.ShouldStop.
type stoppedNextStubConfig struct {
	config.BaseUserSpec
}

// stoppedNextStubStatus is an empty TStatus for the snapshot.
type stoppedNextStubStatus struct{}

// stoppedNextStubState is a minimal fsmv2.State[any, any] used as the
// `s` (self) and `nextState` parameters in StoppedNext tests.
type stoppedNextStubState struct {
	helpers.StoppedBase
	name string
}

func (s *stoppedNextStubState) Next(any) fsmv2.NextResult[any, any] {
	return fsmv2.NextResult[any, any]{}
}

func (s *stoppedNextStubState) String() string {
	return s.name
}

// stoppedNextRunningStub stands in for the "advance to nextState" target.
// It uses RunningHealthyBase to be distinguishable from the self state.
type stoppedNextRunningStub struct {
	helpers.RunningHealthyBase
	name string
}

func (s *stoppedNextRunningStub) Next(any) fsmv2.NextResult[any, any] {
	return fsmv2.NextResult[any, any]{}
}

func (s *stoppedNextRunningStub) String() string {
	return s.name
}

// makeSnap builds a WorkerSnapshot with the given desired flags and
// config-level state for use in StoppedNext branch tests.
func makeSnap(beingRemoved, disabled bool, configState string) fsmv2.WorkerSnapshot[stoppedNextStubConfig, stoppedNextStubStatus] {
	cfg := stoppedNextStubConfig{
		BaseUserSpec: config.BaseUserSpec{State: configState},
	}
	wrapped := fsmv2.WrappedDesiredState[stoppedNextStubConfig]{
		Config:   cfg,
		Disabled: disabled,
	}
	wrapped.SetBeingRemoved(beingRemoved)

	return fsmv2.WorkerSnapshot[stoppedNextStubConfig, stoppedNextStubStatus]{
		Desired: wrapped,
	}
}

var _ = Describe("StoppedNext", func() {
	var (
		self     *stoppedNextStubState
		next     *stoppedNextRunningStub
		advanced = "user wants worker running"
	)

	BeforeEach(func() {
		self = &stoppedNextStubState{name: "Stopped"}
		next = &stoppedNextRunningStub{name: "Running"}
	})

	Context("when IsBeingRemoved=true", func() {
		It("emits SignalNeedsRemoval and stays in self", func() {
			snap := makeSnap(true, false, config.DesiredStateRunning)

			res := helpers.StoppedNext(self, snap, next, advanced)

			Expect(res.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
			Expect(res.State).To(BeIdenticalTo(fsmv2.State[any, any](self)))
			Expect(res.Action).To(BeNil())
			Expect(res.Reason).To(ContainSubstring("removal"))
		})

		It("takes precedence over IsDisabled and Config-stopped", func() {
			snap := makeSnap(true, true, config.DesiredStateStopped)

			res := helpers.StoppedNext(self, snap, next, advanced)

			Expect(res.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
			Expect(res.State).To(BeIdenticalTo(fsmv2.State[any, any](self)))
		})
	})

	Context("when ShouldStop()=false (running, not removed, not disabled)", func() {
		It("advances to nextState with the supplied reason", func() {
			snap := makeSnap(false, false, config.DesiredStateRunning)

			res := helpers.StoppedNext(self, snap, next, advanced)

			Expect(res.Signal).To(Equal(fsmv2.SignalNone))
			Expect(res.State).To(BeIdenticalTo(fsmv2.State[any, any](next)))
			Expect(res.Action).To(BeNil())
			Expect(res.Reason).To(Equal(advanced))
		})

		It("advances even when config state is empty (defaults to running)", func() {
			snap := makeSnap(false, false, "")

			res := helpers.StoppedNext(self, snap, next, advanced)

			Expect(res.Signal).To(Equal(fsmv2.SignalNone))
			Expect(res.State).To(BeIdenticalTo(fsmv2.State[any, any](next)))
		})
	})

	Context("when IsDisabled=true (transient parent-disable)", func() {
		It("stays in self with no signal", func() {
			snap := makeSnap(false, true, config.DesiredStateRunning)

			res := helpers.StoppedNext(self, snap, next, advanced)

			Expect(res.Signal).To(Equal(fsmv2.SignalNone))
			Expect(res.State).To(BeIdenticalTo(fsmv2.State[any, any](self)))
			Expect(res.Action).To(BeNil())
			Expect(res.Reason).To(Equal("worker is stopped"))
		})
	})

	Context("when Config.GetState()==stopped (user-driven stop)", func() {
		It("stays in self with no signal", func() {
			snap := makeSnap(false, false, config.DesiredStateStopped)

			res := helpers.StoppedNext(self, snap, next, advanced)

			Expect(res.Signal).To(Equal(fsmv2.SignalNone))
			Expect(res.State).To(BeIdenticalTo(fsmv2.State[any, any](self)))
			Expect(res.Action).To(BeNil())
			Expect(res.Reason).To(Equal("worker is stopped"))
		})
	})
})
