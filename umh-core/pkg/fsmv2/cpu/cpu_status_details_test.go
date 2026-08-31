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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("CPUStatus carries the measured evidence", func() {
	It("hands Decide's whole Details to the status, not a subset of its fields", func() {
		// Four numbers that cannot stand in for one another, so a copy that
		// dropped or crossed a field moves at least one of them.
		d := newDeps(fixedSampler(cpuhealth.Sample{
			Timestamp:    time.Now(),
			Quota:        diagnosis.Known(2),
			LogicalCpus:  diagnosis.Known(4),
			HostCpus:     diagnosis.Known(8),
			NrPeriods:    diagnosis.Known(1),
			NrThrottled:  diagnosis.Known(0),
			UsageUsec:    diagnosis.Known(5000000),
			Pressure:     diagnosis.Known(0.1),
			Steal:        diagnosis.Known(0),
			HostBusy:     diagnosis.Known(0.5),
			Virtualized:  true,
			PsiAvailable: true,
			CpuScope:     cpuhealth.ScopeHost,
		}), 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())

		Expect(status.Details.LogicalCpus).To(Equal(4.0))
		Expect(status.Details.HostCpus).To(Equal(8.0))
		Expect(status.Details.CapacityCores).To(Equal(2.0), "the quota caps the capacity below the CPU count")
		Expect(status.Details.ReserveCores).To(BeNumerically(">", 0))
		Expect(status.Details.LimitApplies).To(BeTrue())
		Expect(status.Details.PressureApplies).To(BeTrue())
		Expect(status.Details.StealApplies).To(BeTrue())
		Expect(status.Details.HostHeadroomAvailable).To(BeTrue())
	})

	It("fills no evidence on a tick that could not measure", func() {
		d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
			return cpuhealth.Sample{}, context.DeadlineExceeded
		}}, 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).To(HaveOccurred())
		Expect(status.Details).To(Equal(cpuhealth.Details{}),
			"a failed read reports no evidence rather than a zero-valued measurement")
	})

	// The Reading fields are the risk: both of a Reading's fields are unexported,
	// so a Reading without its own marshaller encodes as {} and the number is
	// gone with no error. buildDetails fills no Reading today, so Poll cannot
	// stage that case and these specs build the status directly.
	Describe("on the wire", func() {
		// A present zero: the value an implementation could mistake for an absence.
		filled := func() CPUStatus {
			return CPUStatus{
				Verdict: string(cpuhealth.StateHealthy),
				Message: "CPU healthy.",
				Details: cpuhealth.Details{
					P95UsageCores: diagnosis.Known(0),
					ThrottleRatio: 0.1,
					LogicalCpus:   4,
					HostCpus:      8,
					LimitApplies:  true,
				},
			}
		}

		It("reads a marshalled status back with its evidence intact, Readings included", func() {
			b, err := json.Marshal(filled())
			Expect(err).NotTo(HaveOccurred())

			var back CPUStatus
			Expect(json.Unmarshal(b, &back)).To(Succeed())

			Expect(back.Details).To(Equal(filled().Details))

			v, ok := back.Details.P95UsageCores.Get()
			Expect(ok).To(BeTrue(), "a known zero is a value, not an absence")
			Expect(v).To(Equal(0.0))
		})

		It("nests the evidence under details, with a present Reading written as a number", func() {
			b, err := json.Marshal(filled())
			Expect(err).NotTo(HaveOccurred())

			var top map[string]json.RawMessage
			Expect(json.Unmarshal(b, &top)).To(Succeed())
			Expect(top).To(HaveKey("details"))
			Expect(top).NotTo(HaveKey("p95UsageCores"),
				"a named field keeps the evidence out of the namespace verdict and message share")

			var details map[string]json.RawMessage
			Expect(json.Unmarshal(top["details"], &details)).To(Succeed())

			Expect(string(details["p95UsageCores"])).To(Equal("0"),
				"a present Reading is a JSON number; {} means its value was dropped and null means it was lost as an absence")
		})
	})
})
