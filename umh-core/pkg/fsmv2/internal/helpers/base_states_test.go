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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

var _ = Describe("Phase-Specific Base Types", func() {
	DescribeTable("should return correct LifecyclePhase",
		func(base interface{ LifecyclePhase() config.LifecyclePhase }, expected config.LifecyclePhase) {
			Expect(base.LifecyclePhase()).To(Equal(expected))
		},
		Entry("StartingBase", helpers.StartingBase{}, config.PhaseStarting),
		Entry("RunningHealthyBase", helpers.RunningHealthyBase{}, config.PhaseRunningHealthy),
		Entry("RunningDegradedBase", helpers.RunningDegradedBase{}, config.PhaseRunningDegraded),
		Entry("StoppingBase", helpers.StoppingBase{}, config.PhaseStopping),
		Entry("StoppedBase", helpers.StoppedBase{}, config.PhaseStopped),
	)

	Describe("embedding in state structs", func() {
		type mockStartingState struct {
			helpers.StartingBase
		}

		type mockRunningState struct {
			helpers.RunningHealthyBase
		}

		type mockDegradedState struct {
			helpers.RunningDegradedBase
		}

		type mockStoppingState struct {
			helpers.StoppingBase
		}

		type mockStoppedState struct {
			helpers.StoppedBase
		}

		It("provides LifecyclePhase() to embedded struct for StartingBase", func() {
			state := mockStartingState{}
			Expect(state.LifecyclePhase()).To(Equal(config.PhaseStarting))
		})

		It("provides LifecyclePhase() to embedded struct for RunningHealthyBase", func() {
			state := mockRunningState{}
			Expect(state.LifecyclePhase()).To(Equal(config.PhaseRunningHealthy))
		})

		It("provides LifecyclePhase() to embedded struct for RunningDegradedBase", func() {
			state := mockDegradedState{}
			Expect(state.LifecyclePhase()).To(Equal(config.PhaseRunningDegraded))
		})

		It("provides LifecyclePhase() to embedded struct for StoppingBase", func() {
			state := mockStoppingState{}
			Expect(state.LifecyclePhase()).To(Equal(config.PhaseStopping))
		})

		It("provides LifecyclePhase() to embedded struct for StoppedBase", func() {
			state := mockStoppedState{}
			Expect(state.LifecyclePhase()).To(Equal(config.PhaseStopped))
		})
	})
})
