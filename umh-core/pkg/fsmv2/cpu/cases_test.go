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

// cgroupBase is where a Box serves its cgroup files and where the sampler
// looks for them. One constant so the two cannot drift apart and leave every
// read failing.
const cgroupBase = "/sys/fs/cgroup"

// caseNames is the roster: every case that must exist, in order. It is written
// out here rather than derived from Cases, because a spec that reads the list
// it is checking cannot notice a case going missing. Deleting or reordering an
// entry in Cases fails against this list.
var caseNames = []string{
	"pressure-at-sixty",
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

				status, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred(), "the first read of a servable box must succeed")

				for i := 0; i < c.Ticks; i++ {
					box.Tick(time.Second)
					status, err = Poll(context.Background(), d, CPUConfig{})
					Expect(err).NotTo(HaveOccurred(), "every read of a servable box must succeed")
				}

				// All five the case states, exactly. The verdict alone would
				// pass on a machine judged degraded for the wrong reason,
				// because the reason is only visible in the message and in
				// which signals answered. CPUStatus also carries Polls, which
				// no case states: it is always Ticks + 1, so asserting it would
				// only restate the loop above.
				Expect(status.Verdict).To(Equal(c.Verdict))
				Expect(status.Message).To(Equal(c.Message))
				Expect(status.SignalsCapable).To(Equal(c.SignalsCapable))
				Expect(status.SignalsMeasured).To(Equal(c.SignalsMeasured))
				Expect(status.RefusingAdmission).To(Equal(c.RefusingAdmission))
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
	})
})
