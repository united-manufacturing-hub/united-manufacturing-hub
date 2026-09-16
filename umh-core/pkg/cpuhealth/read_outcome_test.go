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

// pathErr is what a real filesystem returns, and the shape errors.Is needs to
// tell missing from unreadable. An opaque errors.New classifies as ReadError,
// and the distinction under test vanishes.
func pathErr(path string, errno syscall.Errno) error {
	return &fs.PathError{Op: "open", Path: path, Err: errno}
}

// oneFile serves one path and refuses the rest, so a reader that consulted the
// wrong file fails rather than passing on a neighbour's data.
func oneFile(path string, content []byte, err error) filesystem.Service {
	mfs := filesystem.NewMockFileSystem()
	mfs.ReadFileFunc = func(_ context.Context, p string) ([]byte, error) {
		if p == path {
			return content, err
		}

		return nil, pathErr(p, syscall.ENOENT)
	}

	return mfs
}

// Each reader must say WHICH cause it hit, not merely that it failed: "an error
// occurred" passes on a reader returning errEmptyRead for a missing file.
var _ = Describe("a failed read reports its cause", func() {
	const base = "/sys/fs/cgroup"
	ctx := context.Background()

	Describe("readCpuset", func() {
		cpusetPath := base + "/cpuset.cpus.effective"

		It("reports ENOENT as a not-exist error", func() {
			_, err := newCgroupSource(oneFile(cpusetPath, nil, pathErr(cpusetPath, syscall.ENOENT)), base).readCpuset(ctx)
			Expect(err).To(MatchError(fs.ErrNotExist))
			Expect(err).NotTo(MatchError(fs.ErrPermission), "a missing file must not read as a permission problem")
		})

		It("reports EACCES as a permission error", func() {
			_, err := newCgroupSource(oneFile(cpusetPath, nil, pathErr(cpusetPath, syscall.EACCES)), base).readCpuset(ctx)
			Expect(err).To(MatchError(fs.ErrPermission))
			Expect(err).NotTo(MatchError(fs.ErrNotExist), "an unreadable file must not read as a missing one")
		})

		It("reports a zero-byte file as empty", func() {
			_, err := newCgroupSource(oneFile(cpusetPath, []byte(""), nil), base).readCpuset(ctx)
			Expect(err).To(MatchError(errEmptyRead))
		})

		It("reports unparsable content as unparsable", func() {
			_, err := newCgroupSource(oneFile(cpusetPath, []byte("0-abc\n"), nil), base).readCpuset(ctx)
			Expect(err).To(MatchError(errUnparsableRead))
			Expect(err).NotTo(MatchError(errEmptyRead), "content that is present but wrong is not an empty file")
		})

		It("returns the count and no error when the file reads", func() {
			count, err := newCgroupSource(oneFile(cpusetPath, []byte("0-7\n"), nil), base).readCpuset(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(8))
		})
	})

	Describe("readPSI", func() {
		psiPath := base + "/cpu.pressure"

		It("reports ENOENT as a not-exist error", func() {
			_, err := newCgroupSource(oneFile(psiPath, nil, pathErr(psiPath, syscall.ENOENT)), base).readPSI(ctx)
			Expect(err).To(MatchError(fs.ErrNotExist))
		})

		It("reports EACCES as a permission error", func() {
			_, err := newCgroupSource(oneFile(psiPath, nil, pathErr(psiPath, syscall.EACCES)), base).readPSI(ctx)
			Expect(err).To(MatchError(fs.ErrPermission))
		})

		It("reports an unparsable avg60 as unparsable", func() {
			_, err := newCgroupSource(oneFile(psiPath, []byte("some avg10=0.00 avg60=abc total=1\n"), nil), base).readPSI(ctx)
			Expect(err).To(MatchError(errUnparsableRead))
		})
	})

	Describe("the raw reads", func() {
		ctrlPath := base + "/cgroup.controllers"

		It("reports a blank file as empty, the same as the readers that parse", func() {
			// ReadEmpty is declared as "the file was read and held nothing", so a
			// raw read of a blank file is exactly that. A blank cgroup.controllers
			// means the parent delegated no controllers, which is the broken mount
			// this read exists to show; reporting ok would hide it behind the raw
			// string.
			text, outcome := newCgroupSource(oneFile(ctrlPath, []byte(""), nil), base).readControllers(ctx)

			Expect(outcome).To(Equal(ReadEmpty))
			Expect(text).To(BeEmpty())
		})

		It("keeps ok for a file that holds something", func() {
			text, outcome := newCgroupSource(oneFile(ctrlPath, []byte("cpu memory\n"), nil), base).readControllers(ctx)

			Expect(outcome).To(Equal(ReadOK))
			Expect(text).To(Equal("cpu memory\n"))
		})

		It("still names the cause when the file cannot be read", func() {
			_, outcome := newCgroupSource(oneFile(ctrlPath, nil, pathErr(ctrlPath, syscall.EACCES)), base).readControllers(ctx)

			Expect(outcome).To(Equal(ReadPermissionDenied))
		})
	})

	Describe("readStat", func() {
		statPath := base + "/cpu.stat"

		It("reports ENOENT as a not-exist error", func() {
			_, err := newCgroupSource(oneFile(statPath, nil, pathErr(statPath, syscall.ENOENT)), base).readStat(ctx)
			Expect(err).To(MatchError(fs.ErrNotExist))
		})

		It("reports a malformed counter value as unparsable", func() {
			// The sibling readers all map a parse failure to errUnparsableRead.
			// cpu.stat wrapping strconv's error instead would classify as
			// ReadError, the catch-all, and the Sentry facet that tells a
			// garbage counter from an I/O failure would say nothing.
			_, err := newCgroupSource(oneFile(statPath, []byte("usage_usec notanumber\n"), nil), base).readStat(ctx)
			Expect(err).To(MatchError(errUnparsableRead))
			Expect(classifyRead(err)).To(Equal(ReadUnparsable))
		})

		It("keeps strconv's detail alongside the cause", func() {
			_, err := newCgroupSource(oneFile(statPath, []byte("usage_usec notanumber\n"), nil), base).readStat(ctx)
			Expect(err.Error()).To(ContainSubstring("usage_usec"), "the message must still name which counter")
			Expect(err.Error()).To(ContainSubstring("notanumber"), "and the text that would not parse")
		})

		It("treats an absent key as absent, never as malformed", func() {
			stat, err := newCgroupSource(oneFile(statPath, []byte("nr_periods 5\n"), nil), base).readStat(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, ok := stat.Usage.Get()
			Expect(ok).To(BeFalse())
		})
	})

	Describe("readHost", func() {
		It("reports ENOENT as a not-exist error", func() {
			_, _, _, _, err := newHostSource(oneFile("/proc/stat", nil, pathErr("/proc/stat", syscall.ENOENT))).readHost(ctx)
			Expect(err).To(MatchError(fs.ErrNotExist))
		})

		It("reports EACCES as a permission error", func() {
			_, _, _, _, err := newHostSource(oneFile("/proc/stat", nil, pathErr("/proc/stat", syscall.EACCES))).readHost(ctx)
			Expect(err).To(MatchError(fs.ErrPermission))
		})

		It("reports a truncated aggregate line as unparsable", func() {
			_, _, _, _, err := newHostSource(oneFile("/proc/stat", []byte("cpu  1 2 3\ncpu0 1 2 3\n"), nil)).readHost(ctx)
			Expect(err).To(MatchError(errUnparsableRead))
		})

		// strconv.ParseFloat accepts "NaN", "Inf" and "+Inf" as valid floats, so a
		// counter holding one parses and would leave readHost reporting ReadOK on
		// a total no arithmetic can use.
		// https://pkg.go.dev/strconv#ParseFloat
		It("reports a NaN counter as unparsable", func() {
			_, _, _, _, err := newHostSource(oneFile("/proc/stat", []byte("cpu  1 2 3 4 5 6 7 NaN 0 0\ncpu0 1 2 3 4 5 6 7 8 0 0\n"), nil)).readHost(ctx)
			Expect(err).To(MatchError(errUnparsableRead))
		})

		It("reports an infinite counter as unparsable", func() {
			_, _, _, _, err := newHostSource(oneFile("/proc/stat", []byte("cpu  +Inf 2 3 4 5 6 7 8 0 0\ncpu0 1 2 3 4 5 6 7 8 0 0\n"), nil)).readHost(ctx)
			Expect(err).To(MatchError(errUnparsableRead))
		})
	})

	Describe("readQuota", func() {
		maxPath := base + "/cpu.max"

		It("reports ENOENT", func() {
			_, outcome := newCgroupSource(oneFile(maxPath, nil, pathErr(maxPath, syscall.ENOENT)), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadMissing))
		})

		It("reports EACCES", func() {
			_, outcome := newCgroupSource(oneFile(maxPath, nil, pathErr(maxPath, syscall.EACCES)), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadPermissionDenied))
		})

		It("reports a readable no-limit file as ok, never as a failure", func() {
			r, outcome := newCgroupSource(oneFile(maxPath, []byte("max 100000\n"), nil), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadOK), "content 'max' is a present no-limit, not a failed read")
			v, ok := r.Limit.Get()
			Expect(ok).To(BeTrue())
			Expect(v).To(Equal(0.0))
		})
	})

	// classifyRead maps these two through sentinel errors rather than an errno,
	// so they are the cases a rewiring can drop without any syscall changing.
	Describe("readQuota's non-errno causes", func() {
		maxPath := base + "/cpu.max"

		It("reports a zero-byte file as empty, not unparsable", func() {
			_, outcome := newCgroupSource(oneFile(maxPath, []byte(""), nil), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadEmpty))
		})

		It("reports a whitespace-only file as empty", func() {
			_, outcome := newCgroupSource(oneFile(maxPath, []byte("  \n"), nil), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadEmpty))
		})

		It("reports a non-numeric quota as unparsable", func() {
			_, outcome := newCgroupSource(oneFile(maxPath, []byte("abc 100000\n"), nil), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadUnparsable))
		})

		It("reports a non-positive period as unparsable, since it cannot be a divisor", func() {
			_, outcome := newCgroupSource(oneFile(maxPath, []byte("100000 0\n"), nil), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadUnparsable))
		})
	})

	Describe("readVirtualized", func() {
		It("reports not_attempted once the fact is already resolved", func() {
			// virtResolved short-circuits before any ReadFile, so the second call
			// opens nothing. It is the only reader that returns
			// ReadNotAttempted; seedReads is what puts it on every other entry.
			h := newHostSource(oneFile("/proc/cpuinfo", []byte("flags\t\t: fpu hypervisor\n"), nil))

			virt, first := h.readVirtualized(ctx)
			Expect(virt).To(BeTrue())
			Expect(first).To(Equal(ReadOK))

			_, second := h.readVirtualized(ctx)
			Expect(second).To(Equal(ReadNotAttempted))
		})

		It("reports the cpuinfo read outcome", func() {
			_, outcome := newHostSource(oneFile("/proc/cpuinfo", nil, pathErr("/proc/cpuinfo", syscall.ENOENT))).readVirtualized(ctx)
			Expect(outcome).To(Equal(ReadMissing))
		})
	})
})
