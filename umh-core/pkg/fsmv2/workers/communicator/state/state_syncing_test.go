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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
)

func TestSyncingState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SyncingState Suite")
}

var _ = Describe("SyncingState", func() {
	var (
		stateObj *state.SyncingState
	)

	BeforeEach(func() {
		stateObj = &state.SyncingState{}
		_ = stateObj
	})

	PIt("should sync messages and transition to appropriate state", func() {
		// Tests pending: Need registry pattern updates
	})
})

var _ = Describe("SyncingState Circuit Breaker", func() {
	It("applies backoff delay based on ConsecutiveErrors", func() {
		// Arrange: State with accumulated errors
		observed := snapshot.CommunicatorObservedState{
			ConsecutiveErrors: 5,
			Authenticated:     true,
		}

		syncingState := &state.SyncingState{}

		// Act: Get backoff delay
		delay := syncingState.GetBackoffDelay(observed)

		// Assert: Delay increases with errors (exponential backoff capped at 60s)
		// 5 errors → 2^5 = 32 seconds
		Expect(delay).To(BeNumerically(">=", 10*time.Second))
		Expect(delay).To(BeNumerically("<=", 60*time.Second))
	})

	It("returns zero delay when no errors", func() {
		observed := snapshot.CommunicatorObservedState{
			ConsecutiveErrors: 0,
			Authenticated:     true,
		}

		syncingState := &state.SyncingState{}
		delay := syncingState.GetBackoffDelay(observed)

		Expect(delay).To(Equal(time.Duration(0)))
	})

	It("caps backoff at 60 seconds for many errors", func() {
		observed := snapshot.CommunicatorObservedState{
			ConsecutiveErrors: 100, // Many errors
			Authenticated:     true,
		}

		syncingState := &state.SyncingState{}
		delay := syncingState.GetBackoffDelay(observed)

		// 2^100 seconds would be huge, but should be capped at 60s
		Expect(delay).To(Equal(60 * time.Second))
	})
})
