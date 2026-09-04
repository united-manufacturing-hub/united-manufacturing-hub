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
	"io/fs"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A read can start failing at any point, not only at startup: a cgroup can be
// remounted, a container reconfigured, or a read can fail once and recover.
// Reporting only at construction leaves all of that silent.
var _ = Describe("a read that starts failing later is reported too", func() {
	cpuset := cgroupBase + "/cpuset.cpus.effective"
	ctx := context.Background()

	enoent := func(p string) error {
		return &fs.PathError{Op: "open", Path: p, Err: syscall.ENOENT}
	}

	It("reports a failure that begins after startup", func() {
		broken := false
		events, d := buildPollable(func(p string) error {
			if broken && p == cpuset {
				return enoent(p)
			}

			return nil
		})
		Expect(msgs(events)).To(BeEmpty(), "precondition: construction saw a healthy container")

		broken = true
		_, _ = Poll(ctx, d, CPUConfig{})

		Expect(msgs(events)).To(ConsistOf("cpu::read_failed::cpuset_cpus_effective::enoent"),
			"a read that only starts failing at tick 1 must still be reported")
	})

	It("reports a repeating failure once, not once per measurement", func() {
		// The worker samples once a second. Reporting per measurement would be
		// 86,400 events a day per instance per file, and the five-minute
		// debouncer would still let 288 through.
		// The failure must begin AFTER construction. Failing from the first
		// read would let construction emit the one event this asserts, and the
		// spec would pass with no per-tick reporting at all.
		broken := false
		events, d := buildPollable(func(p string) error {
			if broken && p == cpuset {
				return enoent(p)
			}

			return nil
		})
		Expect(msgs(events)).To(BeEmpty(), "precondition: construction was healthy")

		broken = true
		for range 40 {
			_, _ = Poll(ctx, d, CPUConfig{})
		}

		Expect(msgs(events)).To(HaveLen(1),
			"one cause, one event, however many measurements observed it")
	})

	It("does not report the same cause twice when construction already saw it", func() {
		// Construction and the tick loop share one gate. Two gates would report
		// a startup failure again on the first tick.
		events, d := buildPollable(func(p string) error {
			if p == cpuset {
				return enoent(p)
			}

			return nil
		})
		Expect(msgs(events)).To(HaveLen(1), "precondition: construction reported it")

		_, _ = Poll(ctx, d, CPUConfig{})

		Expect(msgs(events)).To(HaveLen(1), "the first tick must not repeat construction's event")
	})

	It("reports again when the cause changes on the same file", func() {
		// A machine whose situation changes is worth a new event: the pair is
		// new, so the gate does not hold it.
		eacces := false
		events, d := buildPollable(func(p string) error {
			if p != cpuset {
				return nil
			}
			if eacces {
				return &fs.PathError{Op: "open", Path: p, Err: syscall.EACCES}
			}

			return enoent(p)
		})
		Expect(msgs(events)).To(ConsistOf("cpu::read_failed::cpuset_cpus_effective::enoent"))

		eacces = true
		_, _ = Poll(ctx, d, CPUConfig{})

		Expect(msgs(events)).To(ConsistOf(
			"cpu::read_failed::cpuset_cpus_effective::enoent",
			"cpu::read_failed::cpuset_cpus_effective::eacces",
		), "a changed cause on the same file is a new fact")
	})

	It("stays silent while shutting down", func() {
		// A cancelled context makes every in-flight read fail, and those errors
		// classify as `error` because they are neither missing nor unreadable
		// files. Reporting them would emit up to six events on every graceful
		// shutdown of every instance.
		events, d := buildPollable(nil)
		Expect(msgs(events)).To(BeEmpty())

		stopping, cancel := context.WithCancel(context.Background())
		cancel()

		_, _ = Poll(stopping, d, CPUConfig{})

		Expect(msgs(events)).To(BeEmpty(),
			"shutdown is not a failure to report")
	})

	It("stays silent on a healthy container however long it runs", func() {
		events, d := buildPollable(nil)

		for range 40 {
			_, _ = Poll(ctx, d, CPUConfig{})
		}

		Expect(msgs(events)).To(BeEmpty())
	})
})
