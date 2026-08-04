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

// The sampler reads cpu.pressure's "some" avg60, divides the kernel's 0..100
// figure by 100 to get the 0..1 fraction the marks are denominated in, and
// remembers that PSI exists once seen. PSI presence is a STICKY bool set on the
// first successful cpu.pressure read and never cleared; THIS tick's read
// success is Pressure's own Reading, absent when the read fails that tick. The
// two can therefore differ, and this test drives the divergence: a fake
// filesystem that serves cpu.pressure on the first Read and fails it on the
// second, then requires PsiAvailable true AND Pressure absent on the second
// snapshot.
package cpuhealth_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("pressure", func() {
	const base = "/sys/fs/cgroup"

	It("remembers PSI exists once seen, while this tick's success is Pressure's own Reading", func() {
		ctx := context.Background()

		pressureReads := 0
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return []byte("usage_usec 5000000\n"), nil
			case base + "/cpu.max":
				return []byte("max 100000\n"), nil
			case base + "/cpu.pressure":
				pressureReads++
				if pressureReads == 1 {
					// some avg10=1.23 avg60=20.41 avg300=9.87 total=123456
					return []byte("some avg10=1.23 avg60=20.41 avg300=9.87 total=123456\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n"), nil
				}
				return nil, errors.New("permission denied")
			default:
				return nil, errors.New("unreadable")
			}
		}

		s := cpuhealth.NewCgroupSampler(fs, base)

		// First snapshot: cpu.pressure reads. The some avg60 lands as a 0..1
		// fraction — the kernel reports 0..100 and the marks are fractions.
		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		frac, present := first.Pressure.Get()
		Expect(present).To(BeTrue(), "a readable cpu.pressure some avg60 must be a present Pressure Reading")
		Expect(frac).To(Equal(20.41/100.0), "avg60 is 0..100; dividing by 100 gives the 0..1 fraction the marks use")
		Expect(first.PsiAvailable).To(BeTrue(), "PsiAvailable is set the first tick cpu.pressure reads successfully")

		// Second snapshot: the read FAILS, but PSI existence is sticky. The
		// per-tick read success is Pressure's own Reading, now absent — the two
		// facts must diverge.
		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, present = second.Pressure.Get()
		Expect(present).To(BeFalse(), "this tick's failed cpu.pressure read must leave Pressure absent")
		Expect(second.PsiAvailable).To(BeTrue(), "PSI presence is remembered across ticks; a read failure must not clear it")
	})

	It("treats a literal-zero avg60 as a present healthy 0.0, never as no-signal", func() {
		ctx := context.Background()

		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return []byte("usage_usec 5000000\n"), nil
			case base + "/cpu.max":
				return []byte("max 100000\n"), nil
			case base + "/cpu.pressure":
				// A healthy system's constant output: the some avg60 is exactly
				// 0.00. That is a PRESENT zero, not an absence — a guard that
				// treats frac == 0 as no-signal would silently unset every
				// healthy cgroup's Pressure while the sticky flag stays true.
				return []byte("some avg10=0.00 avg60=0.00 avg300=0.00 total=0\n"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}

		s := cpuhealth.NewCgroupSampler(fs, base)

		snapshot, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		frac, present := snapshot.Pressure.Get()
		Expect(present).To(BeTrue(), "a literal-zero avg60 is a present Pressure Reading — the healthy constant, not absence")
		Expect(frac).To(Equal(0.0), "the fraction for avg60=0.00 is exactly 0.0")
		Expect(snapshot.PsiAvailable).To(BeTrue(), "a successful read sets the sticky flag even when the value is zero")
	})

	It("leaves PsiAvailable false from the very first tick when cpu.pressure is unreadable", func() {
		ctx := context.Background()

		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return []byte("usage_usec 5000000\n"), nil
			case base + "/cpu.max":
				return []byte("max 100000\n"), nil
			default:
				// cpu.pressure is never served: a kernel without PSI, or a
				// cgroup-v1 host. This is the complement of "seen then failed"
				// — the never-seen corner of the sticky semantic.
				return nil, errors.New("permission denied")
			}
		}

		s := cpuhealth.NewCgroupSampler(fs, base)

		snapshot, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, present := snapshot.Pressure.Get()
		Expect(present).To(BeFalse(), "on a host without PSI, Pressure is absent from the start")
		Expect(snapshot.PsiAvailable).To(BeFalse(), "PsiAvailable stays false when cpu.pressure was never readable")
		quota, quotaPresent := snapshot.Quota.Get()
		Expect(quotaPresent).To(BeTrue(), "an unreadable cpu.pressure must not disturb cpu.max; Quota still reads")
		Expect(quota).To(Equal(0.0), "cpu.max 'max' is a definite no-limit")
	})

	It("reads an unparsable some avg60 as absent, never as a fabricated healthy 0.0", func() {
		ctx := context.Background()

		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return []byte("usage_usec 5000000\n"), nil
			case base + "/cpu.max":
				return []byte("max 100000\n"), nil
			case base + "/cpu.pressure":
				// Malformed avg60: the line parses, the fraction does not. An
				// unparsable avg60 must be no pressure this tick, like an
				// unparsable cpu.max is no-signal — not a present 0.0.
				return []byte("some avg10=1.23 avg60=abc avg300=9.87 total=123456\n"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}

		s := cpuhealth.NewCgroupSampler(fs, base)

		snapshot, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, present := snapshot.Pressure.Get()
		Expect(present).To(BeFalse(), "an unparsable some avg60 must leave Pressure absent, not fabricate a healthy 0.0")
		Expect(snapshot.PsiAvailable).To(BeFalse(), "an unparsable avg60 must not be counted as PSI presence; the sticky flag stays unset")
	})
})
