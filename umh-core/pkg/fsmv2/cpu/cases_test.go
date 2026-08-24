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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// caseNames is the roster: every case that must exist, in order. It is written
// out here rather than derived from Cases, because a spec that reads the list
// it is checking cannot notice a case going missing. Deleting or reordering an
// entry in Cases fails against this list.
//
// The order is a reading arc rather than an accident, so a new case belongs
// where the arc puts it and not at the end: a healthy machine first, then one
// too young to have measured anything, then the capacity story with its
// attribution pair, then pressure, then the CPU-limit cases, then steal, then
// the two machines that do not hold still, and last the two whose files cannot
// be read. The moving machines come after every steady one because they are
// not a new cause — they are the causes above happening over time, and a
// reader needs to know what one answer looks like before reading a sequence of
// them. The read failures close the set because they are where a reader stops
// expecting a verdict at all.
var caseNames = []string{
	"quiet-box",
	"starting-up",
	"host-full-not-us",
	"host-full-because-us",
	"plain-host-no-psi",
	"pressure-at-sixty",
	"at-the-baseline",
	"throttled",
	"limit-full",
	"machine-and-limit-full",
	"noisy-neighbour",
	"flicker",
	"recovery",
	"cannot-measure",
	"no-host-stats",
}

var _ = Describe("the named machine situations", func() {
	Describe("each situation, driven through the real sampler and engine", func() {
		for _, c := range Cases {
			It("answers "+c.Name+" the way the case states", func() {
				// A fresh Box and fresh deps for every case. The engine holds
				// each signal's 60-second window and its latch, so a shared one
				// would let an earlier case's history decide this verdict.
				box := fakebox.NewBox(cgroupBase, c.Box)
				d := NewDepsWithSampler(
					deps.Identity{ID: "cpu-cases", WorkerType: WorkerType},
					deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, deps.Identity{ID: "cpu-cases", WorkerType: WorkerType}),
					cpuhealth.NewLinuxSamplerWithClock(box.FS(), cgroupBase, box.Clock()),
				)

				// What a read of THIS machine has to produce. A case stating
				// a PollError describes a machine whose sample cannot be read
				// at all, so the demand is the error and the empty status; on
				// every other case it is a successful read.
				expectRead := func(status CPUStatus, err error) {
					if c.PollError != "" {
						Expect(err).To(MatchError(ContainSubstring(c.PollError)))
						Expect(status).To(Equal(CPUStatus{}),
							"a read that failed reports nothing, never a healthy zero")

						return
					}
					Expect(err).NotTo(HaveOccurred(), "every read of a servable box must succeed")
				}

				// Every read's verdict, in order, so that a case stating
				// VerdictStretches can be checked against what the whole
				// sequence did. A machine that could not be read contributes
				// nothing here, because it produced no verdicts.
				var (
					verdicts []string
					status   CPUStatus
					err      error
				)

				read := func() {
					status, err = Poll(context.Background(), d, CPUConfig{})
					expectRead(status, err)

					if c.PollError == "" {
						verdicts = append(verdicts, status.Verdict)
					}
				}

				// The reads of one stretch of ticks at one condition: one
				// tick and one read each. The reads at Box come first and
				// follow the read taken before any tick, which is why that
				// one is taken outside.
				runPhase := func(ticks int) {
					for i := 0; i < ticks; i++ {
						box.Tick(time.Second)
						read()
					}
				}

				read()
				runPhase(c.Ticks)

				// A phase changes the machine and then reads it. Set accrues
				// nothing itself, so the change reaches the reads through the
				// ticks below it and not through the counters already served.
				for _, p := range c.Phases {
					box.Set(p.Box)
					runPhase(p.Ticks)
				}

				// The answer fields are the whole assertion for a machine that
				// could be read. One that could not has already been asserted
				// on above, and has no status to compare against.
				if c.PollError != "" {
					return
				}

				// All five the case states, exactly. The verdict alone would
				// pass on a machine judged degraded for the wrong reason,
				// because the reason is only visible in the message and in
				// which signals answered. CPUStatus also carries Polls, which
				// no case states. On a read that succeeds it is Ticks + 1, so
				// asserting it would only restate the loop above. On a read
				// that fails it is 0, because Poll returns before the counter
				// moves. One rule does not cover both, so the field is left
				// unasserted rather than given two.
				Expect(status.Verdict).To(Equal(c.Verdict))
				Expect(status.Message).To(Equal(c.Message))
				Expect(status.SignalsCapable).To(Equal(c.SignalsCapable))
				Expect(status.SignalsMeasured).To(Equal(c.SignalsMeasured))
				Expect(status.RefusingAdmission).To(Equal(c.RefusingAdmission))

				// What the verdict did along the way, on the cases that state
				// it. Asserted last because it is the only claim here about
				// reads other than the judged one, and a case whose judged
				// answer is already wrong is better read as that.
				if len(c.VerdictStretches) > 0 {
					Expect(Stretches(verdicts)).To(Equal(c.VerdictStretches),
						"the verdict moved differently across the sequence than the case states")
				}
			})
		}
	})

	Describe("the set as a whole", func() {
		It("holds every named case once, in the stated order", func() {
			names := make([]string, 0, len(Cases))
			for _, c := range Cases {
				names = append(names, c.Name)
			}
			Expect(names).To(Equal(caseNames),
				"a case dropped, added or reordered must fail here rather than shrink the set quietly")
		})

		It("gives every case a name of its own", func() {
			seen := map[string]bool{}
			for _, c := range Cases {
				Expect(seen[c.Name]).To(BeFalse(), "duplicate case name: "+c.Name)
				seen[c.Name] = true
			}
		})

		It("says of every case what it exists to show", func() {
			for _, c := range Cases {
				Expect(c.Why).NotTo(BeEmpty(), "case "+c.Name+" states no reason to exist")
			}
		})

		It("makes every moving machine state what the verdict did while it moved", func() {
			// Nothing else here would catch a phased case that states only
			// its last answer. Such a case runs its whole sequence and
			// asserts one read of it, so it passes whether the verdict held
			// or flapped on every tick along the way — which is the one thing
			// a moving machine is in the set to say.
			//
			// Counted, and required to be non-zero below. This walk skips
			// every steady case, so a set that lost both moving machines
			// would leave it iterating nothing and passing, and the package
			// owning the latch claim would not notice its only evidence for
			// that claim had gone.
			moving := 0

			for _, c := range Cases {
				if len(c.Phases) == 0 {
					continue
				}

				moving++

				Expect(c.VerdictStretches).NotTo(BeEmpty(),
					"case "+c.Name+" moves its machine and states nothing about what the verdict did")

				for _, p := range c.Phases {
					Expect(p.Ticks).To(BeNumerically(">", 0),
						"case "+c.Name+" has a phase that changes the machine and never reads it")
				}
			}

			Expect(moving).To(BeNumerically(">", 0),
				"no case moves its machine, so this package no longer demonstrates that a held verdict survives a reading that moves")
		})
	})
})
