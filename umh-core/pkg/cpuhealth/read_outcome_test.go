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
// tell a missing file from an unreadable one. An opaque errors.New would
// classify as ReadError and the distinction under test would vanish.
func pathErr(path string, errno syscall.Errno) error {
	return &fs.PathError{Op: "open", Path: path, Err: errno}
}

// oneFile serves content/err for path and refuses everything else, so a reader
// that consulted the wrong file fails rather than passing on a neighbour's data.
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

// Each reader must say WHICH of the four causes it hit, not merely that it
// failed. Asserting only "an error occurred" would pass on a reader that
// returned errEmptyRead for a missing file.
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
	})

	// readQuota and readVirtualized return a ReadOutcome rather than an error:
	// neither has an `ok bool` to replace, so the outcome carries the reason
	// while the existing return carries presence. See the spec's R2 override.
	Describe("readQuota", func() {
		maxPath := base + "/cpu.max"

		It("reports ENOENT", func() {
			_, outcome := newCgroupSource(oneFile(maxPath, nil, pathErr(maxPath, syscall.ENOENT)), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadENOENT))
		})

		It("reports EACCES", func() {
			_, outcome := newCgroupSource(oneFile(maxPath, nil, pathErr(maxPath, syscall.EACCES)), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadEACCES))
		})

		It("reports a readable no-limit file as ok, never as a failure", func() {
			r, outcome := newCgroupSource(oneFile(maxPath, []byte("max 100000\n"), nil), base).readQuota(ctx)
			Expect(outcome).To(Equal(ReadOK), "content 'max' is a present no-limit, not a failed read")
			v, ok := r.Get()
			Expect(ok).To(BeTrue())
			Expect(v).To(Equal(0.0))
		})
	})

	// These four classifications had to be chosen during implementation and no
	// assertion covered them: deleting both sentinel cases from classifyRead's
	// switch left the whole suite green. Locked here so the mapping cannot be
	// silently rewired.
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
			// virtResolved short-circuits before any ReadFile, so the second
			// call opens nothing. This is the only producer of ReadNotAttempted
			// in the sampler today; without it the value would be declared and
			// never produced.
			h := newHostSource(oneFile("/proc/cpuinfo", []byte("flags\t\t: fpu hypervisor\n"), nil))

			virt, first := h.readVirtualized(ctx)
			Expect(virt).To(BeTrue())
			Expect(first).To(Equal(ReadOK))

			_, second := h.readVirtualized(ctx)
			Expect(second).To(Equal(ReadNotAttempted))
		})

		It("reports the cpuinfo read outcome", func() {
			_, outcome := newHostSource(oneFile("/proc/cpuinfo", nil, pathErr("/proc/cpuinfo", syscall.ENOENT))).readVirtualized(ctx)
			Expect(outcome).To(Equal(ReadENOENT))
		})
	})
})
