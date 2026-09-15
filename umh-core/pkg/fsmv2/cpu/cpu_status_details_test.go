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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
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
				Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
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
			Expect(top).To(HaveKey("verdict"))
			Expect(top).NotTo(HaveKey("p95UsageCores"),
				"a named field keeps the evidence out of the namespace verdict and message share")

			var details map[string]json.RawMessage
			Expect(json.Unmarshal(top["details"], &details)).To(Succeed())

			Expect(string(details["p95UsageCores"])).To(Equal("0"),
				"a present Reading is a JSON number; {} means its value was dropped and null means it was lost as an absence")
		})
	})
})

// The stored document is what simple.Status marshals from a CPUStatus: the
// result's keys flattened to the top level, alongside the health verdict's
// reason and degraded. The CPU status reader in container_monitor loads that
// document back through fsmv2client, so a loader must read back the whole
// verdict.
var _ = Describe("the verdict on the stored document", func() {
	It("carries Decide's whole verdict through a store and load round trip", func() {
		// Pressure fires above its mark on the first sample, so Decide
		// returns degraded for this sample on the first tick, with an
		// attribution and a ranked cause, without any window warm-up. A
		// quiet sample returns healthy, and a healthy verdict carries no
		// attribution and no causes to observe.
		sample := cpuhealth.Sample{
			Timestamp:    time.Now(),
			Quota:        diagnosis.Known(0),
			NrPeriods:    diagnosis.Known(1),
			Pressure:     diagnosis.Known(0.9),
			HostBusy:     diagnosis.Known(0.5),
			Virtualized:  true,
			PsiAvailable: true,
		}

		// The expected verdict is what Decide returns for this sample. It
		// runs on a second engine, not the one Poll feeds, because Decide
		// advances the windows it reads.
		twin := newDeps(fixedSampler(sample), 4, 0)
		expected, _ := cpuhealth.Decide(twin.engine, sample, cpuhealth.DeriveEnvironment(sample))
		Expect(expected.State).To(Equal(cpuhealth.StateDegraded))
		Expect(expected.Attribution).NotTo(BeEmpty())
		Expect(expected.Causes).NotTo(BeEmpty())

		d := newDeps(fixedSampler(sample), 4, 0)
		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Verdict).To(Equal(expected),
			"after Poll the status carries Decide's whole verdict")

		// Store the poll result as the framework does: wrapped in
		// simple.Status.
		health := monitorSpec.Health(CPUConfig{}, status)
		stored, err := json.Marshal(simple.Status[CPUStatus]{
			Result:   status,
			Degraded: health.Degraded,
			Reason:   health.Reason,
		})
		Expect(err).NotTo(HaveOccurred())

		var back simple.Status[CPUStatus]
		Expect(json.Unmarshal(stored, &back)).To(Succeed(),
			"the stored document must load")
		Expect(back.Result.Verdict).To(Equal(expected),
			"a loader reads the whole verdict back, not only its state")
	})

	// The old bytes are written out literally because no code path still
	// produces them. This Describe block's opening comment says how the keys
	// are flattened.
	It("loads an older build's document whose verdict key holds the bare state string", func() {
		old := []byte(`{"verdict":"healthy","message":"CPU healthy.","details":{},"reason":"CPU healthy.","degraded":false}`)

		var back simple.Status[CPUStatus]
		Expect(json.Unmarshal(old, &back)).To(Succeed(),
			"a stored document written by an older build must still load")

		Expect(back.Result.Verdict).To(Equal(cpuhealth.Verdict{}),
			"a bare string carries no attribution and no causes, so the old document decodes as no verdict at all")
		Expect(back.Result.Message).To(Equal("CPU healthy."),
			"the rest of the document loads too")
	})

	// The degraded sibling of the same old shape: an older build stored a
	// degraded tick with simple.Status's Degraded flag set, and that flag —
	// not the verdict — carried the judgement.
	It("loads an older build's degraded document with an empty verdict, the framework Degraded flag carrying the degraded state", func() {
		old := []byte(`{"verdict":"degraded","message":"CPU degraded.","details":{},"reason":"CPU degraded.","degraded":true}`)

		var back simple.Status[CPUStatus]
		Expect(json.Unmarshal(old, &back)).To(Succeed(),
			"a stored document written by an older build must still load")

		Expect(back.Result.Verdict).To(Equal(cpuhealth.Verdict{}),
			"the degraded bare string decodes as no verdict at all, like every bare string")
		Expect(back.Degraded).To(BeTrue(),
			"the degraded case is carried by the framework Degraded flag, not by the verdict")
	})

	It("fails the document's load only when the verdict key holds a value that is neither a string, nor null, nor an object", func() {
		document := func(verdictKey string) []byte {
			return []byte(`{"verdict":` + verdictKey + `,"message":"CPU healthy.","details":{},"reason":"CPU healthy.","degraded":false}`)
		}

		garbage := document(`"bogus"`)
		var backGarbage simple.Status[CPUStatus]
		Expect(json.Unmarshal(garbage, &backGarbage)).To(Succeed(),
			"a bare string is the old shape whatever it spells")
		Expect(backGarbage.Result.Verdict).To(Equal(cpuhealth.Verdict{}),
			"nothing reads the string back in: no state is kept from it")

		null := document(`null`)
		var backNull simple.Status[CPUStatus]
		Expect(json.Unmarshal(null, &backNull)).To(Succeed(),
			"unmarshalling null into a State is a no-op that returns no error, so the bare-string branch takes it too")
		Expect(backNull.Result.Verdict).To(Equal(cpuhealth.Verdict{}),
			"null decodes as no verdict at all, like a bare string")

		for _, bad := range []struct {
			name  string
			value string
		}{
			{"number", `3`},
			{"bool", `true`},
			{"array", `[]`},
		} {
			var backBad simple.Status[CPUStatus]
			Expect(json.Unmarshal(document(bad.value), &backBad)).NotTo(Succeed(),
				"a "+bad.name+" is no shape a stored document holds in its verdict key")
		}
	})
})
