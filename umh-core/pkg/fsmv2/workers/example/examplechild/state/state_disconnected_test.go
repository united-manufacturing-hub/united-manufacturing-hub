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

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
)

var _ = Describe("DisconnectedState (examplechild)", func() {
	var s *state.DisconnectedState

	BeforeEach(func() {
		s = &state.DisconnectedState{}
	})

	It("should return PhaseRunningDegraded LifecyclePhase", func() {
		Expect(s.LifecyclePhase()).To(Equal(config.PhaseRunningDegraded))
	})

	It("transitions to TryingToStop when parent stops the child", func() {
		snap := makeChildSnapshot(config.DesiredStateStopped, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToStopState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
	})

	It("transitions to TryingToStop on shutdown request", func() {
		snap := makeChildSnapshot(config.DesiredStateRunning, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToStopState{}))
	})

	It("transitions to TryingToConnect when parent still wants child running", func() {
		snap := makeChildSnapshot(config.DesiredStateRunning, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToConnectState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
	})

	It("returns a stable String() name", func() {
		Expect(s.String()).To(Equal("Disconnected"))
	})
})
