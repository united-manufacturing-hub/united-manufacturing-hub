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

package container_monitor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
)

// The three CPU Prometheus series are read from whichever of the two filled the
// record -- the worker's evidence or the legacy fields -- and USE_FSMV2_CPU says
// which. Only one of them is ever populated, so a
// gate that read the wrong one publishes nothing rather than a wrong number —
// which is why these specs assert the ok flag as well as the values.
var _ = Describe("the CPU gauge source", func() {
	// Both sources staged on one record, disagreeing on purpose. The legacy
	// fields say 1000 mCPU over 4 cores; the evidence says 3.5 cores over 8.
	// Production never produces this record: the point is that each spec below
	// names which half it read.
	bothSourcesStaged := func() *models.CPU {
		usage, cores := 1000.0, 4

		return &models.CPU{
			TotalUsageMCpu: &usage,
			CoreCount:      &cores,
			CPUHealth: &models.CPUHealth{
				Details: cpuhealth.Details{
					AvgUsageCores:   3.5,
					LogicalCpus:     8,
					UsageRingActive: true,
				},
			},
		}
	}

	It("reads the fsmv2 evidence when the flag is on", func() {
		usageMCores, cores, ok := container_monitor.CPUGaugeInputs(bothSourcesStaged(), true)

		Expect(ok).To(BeTrue())
		Expect(usageMCores).To(Equal(3500.0))
		Expect(cores).To(Equal(8.0))
	})

	It("reads the legacy fields when the flag is off", func() {
		usageMCores, cores, ok := container_monitor.CPUGaugeInputs(bothSourcesStaged(), false)

		Expect(ok).To(BeTrue())
		Expect(usageMCores).To(Equal(1000.0))
		Expect(cores).To(Equal(4.0))
	})

	It("reports nothing measured when the flag is on and no evidence arrived", func() {
		// The legacy fields are populated and must NOT be read: under the flag
		// they are not the measurement in use.
		usage, cores := 1000.0, 4
		_, _, ok := container_monitor.CPUGaugeInputs(&models.CPU{
			TotalUsageMCpu: &usage,
			CoreCount:      &cores,
		}, true)

		Expect(ok).To(BeFalse())
	})

	It("reports nothing measured when the usage ring has not filled", func() {
		// AvgUsageCores is 0 here and 0 is a legitimate usage figure, so only
		// UsageRingActive separates "idle" from "not measured yet". Without this
		// gate a freshly started core records an idle box for its first tick.
		_, _, ok := container_monitor.CPUGaugeInputs(&models.CPU{
			CPUHealth: &models.CPUHealth{
				Details: cpuhealth.Details{LogicalCpus: 8, UsageRingActive: false},
			},
		}, true)

		Expect(ok).To(BeFalse())
	})

	It("reports nothing measured when the core count is absent, rather than dividing by it", func() {
		// LogicalCpus is 0 when the cpuset read failed. The load-percent gauge
		// divides by this, so publishing it would record +Inf.
		_, _, ok := container_monitor.CPUGaugeInputs(&models.CPU{
			CPUHealth: &models.CPUHealth{
				Details: cpuhealth.Details{AvgUsageCores: 3.5, LogicalCpus: 0, UsageRingActive: true},
			},
		}, true)

		Expect(ok).To(BeFalse())
	})

	It("reports nothing measured when a legacy tick measured no usage", func() {
		cores := 4
		_, _, ok := container_monitor.CPUGaugeInputs(&models.CPU{CoreCount: &cores}, false)

		Expect(ok).To(BeFalse())
	})
})
