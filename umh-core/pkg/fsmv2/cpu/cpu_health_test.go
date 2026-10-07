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
		Expect(status.Verdict.State).To(Equal(cpuhealth.StateDegraded),
			"this spec needs a degraded verdict to have anything to map")

		health := monitorSpec.Health(CPUConfig{}, status)
		Expect(health.Degraded).To(BeTrue())
		Expect(health.Reason).To(Equal(status.Message),
			"the composed customer message is the reason an operator sees")
	})

	It("keeps the worker healthy when Decide judged the cgroup healthy", func() {
		Expect(monitorSpec.Health).NotTo(BeNil(),
			"the spec must wire a health check, or only a poll error can degrade this worker")

		d := newDeps(newTickSampler(quietTick(0), quietTick(1)), 4, 2)

		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Verdict.State).To(Equal(cpuhealth.StateHealthy),
			"this spec needs a healthy verdict to have anything to map")
		Expect(status.Details.UsageRingActive).To(BeTrue(),
			"this spec needs the usage measured, or the worker is still starting up")

		health := monitorSpec.Health(CPUConfig{}, status)
		Expect(health.Degraded).To(BeFalse())
		Expect(health.Reason).To(Equal(status.Message),
			"the composed customer message is the reason an operator sees")
	})

	It("degrades the worker while the CPU usage is not measured yet, and turns healthy once it is", func() {
		d := newDeps(newTickSampler(quietTick(0), quietTick(1)), 4, 2)

		starting, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(starting.Details.UsageRingActive).To(BeFalse(),
			"this spec needs a first tick whose usage is not measured yet")

		health := monitorSpec.Health(CPUConfig{}, starting)
		Expect(health.Degraded).To(BeTrue())
		Expect(health.Reason).To(HavePrefix("CPU: starting up."))
		Expect(starting.Verdict.State).NotTo(Equal(cpuhealth.StateDegraded),
			"a degraded verdict needs a cause, and nothing has fired")

		measured, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(monitorSpec.Health(CPUConfig{}, measured).Degraded).To(BeFalse())
	})
})

// quietTick is a sample on a container limited to 2 of 4 cores where every
// signal is readable and none fires. The i-th tick is i seconds after
// sampleAt, so the engine sees distinct ticks.
func quietTick(i int) cpuhealth.Sample {
	return cpuhealth.Sample{
		Timestamp:    sampleAt.Add(time.Duration(i) * time.Second),
		Quota:        diagnosis.Known(2),
		LogicalCpus:  diagnosis.Known(4),
		HostCpus:     diagnosis.Known(4),
		NrPeriods:    diagnosis.Known(100 * float64(i+1)),
		NrThrottled:  diagnosis.Known(0),
		UsageCores:   diagnosis.Known(0.5),
		Pressure:     diagnosis.Known(0),
		Steal:        diagnosis.Known(0),
		HostBusy:     diagnosis.Known(0.5),
		PsiAvailable: true,
		CpuScope:     cpuhealth.ScopeHost,
	}
}
