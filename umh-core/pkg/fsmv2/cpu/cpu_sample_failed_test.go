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
	"io/fs"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// cpu.stat is the one read whose failure voids the whole sample: the other five
// drop a single signal and leave the measurement usable. The verb says which
// happened, so a reader knows from the issue title whether there is any
// measurement at all.
var _ = Describe("cpu.stat reports under a verb that says what its failure cost", func() {
	statPath := cgroupBase + "/cpu.stat"

	It("uses sample_failed when the read itself failed", func() {
		events, _, _ := build(map[string]error{
			statPath: &fs.PathError{Op: "open", Path: statPath, Err: syscall.ENOENT},
		})

		Expect(msgs(events)).To(ConsistOf("cpu::sample_failed::cpu_stat::enoent"),
			"one event: the four reads after cpu.stat never happened, so they have nothing to report")
	})

	It("retires the prose warning that used to cover this", func() {
		// The old message was a sentence, which grouped separately from every
		// other event under this feature tag and told a reader nothing the
		// structured event does not.
		events, _, _ := build(map[string]error{
			statPath: &fs.PathError{Op: "open", Path: statPath, Err: syscall.ENOENT},
		})

		for _, m := range msgs(events) {
			Expect(m).NotTo(ContainSubstring("startup cgroup snapshot failed"),
				"the prose warning must be gone, not emitted alongside the new event")
			Expect(m).NotTo(ContainSubstring(" "), "message %q is prose, not an identifier", m)
		}
	})

	It("uses read_failed when the file read fine but carried no usage figure", func() {
		// A zero-byte cpu.stat returns no error: parseCounter treats a missing
		// key as absent rather than malformed. So the sample survives, there is
		// no usage rate, and the verb must say the read failed rather than that
		// the sample did.
		events, _, _ := build(map[string]error{})
		Expect(msgs(events)).To(BeEmpty(), "precondition: the healthy fixture is quiet")

		events2 := buildWithFiles(map[string][]byte{statPath: []byte("")})
		Expect(msgs(events2)).To(ConsistOf("cpu::read_failed::cpu_stat::empty"))
	})

	It("uses the same reason for a key with no value, and lets the raw text tell them apart", func() {
		// One token for both, because diagnosis.Reading carries a single
		// presence bit and cannot distinguish them. The raw text on the event
		// is what shows which case it was.
		events := buildWithFiles(map[string][]byte{statPath: []byte("usage_usec\n")})

		Expect(msgs(events)).To(ConsistOf("cpu::read_failed::cpu_stat::empty"))
		Expect((*events)[0].Fields).To(HaveKeyWithValue("cpu_stat_raw", "usage_usec\n"),
			"the raw text is what distinguishes an empty file from a malformed one")
	})
})

// The acceptance test for the whole change. ENG-5810's bar: a real failure
// produces one Sentry event carrying enough to name the failing file and rule
// out the alternatives, with nobody running a command on the machine.
var _ = Describe("one event is enough to diagnose the machine", func() {
	It("names the failure and rules out every alternative cause", func() {
		cpuset := cgroupBase + "/cpuset.cpus.effective"
		events, _, _ := build(map[string]error{
			cpuset: &fs.PathError{Op: "open", Path: cpuset, Err: syscall.ENOENT},
		})

		Expect(*events).To(HaveLen(1), "one failure, one issue to triage")
		e := (*events)[0]

		By("naming which file failed and how")
		Expect(e.Msg).To(Equal("cpu::read_failed::cpuset_cpus_effective::enoent"))
		Expect(e.Fields).To(HaveKeyWithValue("path", cpuset))

		By("ruling out a broken mount, a wrong base, and cgroup v1")
		Expect(e.Fields).To(HaveKeyWithValue("cpu_stat_read", string("ok")),
			"a sibling read succeeding is what rules out a broken mount")
		Expect(e.Fields).To(HaveKeyWithValue("proc_self_cgroup_raw", "0::/\n"),
			"the v2-only path shape rules out cgroup v1")
		Expect(e.Fields).To(HaveKey("cgroup_base_entry_count"))
		Expect(e.Fields).To(HaveKeyWithValue("cgroup_base", cgroupBase))

		By("carrying the controller list, which is the conclusion")
		// A missing cpuset token here is the delegation finding. It ships raw
		// because the failing shape has never been observed: a parsed boolean
		// would answer only the question we thought to ask.
		Expect(e.Fields).To(HaveKeyWithValue("cgroup_controllers_raw", evidenceControllers))

		By("ruling out a permission problem and a parse bug")
		// enoent rather than eacces is what excludes both, and it is in the
		// message, so it is visible in the Sentry issue list without opening
		// the event.
		Expect(e.Msg).To(HaveSuffix("::enoent"))

		By("keeping every variable value out of the grouping key")
		Expect(strings.Count(e.Msg, "::")).To(Equal(3), "verb, file, reason — nothing else")
		Expect(e.Msg).NotTo(ContainSubstring("/"))
	})

	It("emits nothing at all from a healthy container", func() {
		// Paired with the case above deliberately: a zero-event assertion is
		// vacuous on its own, and passes against no implementation.
		events, _, _ := build(nil)

		Expect(msgs(events)).To(BeEmpty())
	})
})
