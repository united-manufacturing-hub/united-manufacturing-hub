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

// The CPU scope. The sampler reports whether its logical CPU
// count describes EVERY CPU on the machine (ScopeHost) or only the ones this
// container may run on (ScopeAffinity), and ScopeUnknown when the machine's CPU
// count cannot be read — never a silent ScopeHost on a failing read, since a
// pinned idle container misread as host makes the host-headroom subtraction
// invalid. The machine's count is
// the number of per-CPU (cpu0, cpu1, …) lines in /proc/stat, the same bytes the
// host-signal read already consumes; the container's allowed set is read from
// the cgroup's effective cpuset, so both sides of the comparison ride the
// injectable filesystem and the scope is unit-testable without hardware.
package cpuhealth_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("CPU scope", func() {
	const base = "/sys/fs/cgroup"

	// A machine with four per-CPU lines: cpu0..cpu3, alongside the aggregate
	// "cpu " line the host signals come from. The count of these lines is the
	// machine's CPU count.
	fourCPUProcStat := []byte("cpu  100 20 80 250 55 0 0 10 1000 200\n" +
		"cpu0 0 0 0 0 0 0 0 0 0 0\n" +
		"cpu1 0 0 0 0 0 0 0 0 0 0\n" +
		"cpu2 0 0 0 0 0 0 0 0 0 0\n" +
		"cpu3 0 0 0 0 0 0 0 0 0 0\n" +
		"intr 0\n")

	// newScopeSampler serves a cgroup with cpu.stat and cpu.max so Read succeeds,
	// plus the caller-chosen /proc/stat (procErr makes that read fail) and the
	// caller-chosen effective cpuset (cpuset "" makes that read fail). Every other
	// path is unreadable so nothing unexpected is silently served.
	newScopeSampler := func(procStat []byte, procErr bool, cpuset string) cpuhealth.Sampler {
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return []byte("usage_usec 5000000\nnr_periods 0\nnr_throttled 0\n"), nil
			case base + "/cpu.max":
				return []byte("max 100000\n"), nil
			case "/proc/stat":
				if procErr {
					return nil, errors.New("unreadable")
				}
				return procStat, nil
			case base + "/cpuset.cpus.effective":
				if cpuset == "" {
					return nil, errors.New("unreadable")
				}
				return []byte(cpuset), nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		return cpuhealth.NewLinuxSampler(fs, base)
	}

	It("reports the CPU scope as host, affinity, or unknown against the machine's CPU count, and never assumes host on an unreadable count", func() {
		ctx := context.Background()

		// When the container's allowed set equals the machine's full set, the
		// logical count describes every machine CPU: ScopeHost.
		hostS, err := newScopeSampler(fourCPUProcStat, false, "0-3").Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hostS.CpuScope).To(Equal(cpuhealth.ScopeHost),
			"an allowed set covering all machine CPUs must read ScopeHost")

		// When the container is pinned to a strict subset, the logical count
		// describes only those CPUs: ScopeAffinity. This is the shape that matters — an idle
		// pinned container must not be misread as host.
		affS, err := newScopeSampler(fourCPUProcStat, false, "0-1").Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(affS.CpuScope).To(Equal(cpuhealth.ScopeAffinity),
			"an allowed set pinning the container to 2 of 4 CPUs must read ScopeAffinity, not ScopeHost")

		// The machine count is kept on the snapshot, not only the comparison's
		// outcome: HostCpus is present and equals the four per-CPU
		// lines of the same /proc/stat read.
		machine, ok := affS.HostCpus.Get()
		Expect(ok).To(BeTrue(), "the machine's CPU count must be kept on the snapshot")
		Expect(machine).To(Equal(4.0), "four per-CPU lines are four machine CPUs")

		// The container's logical CPU count — the "2" in "pinned to 2 of 8" —
		// is kept beside the scope, from the same cpuset read, so the withheld
		// headroom sentence can name it.
		logical, ok := affS.LogicalCpus.Get()
		Expect(ok).To(BeTrue(), "the logical CPU count must be kept on the snapshot")
		Expect(logical).To(Equal(2.0), "an allowed set of two CPUs is a logical count of two")

		// When the machine's CPU count cannot be read, the scope is unknown —
		// NOT a silent ScopeHost. Defaulting a failed read to host would reinstate
		// the invalid subtraction by another route: the container is still pinned, we have just
		// stopped being able to say so.
		unkS, err := newScopeSampler(nil, true, "0-1").Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(unkS.CpuScope).To(Equal(cpuhealth.ScopeUnknown),
			"an unreadable machine CPU count must read ScopeUnknown, never ScopeHost")

		// The scope is deterministic and stable: startup facts do not move, so a
		// second read of the same sampler agrees with the first.
		again, err := newScopeSampler(fourCPUProcStat, false, "0-1").Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(again.CpuScope).To(Equal(affS.CpuScope),
			"the scope must be stable across reads of the same configuration")

		// A NON-CONTIGUOUS allowed set names the CPU count the scope exists for: static
		// CPU manager pins a pod to scattered CPUs, e.g. "0,2". A build that
		// parses only a single contiguous "lo-hi" range would drop these to
		// ScopeUnknown and lose the pinned-signal entirely.
		spread, err := newScopeSampler(fourCPUProcStat, false, "0,2").Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(spread.CpuScope).To(Equal(cpuhealth.ScopeAffinity),
			"a comma-separated affine set (0,2 on 4 CPUs) must read ScopeAffinity, not ScopeUnknown")

		// A comma-separated set that still covers every machine CPU reads
		// ScopeHost — comma-lists must not be conflated with affinity.
		spreadHost, err := newScopeSampler(fourCPUProcStat, false, "0,1,2,3").Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(spreadHost.CpuScope).To(Equal(cpuhealth.ScopeHost),
			"a comma-separated set covering all machine CPUs must read ScopeHost")
	})

	It("reports the scope as unknown when the machine count is known but the cpuset cannot be read — never a silent host", func() {
		ctx := context.Background()

		// The machine's CPU count IS readable (four per-CPU lines), but the
		// container's effective cpuset is unreadable (cpuset controller not
		// enabled, or a transient failure). We can see the machine has 4 CPUs but
		// not how many this container may run on, so the scope must be unknown —
		// a regression that defaulted this to ScopeHost would pass every earlier
		// case and reinstate the invalid subtraction.
		s, err := newScopeSampler(fourCPUProcStat, false, "").Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(s.CpuScope).To(Equal(cpuhealth.ScopeUnknown),
			"a readable machine count with an unreadable cpuset must read ScopeUnknown, never ScopeHost")
		machine, ok := s.HostCpus.Get()
		Expect(ok).To(BeTrue(), "the machine count still reads even when the cpuset does not")
		Expect(machine).To(Equal(4.0))
	})
})
