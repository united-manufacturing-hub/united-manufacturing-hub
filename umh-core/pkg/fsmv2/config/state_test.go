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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("State Functions", func() {
	Describe("ValidateDesiredState", func() {
		Context("with valid desired states", func() {
			It("returns nil for 'running'", func() {
				Expect(config.ValidateDesiredState(config.DesiredStateRunning)).To(Succeed())
			})

			It("returns nil for 'stopped'", func() {
				Expect(config.ValidateDesiredState(config.DesiredStateStopped)).To(Succeed())
			})
		})

		Context("with invalid desired states", func() {
			It("returns error for empty string", func() {
				err := config.ValidateDesiredState("")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid desired state"))
				Expect(err.Error()).To(ContainSubstring("only 'stopped' or 'running' are allowed"))
			})

			It("returns error with helpful hint for 'starting'", func() {
				err := config.ValidateDesiredState("starting")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Use 'running' for components that should be active"))
			})

			It("returns error with helpful hint for 'active'", func() {
				err := config.ValidateDesiredState("active")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Use 'running' for components that should be active"))
			})

			It("returns error with helpful hint for 'connected'", func() {
				err := config.ValidateDesiredState("connected")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Use 'running' for components that should be active"))
			})

			It("returns error with helpful hint for 'running_connected'", func() {
				err := config.ValidateDesiredState("running_connected")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Use 'running' for components that should be active"))
			})

			It("returns error with helpful hint for 'stopping'", func() {
				err := config.ValidateDesiredState("stopping")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Use 'stopped' for components that should be inactive"))
			})

			It("returns error with helpful hint for 'inactive'", func() {
				err := config.ValidateDesiredState("inactive")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Use 'stopped' for components that should be inactive"))
			})

			It("returns error with helpful hint for 'disconnected'", func() {
				err := config.ValidateDesiredState("disconnected")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Use 'stopped' for components that should be inactive"))
			})

			It("returns generic hint for unrecognized states", func() {
				err := config.ValidateDesiredState("unknown_state")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Use 'running' for active components or 'stopped' for inactive ones"))
			})
		})
	})

	Describe("IsValidDesiredState", func() {
		It("returns true for 'running'", func() {
			Expect(config.IsValidDesiredState(config.DesiredStateRunning)).To(BeTrue())
		})

		It("returns true for 'stopped'", func() {
			Expect(config.IsValidDesiredState(config.DesiredStateStopped)).To(BeTrue())
		})

		It("returns false for empty string", func() {
			Expect(config.IsValidDesiredState("")).To(BeFalse())
		})

		It("returns false for observed states", func() {
			Expect(config.IsValidDesiredState("running_connected")).To(BeFalse())
			Expect(config.IsValidDesiredState("trying_to_start_connecting")).To(BeFalse())
			Expect(config.IsValidDesiredState("trying_to_stop_disconnecting")).To(BeFalse())
		})

		It("returns false for common mistakes", func() {
			Expect(config.IsValidDesiredState("starting")).To(BeFalse())
			Expect(config.IsValidDesiredState("stopping")).To(BeFalse())
			Expect(config.IsValidDesiredState("active")).To(BeFalse())
			Expect(config.IsValidDesiredState("inactive")).To(BeFalse())
		})
	})

	Describe("ParseLifecyclePhase", func() {
		It("returns PhaseStopped for 'stopped'", func() {
			Expect(config.ParseLifecyclePhase("stopped")).To(Equal(config.PhaseStopped))
		})

		It("returns PhaseStarting for trying_to_start_* states", func() {
			Expect(config.ParseLifecyclePhase("trying_to_start_connecting")).To(Equal(config.PhaseStarting))
			Expect(config.ParseLifecyclePhase("trying_to_start_authenticating")).To(Equal(config.PhaseStarting))
		})

		It("returns PhaseRunningHealthy for running_healthy_* states", func() {
			Expect(config.ParseLifecyclePhase("running_healthy_connected")).To(Equal(config.PhaseRunningHealthy))
			Expect(config.ParseLifecyclePhase("running_healthy_syncing")).To(Equal(config.PhaseRunningHealthy))
		})

		It("returns PhaseRunningDegraded for running_degraded_* states", func() {
			Expect(config.ParseLifecyclePhase("running_degraded_partial")).To(Equal(config.PhaseRunningDegraded))
			Expect(config.ParseLifecyclePhase("running_degraded_unhealthy_children")).To(Equal(config.PhaseRunningDegraded))
		})

		It("returns PhaseStopping for trying_to_stop_* states", func() {
			Expect(config.ParseLifecyclePhase("trying_to_stop_disconnecting")).To(Equal(config.PhaseStopping))
			Expect(config.ParseLifecyclePhase("trying_to_stop_cleanup")).To(Equal(config.PhaseStopping))
		})

		It("returns PhaseUnknown for unknown_* states", func() {
			Expect(config.ParseLifecyclePhase("unknown_error")).To(Equal(config.PhaseUnknown))
			Expect(config.ParseLifecyclePhase("unknown_initializing")).To(Equal(config.PhaseUnknown))
		})

		It("returns PhaseUnknown for unrecognized patterns", func() {
			Expect(config.ParseLifecyclePhase("")).To(Equal(config.PhaseUnknown))
			Expect(config.ParseLifecyclePhase("invalid")).To(Equal(config.PhaseUnknown))
			Expect(config.ParseLifecyclePhase("running_connected")).To(Equal(config.PhaseUnknown)) // Old format without healthy/degraded
		})
	})

	Describe("LifecyclePhase enum", func() {
		Describe("Prefix()", func() {
			It("returns correct prefix for each phase", func() {
				Expect(config.PhaseUnknown.Prefix()).To(Equal("unknown_"))
				Expect(config.PhaseStopped.Prefix()).To(Equal("stopped"))
				Expect(config.PhaseStarting.Prefix()).To(Equal("trying_to_start_"))
				Expect(config.PhaseRunningHealthy.Prefix()).To(Equal("running_healthy_"))
				Expect(config.PhaseRunningDegraded.Prefix()).To(Equal("running_degraded_"))
				Expect(config.PhaseStopping.Prefix()).To(Equal("trying_to_stop_"))
			})
		})

		Describe("String()", func() {
			It("returns human-readable names", func() {
				Expect(config.PhaseUnknown.String()).To(Equal("Unknown"))
				Expect(config.PhaseStopped.String()).To(Equal("Stopped"))
				Expect(config.PhaseStarting.String()).To(Equal("Starting"))
				Expect(config.PhaseRunningHealthy.String()).To(Equal("RunningHealthy"))
				Expect(config.PhaseRunningDegraded.String()).To(Equal("RunningDegraded"))
				Expect(config.PhaseStopping.String()).To(Equal("Stopping"))
			})
		})

		Describe("IsHealthy()", func() {
			It("returns true ONLY for PhaseRunningHealthy", func() {
				Expect(config.PhaseRunningHealthy.IsHealthy()).To(BeTrue())
			})

			It("returns false for all other phases", func() {
				Expect(config.PhaseUnknown.IsHealthy()).To(BeFalse())
				Expect(config.PhaseStopped.IsHealthy()).To(BeFalse())
				Expect(config.PhaseStarting.IsHealthy()).To(BeFalse())
				Expect(config.PhaseRunningDegraded.IsHealthy()).To(BeFalse())
				Expect(config.PhaseStopping.IsHealthy()).To(BeFalse())
			})

			It("returns false for PhaseRunningDegraded (operational but NOT healthy)", func() {
				Expect(config.PhaseRunningDegraded.IsHealthy()).To(BeFalse())
				Expect(config.PhaseRunningDegraded.IsOperational()).To(BeTrue())
			})
		})

		Describe("IsOperational()", func() {
			It("returns true for both running phases", func() {
				Expect(config.PhaseRunningHealthy.IsOperational()).To(BeTrue())
				Expect(config.PhaseRunningDegraded.IsOperational()).To(BeTrue())
			})

			It("returns false for non-running phases", func() {
				Expect(config.PhaseUnknown.IsOperational()).To(BeFalse())
				Expect(config.PhaseStopped.IsOperational()).To(BeFalse())
				Expect(config.PhaseStarting.IsOperational()).To(BeFalse())
				Expect(config.PhaseStopping.IsOperational()).To(BeFalse())
			})
		})

		Describe("IsTransitioning()", func() {
			It("returns true for starting and stopping phases", func() {
				Expect(config.PhaseStarting.IsTransitioning()).To(BeTrue())
				Expect(config.PhaseStopping.IsTransitioning()).To(BeTrue())
			})

			It("returns false for stable phases", func() {
				Expect(config.PhaseUnknown.IsTransitioning()).To(BeFalse())
				Expect(config.PhaseStopped.IsTransitioning()).To(BeFalse())
				Expect(config.PhaseRunningHealthy.IsTransitioning()).To(BeFalse())
				Expect(config.PhaseRunningDegraded.IsTransitioning()).To(BeFalse())
			})
		})

		Describe("IsStopped()", func() {
			It("returns true only for PhaseStopped", func() {
				Expect(config.PhaseStopped.IsStopped()).To(BeTrue())
			})

			It("returns false for all other phases", func() {
				Expect(config.PhaseUnknown.IsStopped()).To(BeFalse())
				Expect(config.PhaseStarting.IsStopped()).To(BeFalse())
				Expect(config.PhaseRunningHealthy.IsStopped()).To(BeFalse())
				Expect(config.PhaseRunningDegraded.IsStopped()).To(BeFalse())
				Expect(config.PhaseStopping.IsStopped()).To(BeFalse())
			})
		})

		Describe("IsDegraded()", func() {
			It("returns true only for PhaseRunningDegraded", func() {
				Expect(config.PhaseRunningDegraded.IsDegraded()).To(BeTrue())
			})

			It("returns false for all other phases", func() {
				Expect(config.PhaseUnknown.IsDegraded()).To(BeFalse())
				Expect(config.PhaseStopped.IsDegraded()).To(BeFalse())
				Expect(config.PhaseStarting.IsDegraded()).To(BeFalse())
				Expect(config.PhaseRunningHealthy.IsDegraded()).To(BeFalse())
				Expect(config.PhaseStopping.IsDegraded()).To(BeFalse())
			})
		})
	})
})
