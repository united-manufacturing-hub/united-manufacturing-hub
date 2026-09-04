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
	"os"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// The controller list a working container serves. The token that matters is
// "cpuset": its absence is the conclusion the whole report exists to deliver,
// so this is the positive control for a discriminator whose failing shape
// nobody has observed yet.
const healthyControllers = "cpuset cpu io memory hugetlb pids rdma\n"

// dirEntries fakes a ReadDir result of n entries. Only the count is read.
func dirEntries(n int) []os.DirEntry {
	entries := make([]os.DirEntry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, fakeDirEntry{})
	}

	return entries
}

type fakeDirEntry struct{}

func (fakeDirEntry) Name() string               { return "entry" }
func (fakeDirEntry) IsDir() bool                { return false }
func (fakeDirEntry) Type() os.FileMode          { return 0 }
func (fakeDirEntry) Info() (os.FileInfo, error) { return nil, nil }

// evidenceFS serves the healthy files plus the three evidence sources, with
// per-path overrides for failure cases.
func evidenceFS(base string, overrides map[string]error, entryCount int, dirErr error) filesystem.Service {
	files := healthyFiles(base)
	files[base+"/cgroup.controllers"] = []byte(healthyControllers)
	files["/proc/self/cgroup"] = []byte("0::/\n")

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
	mfs.ReadDirFunc = func(_ context.Context, _ string) ([]os.DirEntry, error) {
		if dirErr != nil {
			return nil, dirErr
		}

		return dirEntries(entryCount), nil
	}

	return mfs
}

var _ = Describe("the sample carries the surrounding evidence", func() {
	const base = "/sys/fs/cgroup"
	ctx := context.Background()

	read := func(overrides map[string]error, entryCount int, dirErr error) Sample {
		smp, _ := NewLinuxSampler(evidenceFS(base, overrides, entryCount, dirErr), base).Read(ctx)

		return smp
	}

	It("declares the three evidence operations alongside the six reported reads", func() {
		Expect(allReadOps).To(HaveLen(9))
		Expect(allReadOps).To(ContainElements(OpCgroupControllers, OpProcSelfCgroup, OpBaseDir))
	})

	// Provable property 8. These fields have no consumer until the report
	// exists, so without this they would be untested by construction: wrong
	// from the first commit, and trusted by whoever reads them in an incident.
	It("records every raw value byte for byte as the file served it", func() {
		smp := read(nil, 85, nil)

		Expect(smp.ControllersRaw).To(Equal(healthyControllers),
			"the controller list must arrive unparsed and untrimmed of meaning")
		Expect(smp.ProcSelfCgroupRaw).To(Equal("0::/\n"))
		Expect(smp.CPUMaxRaw).To(Equal("200000 100000\n"))
		Expect(smp.CPUStatRaw).To(ContainSubstring("usage_usec 11457863754"),
			"the cpu.stat text is what tells a reader whether an absent usage figure was an empty file or a malformed one")
		Expect(smp.BaseEntryCount).To(Equal(85))
	})

	It("marks the evidence reads ok when they succeed", func() {
		smp := read(nil, 85, nil)

		for _, op := range []ReadOp{OpCgroupControllers, OpProcSelfCgroup, OpBaseDir} {
			Expect(outcomeFor(smp, op)).To(Equal(ReadOK), "evidence read %q", op)
		}
	})

	It("carries the reason and an empty raw when an evidence read fails", func() {
		ctrl := base + "/cgroup.controllers"
		smp := read(map[string]error{ctrl: &fs.PathError{Op: "open", Path: ctrl, Err: syscall.EACCES}}, 85, nil)

		Expect(outcomeFor(smp, OpCgroupControllers)).To(Equal(ReadEACCES))
		Expect(smp.ControllersRaw).To(BeEmpty(),
			"a failed read must not leave stale or invented text in the raw field")
	})

	It("reports the directory read's own failure and a sentinel count", func() {
		smp := read(nil, 0, &fs.PathError{Op: "open", Path: base, Err: syscall.ENOENT})

		Expect(outcomeFor(smp, OpBaseDir)).To(Equal(ReadENOENT))
		Expect(smp.BaseEntryCount).To(Equal(-1),
			"zero entries is a real reading; an unread directory must not look like an empty one")
	})

	// The whole point of the discriminator: a controller list that has been read
	// successfully and simply lacks cpuset. Reporting it parsed would throw away
	// the shape, so the field must stay the raw string.
	It("keeps a controller list with no cpuset token exactly as served", func() {
		files := base + "/cgroup.controllers"
		mfs := filesystem.NewMockFileSystem()
		healthy := healthyFiles(base)
		healthy[files] = []byte("cpu io memory pids\n")
		healthy["/proc/self/cgroup"] = []byte("0::/\n")
		mfs.ReadFileFunc = func(_ context.Context, p string) ([]byte, error) {
			if c, ok := healthy[p]; ok {
				return c, nil
			}

			return nil, &fs.PathError{Op: "open", Path: p, Err: syscall.ENOENT}
		}
		mfs.ReadDirFunc = func(_ context.Context, _ string) ([]os.DirEntry, error) {
			return dirEntries(34), nil
		}

		smp, _ := NewLinuxSampler(mfs, base).Read(ctx)

		Expect(smp.ControllersRaw).To(Equal("cpu io memory pids\n"))
		Expect(outcomeFor(smp, OpCgroupControllers)).To(Equal(ReadOK),
			"a readable list missing cpuset is a successful read, not a failed one")
	})

	It("still gathers the evidence when cpu.stat returned early", func() {
		// The evidence is most needed exactly when a read failed, so it must not
		// sit behind the early return that a cpu.stat failure takes.
		statPath := base + "/cpu.stat"
		smp := read(map[string]error{statPath: &fs.PathError{Op: "open", Path: statPath, Err: syscall.ENOENT}}, 85, nil)

		Expect(smp.ControllersRaw).To(Equal(healthyControllers))
		Expect(smp.BaseEntryCount).To(Equal(85))
		Expect(outcomeFor(smp, OpCgroupControllers)).To(Equal(ReadOK))
	})
})
