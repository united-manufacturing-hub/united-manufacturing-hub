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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/state"
)

var _ = Describe("DegradedState", func() {
	var (
		stateObj *state.DegradedState
		logger   *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		_ = logger
		stateObj = &state.DegradedState{}
	})

	Describe("Next", func() {
		Context("when sync recovers", func() {
			var snap snapshot.CommunicatorSnapshot

			BeforeEach(func() {
				snap = snapshot.CommunicatorSnapshot{
					Desired: snapshot.CommunicatorDesiredState{},
					Observed: snapshot.CommunicatorObservedState{
						Authenticated: true,
					},
				}
			})

			It("should transition to SyncingState", func() {
				nextState, _, _ := stateObj.Next(snap)
				Expect(nextState).To(BeAssignableToTypeOf(&state.SyncingState{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := stateObj.Next(snap)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := stateObj.Next(snap)
				Expect(action).To(BeNil())
			})
		})

		Context("when sync is still unhealthy", func() {
			var snap snapshot.CommunicatorSnapshot

			BeforeEach(func() {
				snap = snapshot.CommunicatorSnapshot{
					Desired: snapshot.CommunicatorDesiredState{},
					Observed: snapshot.CommunicatorObservedState{
						Authenticated: false,
					},
				}
			})

			It("should stay in DegradedState", func() {
				nextState, _, _ := stateObj.Next(snap)
				Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
			})

			It("should emit SyncAction to retry", func() {
				_, _, action := stateObj.Next(snap)
				Expect(action).NotTo(BeNil())
				Expect(action.Name()).To(Equal("sync"))
			})

			It("should not signal anything", func() {
				_, signal, _ := stateObj.Next(snap)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(stateObj.String()).To(Equal("Degraded"))
		})
	})

	Describe("Reason", func() {
		It("should return descriptive reason", func() {
			Expect(stateObj.Reason()).To(Equal("Sync is experiencing errors"))
		})
	})
})
