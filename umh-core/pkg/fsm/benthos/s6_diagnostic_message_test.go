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

package benthos

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	benthos_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

// Sentry groups issues by the error message, so any per-service or per-state value
// interpolated into this message mints a fresh issue for every bridge in every fleet.
// The message may therefore vary only by trigger, which has five bounded literals
// declared at the five logS6DirectoryState call sites.
var _ = Describe("S6 directory health diagnostic message", func() {
	It("varies only by trigger, never by service name or S6 state", func() {
		Expect(s6DirectoryHealthError("degraded_before_restart").Error()).
			To(Equal("S6 directory health issue: trigger=degraded_before_restart"))

		// The five call sites: actions.go:569, actions.go:594,
		// reconcile.go:434, reconcile.go:439, reconcile.go:485.
		triggers := []string{
			"IsBenthosS6Running_empty_state",
			"IsBenthosS6Stopped_empty_state",
			"degraded_before_restart",
			"degraded_restart_failed",
			"stopping_not_existing",
		}
		serviceNames := []string{
			"bridge-line-3-oven",
			"protocolconverter-press-07",
			"streamprocessor-oee-rollup",
		}

		for _, trigger := range triggers {
			message := s6DirectoryHealthError(trigger).Error()

			Expect(message).To(ContainSubstring("trigger=" + trigger))

			for _, serviceName := range serviceNames {
				Expect(message).NotTo(ContainSubstring(serviceName),
					"service name leaked into the message for trigger %s", trigger)
			}

			Expect(message).NotTo(ContainSubstring(benthos_service.S6StateNotExisting),
				"S6 state leaked into the message for trigger %s", trigger)
		}
	})
})
