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
	Describe("MakeState", func() {
		Context("with PrefixStopped", func() {
			It("returns 'stopped' regardless of suffix", func() {
				Expect(config.MakeState(config.PrefixStopped, "")).To(Equal("stopped"))
				Expect(config.MakeState(config.PrefixStopped, "connected")).To(Equal("stopped"))
				Expect(config.MakeState(config.PrefixStopped, "anything")).To(Equal("stopped"))
			})
		})

		Context("with PrefixTryingToStart", func() {
			It("concatenates prefix and suffix", func() {
				Expect(config.MakeState(config.PrefixTryingToStart, "connecting")).To(Equal("trying_to_start_connecting"))
				Expect(config.MakeState(config.PrefixTryingToStart, "authenticating")).To(Equal("trying_to_start_authenticating"))
			})
		})

		Context("with PrefixRunning", func() {
			It("concatenates prefix and suffix", func() {
				Expect(config.MakeState(config.PrefixRunning, "connected")).To(Equal("running_connected"))
				Expect(config.MakeState(config.PrefixRunning, "syncing")).To(Equal("running_syncing"))
			})
		})

		Context("with PrefixTryingToStop", func() {
			It("concatenates prefix and suffix", func() {
				Expect(config.MakeState(config.PrefixTryingToStop, "disconnecting")).To(Equal("trying_to_stop_disconnecting"))
				Expect(config.MakeState(config.PrefixTryingToStop, "cleanup")).To(Equal("trying_to_stop_cleanup"))
			})
		})

		Context("with empty suffix", func() {
			It("returns just the prefix for non-stopped states", func() {
				Expect(config.MakeState(config.PrefixRunning, "")).To(Equal("running_"))
				Expect(config.MakeState(config.PrefixTryingToStart, "")).To(Equal("trying_to_start_"))
				Expect(config.MakeState(config.PrefixTryingToStop, "")).To(Equal("trying_to_stop_"))
			})
		})
	})

	Describe("ValidateDesiredState", func() {
		Context("with valid desired states", func() {
			It("returns nil for 'running'", func() {
				Expect(config.ValidateDesiredState(config.DesiredStateRunning)).To(BeNil())
			})

			It("returns nil for 'stopped'", func() {
				Expect(config.ValidateDesiredState(config.DesiredStateStopped)).To(BeNil())
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

	Describe("GetLifecyclePhase", func() {
		It("returns 'stopped' for stopped state", func() {
			Expect(config.GetLifecyclePhase("stopped")).To(Equal(config.PrefixStopped))
		})

		It("returns PrefixTryingToStart for trying_to_start_* states", func() {
			Expect(config.GetLifecyclePhase("trying_to_start_connecting")).To(Equal(config.PrefixTryingToStart))
			Expect(config.GetLifecyclePhase("trying_to_start_authenticating")).To(Equal(config.PrefixTryingToStart))
		})

		It("returns PrefixRunning for running_* states", func() {
			Expect(config.GetLifecyclePhase("running_connected")).To(Equal(config.PrefixRunning))
			Expect(config.GetLifecyclePhase("running_syncing")).To(Equal(config.PrefixRunning))
		})

		It("returns PrefixTryingToStop for trying_to_stop_* states", func() {
			Expect(config.GetLifecyclePhase("trying_to_stop_disconnecting")).To(Equal(config.PrefixTryingToStop))
			Expect(config.GetLifecyclePhase("trying_to_stop_cleanup")).To(Equal(config.PrefixTryingToStop))
		})

		It("returns empty string for invalid states", func() {
			Expect(config.GetLifecyclePhase("")).To(Equal(""))
			Expect(config.GetLifecyclePhase("invalid")).To(Equal(""))
			Expect(config.GetLifecyclePhase("running")).To(Equal("")) // Missing underscore suffix
		})
	})

	Describe("IsOperational", func() {
		It("returns true for running_* states", func() {
			Expect(config.IsOperational("running_connected")).To(BeTrue())
			Expect(config.IsOperational("running_syncing")).To(BeTrue())
		})

		It("returns false for non-running states", func() {
			Expect(config.IsOperational("stopped")).To(BeFalse())
			Expect(config.IsOperational("trying_to_start_connecting")).To(BeFalse())
			Expect(config.IsOperational("trying_to_stop_disconnecting")).To(BeFalse())
		})
	})

	Describe("IsStopped", func() {
		It("returns true only for exactly 'stopped'", func() {
			Expect(config.IsStopped("stopped")).To(BeTrue())
		})

		It("returns false for all other states", func() {
			Expect(config.IsStopped("running_connected")).To(BeFalse())
			Expect(config.IsStopped("trying_to_start_connecting")).To(BeFalse())
			Expect(config.IsStopped("trying_to_stop_disconnecting")).To(BeFalse())
			Expect(config.IsStopped("")).To(BeFalse())
			Expect(config.IsStopped("stopped_")).To(BeFalse()) // Not exactly "stopped"
		})
	})

	Describe("IsTransitioning", func() {
		It("returns true for trying_to_start_* states", func() {
			Expect(config.IsTransitioning("trying_to_start_connecting")).To(BeTrue())
			Expect(config.IsTransitioning("trying_to_start_authenticating")).To(BeTrue())
		})

		It("returns true for trying_to_stop_* states", func() {
			Expect(config.IsTransitioning("trying_to_stop_disconnecting")).To(BeTrue())
			Expect(config.IsTransitioning("trying_to_stop_cleanup")).To(BeTrue())
		})

		It("returns false for stable states", func() {
			Expect(config.IsTransitioning("stopped")).To(BeFalse())
			Expect(config.IsTransitioning("running_connected")).To(BeFalse())
		})
	})
})
