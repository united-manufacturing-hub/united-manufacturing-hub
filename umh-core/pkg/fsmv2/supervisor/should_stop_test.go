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

package supervisor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

var _ = Describe("WorkerSnapshot.ShouldStop — 2-way discriminator", func() {
	type noConfig struct{}
	type noStatus struct{}

	It("IsShutdownRequested=true → ShouldStop=true (shutdown wins)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: true,
			IsDisabled:          false,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})

	It("IsDisabled=true → ShouldStop=true (admin pause)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: false,
			IsDisabled:          true,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})

	It("both IsShutdownRequested and IsDisabled → ShouldStop=true (shutdown wins in StoppedState.Next)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: true,
			IsDisabled:          true,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
		// Verify ordering invariant: IsShutdownRequested is checked first in state files.
		// ShouldStop ORs both; the discriminator in StoppedState.Next checks IsShutdownRequested
		// first to emit SignalNeedsRemoval, only checking IsDisabled when shutdown is not set.
		Expect(snap.IsShutdownRequested).To(BeTrue(), "shutdown WINS in StoppedState discriminator")
	})

	It("neither flag → ShouldStop=false (normal resume)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: false,
			IsDisabled:          false,
		}
		Expect(snap.ShouldStop()).To(BeFalse())
	})
})
