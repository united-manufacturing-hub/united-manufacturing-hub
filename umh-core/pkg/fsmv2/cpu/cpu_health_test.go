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

package fsmv2cpu

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// These specs read monitorSpec.Health rather than healthFromStatus, because the
// claim is about what the framework calls: calling the function directly would
// still pass with the wiring removed.
var _ = Describe("the worker's own health", func() {
	It("degrades the worker when Decide judged the cgroup degraded", func() {
		Expect(monitorSpec.Health).NotTo(BeNil(),
			"the spec must wire a health check, or only a poll error can degrade this worker")

		// Pressure fires above the mark on the first sample, so this tick is
		// degraded without any window warm-up.
		d := newDeps(fixedSampler(cpuhealth.Sample{
			Timestamp:    time.Now(),
			Quota:        diagnosis.Known(0),
			NrPeriods:    diagnosis.Known(1),
			Pressure:     diagnosis.Known(0.9),
			HostBusy:     diagnosis.Known(0.5),
			Virtualized:  true,
			PsiAvailable: true,
		}), 4, 0)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Verdict).To(Equal(string(cpuhealth.StateDegraded)),
			"this spec needs a degraded verdict to have anything to map")

		health := monitorSpec.Health(CPUConfig{}, status)
		Expect(health.Degraded).To(BeTrue())
		Expect(health.Reason).To(Equal(status.Message),
			"the composed customer message is the reason an operator sees")
	})

	It("keeps the worker healthy when Decide judged the cgroup healthy", func() {
		Expect(monitorSpec.Health).NotTo(BeNil(),
			"the spec must wire a health check, or only a poll error can degrade this worker")

		d := newDeps(fixedSampler(cpuhealth.Sample{
			Timestamp: time.Now(),
			Quota:     diagnosis.Known(2),
			// Every signal present and quiet: nothing fires.
			NrPeriods:   diagnosis.Known(1),
			NrThrottled: diagnosis.Known(0),
			UsageUsec:   diagnosis.Known(5000000),
			Pressure:    diagnosis.Known(0),
			Steal:       diagnosis.Known(0),
			HostBusy:    diagnosis.Known(0.5),
			Virtualized: false,
		}), 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Verdict).To(Equal(string(cpuhealth.StateHealthy)),
			"this spec needs a healthy verdict to have anything to map")

		health := monitorSpec.Health(CPUConfig{}, status)
		Expect(health.Degraded).To(BeFalse())
		Expect(health.Reason).To(Equal(status.Message),
			"the composed customer message is the reason an operator sees")
	})
})
