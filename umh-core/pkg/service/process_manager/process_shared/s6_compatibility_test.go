//go:build internal_process_manager
// +build internal_process_manager

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

package process_shared_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
)

var _ = Describe("S6 Compatibility Layer", func() {
	Describe("MapIPMStatusToS6State", func() {
		It("should map ServiceUp to running state", func() {
			result := process_shared.MapIPMStatusToS6State(process_shared.ServiceUp)
			Expect(result).To(Equal(process_shared.S6OperationalStateRunning))
		})

		It("should map ServiceDown to stopped state", func() {
			result := process_shared.MapIPMStatusToS6State(process_shared.ServiceDown)
			Expect(result).To(Equal(process_shared.S6OperationalStateStopped))
		})

		It("should map ServiceRestarting to starting state", func() {
			result := process_shared.MapIPMStatusToS6State(process_shared.ServiceRestarting)
			Expect(result).To(Equal(process_shared.S6OperationalStateStarting))
		})

		It("should map ServiceUnknown to unknown state", func() {
			result := process_shared.MapIPMStatusToS6State(process_shared.ServiceUnknown)
			Expect(result).To(Equal(process_shared.S6OperationalStateUnknown))
		})

		It("should map invalid status to unknown state", func() {
			result := process_shared.MapIPMStatusToS6State("invalid")
			Expect(result).To(Equal(process_shared.S6OperationalStateUnknown))
		})
	})

	Describe("MapS6StateToIPMStatus", func() {
		It("should map running state to ServiceUp", func() {
			result := process_shared.MapS6StateToIPMStatus(process_shared.S6OperationalStateRunning)
			Expect(result).To(Equal(process_shared.ServiceUp))
		})

		It("should map stopped state to ServiceDown", func() {
			result := process_shared.MapS6StateToIPMStatus(process_shared.S6OperationalStateStopped)
			Expect(result).To(Equal(process_shared.ServiceDown))
		})

		It("should map starting state to ServiceRestarting", func() {
			result := process_shared.MapS6StateToIPMStatus(process_shared.S6OperationalStateStarting)
			Expect(result).To(Equal(process_shared.ServiceRestarting))
		})

		It("should map stopping state to ServiceDown", func() {
			result := process_shared.MapS6StateToIPMStatus(process_shared.S6OperationalStateStopping)
			Expect(result).To(Equal(process_shared.ServiceDown))
		})

		It("should map unknown state to ServiceUnknown", func() {
			result := process_shared.MapS6StateToIPMStatus(process_shared.S6OperationalStateUnknown)
			Expect(result).To(Equal(process_shared.ServiceUnknown))
		})

		It("should map invalid state to ServiceUnknown", func() {
			result := process_shared.MapS6StateToIPMStatus("invalid")
			Expect(result).To(Equal(process_shared.ServiceUnknown))
		})
	})

	Describe("IsCompatibleTransition", func() {
		It("should allow valid stopped -> starting transition", func() {
			result := process_shared.IsCompatibleTransition(
				process_shared.S6OperationalStateStopped,
				process_shared.S6OperationalStateStarting,
			)
			Expect(result).To(BeTrue())
		})

		It("should allow valid starting -> running transition", func() {
			result := process_shared.IsCompatibleTransition(
				process_shared.S6OperationalStateStarting,
				process_shared.S6OperationalStateRunning,
			)
			Expect(result).To(BeTrue())
		})

		It("should allow valid running -> stopping transition", func() {
			result := process_shared.IsCompatibleTransition(
				process_shared.S6OperationalStateRunning,
				process_shared.S6OperationalStateStopping,
			)
			Expect(result).To(BeTrue())
		})

		It("should allow valid stopping -> stopped transition", func() {
			result := process_shared.IsCompatibleTransition(
				process_shared.S6OperationalStateStopping,
				process_shared.S6OperationalStateStopped,
			)
			Expect(result).To(BeTrue())
		})

		It("should reject invalid stopped -> running transition", func() {
			result := process_shared.IsCompatibleTransition(
				process_shared.S6OperationalStateStopped,
				process_shared.S6OperationalStateRunning,
			)
			Expect(result).To(BeFalse())
		})

		It("should reject invalid running -> starting transition", func() {
			result := process_shared.IsCompatibleTransition(
				process_shared.S6OperationalStateRunning,
				process_shared.S6OperationalStateStarting,
			)
			Expect(result).To(BeFalse())
		})

		It("should handle unknown states gracefully", func() {
			result := process_shared.IsCompatibleTransition("invalid", "invalid")
			Expect(result).To(BeFalse())
		})
	})
})
