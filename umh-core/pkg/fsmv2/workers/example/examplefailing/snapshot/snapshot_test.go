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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
)

var _ = Describe("ExamplefailingConfig", func() {
	It("should expose the embedded BaseUserSpec.GetState default", func() {
		cfg := &snapshot.ExamplefailingConfig{}
		Expect(cfg.GetState()).To(Equal("running"))
	})

	Describe("GetMaxFailures", func() {
		It("defaults to 3 when unset", func() {
			cfg := &snapshot.ExamplefailingConfig{}
			Expect(cfg.GetMaxFailures()).To(Equal(3))
		})

		It("returns the configured value when positive", func() {
			cfg := &snapshot.ExamplefailingConfig{MaxFailures: 7}
			Expect(cfg.GetMaxFailures()).To(Equal(7))
		})
	})

	Describe("GetFailureCycles", func() {
		It("defaults to 1 when unset", func() {
			cfg := &snapshot.ExamplefailingConfig{}
			Expect(cfg.GetFailureCycles()).To(Equal(1))
		})

		It("returns the configured value when positive", func() {
			cfg := &snapshot.ExamplefailingConfig{FailureCycles: 4}
			Expect(cfg.GetFailureCycles()).To(Equal(4))
		})
	})

	Describe("GetRestartAfterFailures", func() {
		It("returns the configured restart threshold (0 = disabled)", func() {
			cfg := &snapshot.ExamplefailingConfig{}
			Expect(cfg.GetRestartAfterFailures()).To(Equal(0))

			cfg.RestartAfterFailures = 5
			Expect(cfg.GetRestartAfterFailures()).To(Equal(5))
		})
	})

	Describe("GetRecoveryDelayObservations", func() {
		It("returns the configured observation delay threshold", func() {
			cfg := &snapshot.ExamplefailingConfig{}
			Expect(cfg.GetRecoveryDelayObservations()).To(Equal(0))

			cfg.RecoveryDelayObservations = 2
			Expect(cfg.GetRecoveryDelayObservations()).To(Equal(2))
		})
	})
})

var _ = Describe("ExamplefailingStatus", func() {
	It("should default to zero-valued fields", func() {
		status := snapshot.ExamplefailingStatus{}
		Expect(status.ConnectionHealth).To(Equal(""))
		Expect(status.ConnectAttempts).To(Equal(0))
		Expect(status.RestartAfterFailures).To(Equal(0))
		Expect(status.TicksInConnectedState).To(Equal(0))
		Expect(status.CurrentCycle).To(Equal(0))
		Expect(status.TotalCycles).To(Equal(0))
		Expect(status.ObservationsSinceFailure).To(Equal(0))
		Expect(status.ShouldFail).To(BeFalse())
		Expect(status.AllCyclesComplete).To(BeFalse())
		Expect(status.RecoveryDelayActive).To(BeFalse())
	})
})
