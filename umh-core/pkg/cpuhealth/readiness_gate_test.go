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

// The per-signal readiness gate is Ready and nothing else. NoInstrument (a
// bare-metal box has no steal instrument at all) and NoneReady (the window is
// too thin to trust) are different reasons for the same fact: this tick has no
// usable reading, and printing a confident number for either states a figure
// that was never measured.
//
// The bare-metal half is covered by the verdict spec's StealSignalReady
// assertion. This file covers the thin-window half, which nothing else sees: on
// the first tick of a fresh engine every window holds one sample, and a gate
// written "not NoInstrument" instead of "== Ready" reports the throttle and
// steal signals ready off that single sample.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the per-signal readiness gate on a thin window", func() {
	It("should report a NoneReady signal as not ready, on the first tick of a fresh engine", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())

		// Every capability granted, so nothing here resolves NoInstrument: the
		// instruments exist, they simply have one sample to fold. That is what
		// separates this spec from the bare-metal one — it fails only for a gate
		// that accepts NoneReady.
		env := diagnosis.NewEnvironment(HasLimit, HasVirtualization, HasPressureStats)

		smp := Sample{
			Timestamp:    time.Now(),
			CpuScope:     ScopeHost,
			Virtualized:  true,
			PsiAvailable: true,
			Quota:        diagnosis.Known(2),
			LogicalCpus:  diagnosis.Known(4),
			HostCpus:     diagnosis.Known(4),
			Pressure:     diagnosis.Known(0.1),
			Steal:        diagnosis.Known(0),
			HostBusy:     diagnosis.Known(0.5),
			UsageCores:   diagnosis.Known(0.2),
			NrPeriods:    diagnosis.Known(100),
			NrThrottled:  diagnosis.Known(2),
		}

		_, sig := Decide(engine, smp, env)

		// The premise, read off the same windows Decide judged: one sample is
		// not a window, so both signals resolve NoneReady rather than
		// NoInstrument. Without this the assertions below would still pass under
		// a gate that only excludes NoInstrument, and the spec would certify the
		// defect.
		t := cpuTable(4, 2.0)
		_, _, _, throttleAvail := engine.Select(signalNamed(t, "throttling"), env)
		Expect(throttleAvail).To(Equal(diagnosis.NoneReady),
			"one sample cannot fill the throttle window, and the instrument exists")
		_, _, _, stealAvail := engine.Select(signalNamed(t, "steal"), env)
		Expect(stealAvail).To(Equal(diagnosis.NoneReady),
			"one sample cannot fill the steal window, and a virtualized box has the instrument")

		// So the message layer must be told it has no number to print.
		Expect(sig.ThrottleSignalReady).To(BeFalse(),
			"a NoneReady throttle signal must not be reported ready: the message would publish a throttle ratio off one sample")
		Expect(sig.StealSignalReady).To(BeFalse(),
			"a NoneReady steal signal must not be reported ready: the message would publish a steal budget off one sample")
	})
})
