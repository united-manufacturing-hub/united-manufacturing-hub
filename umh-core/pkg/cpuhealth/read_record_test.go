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

package cpuhealth

import (
	"context"
	"io/fs"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// healthyFiles is what a working container serves, measured on a live box on
// 2026-09-03, so the healthy control is a machine we have seen.
func healthyFiles(base string) map[string][]byte {
	return map[string][]byte{
		base + "/cpu.stat":              []byte("usage_usec 11457863754\nuser_usec 9083319081\nsystem_usec 2374544673\nnr_periods 338962\nnr_throttled 903\nthrottled_usec 191447776\n"),
		base + "/cpu.max":               []byte("200000 100000\n"),
		base + "/cpu.pressure":          []byte("some avg10=0.00 avg60=0.00 avg300=0.00 total=132807436\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=97133489\n"),
		base + "/cpuset.cpus.effective": []byte("0-7\n"),
		"/proc/stat":                    []byte("cpu  1206136 0 377908 40066890 12729 0 97867 0 0 0\ncpu0 1 0 1 1 1 0 1 0 0 0\ncpu1 1 0 1 1 1 0 1 0 0 0\n"),
		"/proc/cpuinfo":                 []byte("flags\t\t: fpu vme hypervisor\n"),
	}
}

// fsServing serves files, overrides first. An override to nil-with-error is a
// failed read; a path in neither map returns ENOENT, so a reader that consulted
// an unexpected path fails rather than passing quietly.
func fsServing(files map[string][]byte, overrides map[string]error) filesystem.Service {
	mfs := filesystem.NewMockFileSystem()
	mfs.ReadFileFunc = func(_ context.Context, p string) ([]byte, error) {
		if err, ok := overrides[p]; ok {
			return nil, err
		}
		if content, ok := files[p]; ok {
			return content, nil
		}

		return nil, &fs.PathError{Op: "open", Path: p, Err: syscall.ENOENT}
	}

	return mfs
}

func outcomeFor(smp Sample, op ReadOp) ReadOutcome {
	for _, r := range smp.Reads {
		if r.Op == op {
			return r.Outcome
		}
	}

	return ReadOutcome("<op not recorded>")
}

var _ = Describe("the sample records what each read produced", func() {
	const base = "/sys/fs/cgroup"
	ctx := context.Background()

	read := func(overrides map[string]error) Sample {
		smp, _ := NewLinuxSampler(fsServing(healthyFiles(base), overrides), base).Read(ctx)

		return smp
	}

	// Structural, not a hand-written length: a new read operation added to
	// allReadOps without being recorded fails here.
	It("records exactly one entry per declared read operation", func() {
		smp := read(nil)

		Expect(smp.Reads).To(HaveLen(len(allReadOps)),
			"every declared read operation must appear, and none twice")

		seen := map[ReadOp]int{}
		for _, r := range smp.Reads {
			seen[r.Op]++
		}
		for _, op := range allReadOps {
			Expect(seen[op]).To(Equal(1), "read operation %q must be recorded exactly once", op)
		}
	})

	It("records ok for every read a healthy container serves", func() {
		smp := read(nil)

		for _, op := range []ReadOp{OpCPUStat, OpCPUMax, OpCPUPressure, OpCpusetCPUs, OpProcStat, OpProcCpuinfo} {
			Expect(outcomeFor(smp, op)).To(Equal(ReadOK), "healthy container, read %q", op)
		}
	})

	It("does not record the DMI reads at all", func() {
		// The DMI files are excluded from reporting: a missing product_name in a
		// container is normal, so an event would alert on correct absence.
		// readVirtualized therefore reports only its /proc/cpuinfo read.
		for _, op := range allReadOps {
			Expect(string(op)).NotTo(ContainSubstring("dmi"),
				"DMI reads are excluded from reporting, so they must not be recorded")
		}
	})

	It("records the failing read's cause and leaves its siblings ok", func() {
		cpuset := base + "/cpuset.cpus.effective"
		smp := read(map[string]error{cpuset: &fs.PathError{Op: "open", Path: cpuset, Err: syscall.ENOENT}})

		Expect(outcomeFor(smp, OpCpusetCPUs)).To(Equal(ReadENOENT))
		Expect(outcomeFor(smp, OpCPUStat)).To(Equal(ReadOK), "a cpuset failure must not be blamed on its siblings")
		Expect(outcomeFor(smp, OpProcStat)).To(Equal(ReadOK))
	})

	It("marks the cpuset read not_attempted when /proc/stat failed first", func() {
		// read.go nests the cpuset read inside the host read's success branch, so
		// a failed /proc/stat never opens the cpuset file. Recording a failure
		// for it would name the wrong file.
		smp := read(map[string]error{"/proc/stat": &fs.PathError{Op: "open", Path: "/proc/stat", Err: syscall.EACCES}})

		Expect(outcomeFor(smp, OpProcStat)).To(Equal(ReadEACCES))
		Expect(outcomeFor(smp, OpCpusetCPUs)).To(Equal(ReadNotAttempted))
	})

	It("marks every downstream read not_attempted when cpu.stat failed", func() {
		// cpu.stat is the one read whose failure returns from Read, so the reads
		// after it never happen — an event each, if not_attempted minted one.
		statPath := base + "/cpu.stat"
		smp := read(map[string]error{statPath: &fs.PathError{Op: "open", Path: statPath, Err: syscall.ENOENT}})

		Expect(outcomeFor(smp, OpCPUStat)).To(Equal(ReadENOENT))
		for _, op := range []ReadOp{OpProcStat, OpCpusetCPUs, OpProcCpuinfo, OpCPUMax} {
			Expect(outcomeFor(smp, op)).To(Equal(ReadNotAttempted),
				"read %q happens after cpu.stat, which returned early", op)
		}
	})

	It("records pressure before cpu.stat, since it is read first", func() {
		// readPSI runs ahead of readStat, so a cpu.stat failure must NOT mark
		// pressure not_attempted: it was already read.
		statPath := base + "/cpu.stat"
		smp := read(map[string]error{statPath: &fs.PathError{Op: "open", Path: statPath, Err: syscall.ENOENT}})

		Expect(outcomeFor(smp, OpCPUPressure)).To(Equal(ReadOK))
	})
})
