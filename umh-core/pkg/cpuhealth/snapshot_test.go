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

// One snapshot per tick: Read stamps a single Timestamp and takes every field
// off the same read; the whole snapshot is built from OUTSIDE pkg/diagnosis
// through Known and Unknown (so a Reading with no constructor cannot satisfy
// this file); DeriveEnvironment turns exactly three facts — Virtualized,
// whether Quota names a POSITIVE number, and the sticky PsiAvailable — into an
// Environment via NewEnvironment;
// and only an unreadable or unparseable cpu.stat fails the whole snapshot, every
// other source failing leaves one field Unknown() and returns the snapshot.
package cpuhealth_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("one snapshot per tick", func() {
	const base = "/sys/fs/cgroup"

	stat := []byte("usage_usec 5000000\nuser_usec 4000000\nsystem_usec 1000000\nnr_periods 10\nnr_throttled 2\n")
	procStat := "cpu  100 0 300 5000 0 0 10 0 0 0\ncpu0 50 0 150 2500 0 0 5 0 0 0\ncpu1 50 0 150 2500 0 0 5 0 0 0\n"

	// sampler returns a Sampler whose fake filesystem fails every source by
	// default, so each case opts in to exactly the reads it needs.
	sampler := func(mutate func(fs *filesystem.MockFileSystem)) cpuhealth.Sampler {
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return stat, nil
			case base + "/cpu.max":
				return []byte("200000 100000"), nil
			case base + "/cpu.pressure":
				return []byte("some avg10=1.00 avg60=2.00 avg300=3.00 total=0\n"), nil
			case base + "/cpuset.cpus.effective":
				return []byte("0-1"), nil
			case "/proc/stat":
				return []byte(procStat), nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		if mutate != nil {
			mutate(fs)
		}
		return cpuhealth.NewLinuxSampler(fs, base)
	}

	It("stamps one timestamp, builds the environment from two facts, and fails the whole snapshot only on cpu.stat", func() {
		// Spec 1 + 2: a real Read builds every field off one read, and the
		// snapshot is constructible from OUTSIDE pkg/diagnosis through Known and
		// Unknown. The single Timestamp is set once per tick.
		ctx := context.Background()
		smp, err := sampler(nil).Read(ctx)
		Expect(err).To(BeNil())
		Expect(smp.Timestamp.IsZero()).To(BeFalse())
		Expect(smp.UsageUsec).To(Equal(diagnosis.Known(5000000)))
		Expect(smp.NrPeriods).To(Equal(diagnosis.Known(10)))
		Expect(smp.NrThrottled).To(Equal(diagnosis.Known(2)))
		Expect(smp.Quota).To(Equal(diagnosis.Known(2)))
		Expect(smp.Pressure).To(Equal(diagnosis.Known(0.02)))

		// Spec 3: DeriveEnvironment reads exactly three facts and calls
		// NewEnvironment. A snapshot with Virtualized true and Quota at
		// Known(2) satisfies both capabilities; one with neither satisfies
		// neither; and the load-bearing third case — Quota at Known(0), a
		// PRESENT uncapped read — must NOT satisfy HasLimit (positivity, not
		// presence). The fourth case: PsiAvailable true satisfies
		// HasPressureStats, and its absence (the zero value) does not.
		both := diagnosis.Environment{}
		both = cpuhealth.DeriveEnvironment(cpuhealth.Sample{Timestamp: smp.Timestamp, Virtualized: true, Quota: diagnosis.Known(2)})
		Expect(both.Has(cpuhealth.HasVirtualization)).To(BeTrue())
		Expect(both.Has(cpuhealth.HasLimit)).To(BeTrue())

		neither := cpuhealth.DeriveEnvironment(cpuhealth.Sample{Timestamp: smp.Timestamp})
		Expect(neither.Has(cpuhealth.HasVirtualization)).To(BeFalse())
		Expect(neither.Has(cpuhealth.HasLimit)).To(BeFalse())

		uncapped := cpuhealth.DeriveEnvironment(cpuhealth.Sample{Timestamp: smp.Timestamp, Virtualized: true, Quota: diagnosis.Known(0)})
		Expect(uncapped.Has(cpuhealth.HasVirtualization)).To(BeTrue())
		Expect(uncapped.Has(cpuhealth.HasLimit)).To(BeFalse())

		psiless := cpuhealth.DeriveEnvironment(cpuhealth.Sample{Timestamp: smp.Timestamp})
		Expect(psiless.Has(cpuhealth.HasPressureStats)).To(BeFalse())
		withPsi := cpuhealth.DeriveEnvironment(cpuhealth.Sample{Timestamp: smp.Timestamp, PsiAvailable: true})
		Expect(withPsi.Has(cpuhealth.HasPressureStats)).To(BeTrue())

		// Spec 4: a fake filesystem whose cpu.pressure alone is missing returns
		// err == nil with Sample.Pressure absent and the cpu.stat fields still
		// present.
		noPressure := sampler(func(fs *filesystem.MockFileSystem) {
			fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
				if path == base+"/cpu.pressure" {
					return nil, errors.New("missing")
				}
				inner, err := stat, error(nil)
				switch path {
				case base + "/cpu.stat":
					inner, err = stat, nil
				case base + "/cpu.max":
					inner, err = []byte("200000 100000"), nil
				case base + "/cpuset.cpus.effective":
					inner, err = []byte("0-1"), nil
				case "/proc/stat":
					inner, err = []byte(procStat), nil
				default:
					inner, err = nil, errors.New("unreadable")
				}
				return inner, err
			}
		})
		smp2, err := noPressure.Read(ctx)
		Expect(err).To(BeNil())
		Expect(smp2.Pressure).To(Equal(diagnosis.Unknown()))
		Expect(smp2.UsageUsec).To(Equal(diagnosis.Known(5000000)))
		Expect(smp2.NrPeriods).To(Equal(diagnosis.Known(10)))
		Expect(smp2.NrThrottled).To(Equal(diagnosis.Known(2)))

		// Spec 4: a fake filesystem whose cpu.stat is missing returns a non-nil
		// error and no usable snapshot.
		noStat := sampler(func(fs *filesystem.MockFileSystem) {
			fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
				if path == base+"/cpu.stat" {
					return nil, errors.New("missing")
				}
				inner, err := stat, error(nil)
				if path == base+"/cpu.max" {
					inner, err = []byte("200000 100000"), nil
				}
				return inner, err
			}
		})
		_, err = noStat.Read(ctx)
		Expect(err).NotTo(BeNil())

		// Spec 4: a fake filesystem whose cpu.stat is readable but whose
		// value cannot be parsed (non-numeric usage_usec) also fails the
		// whole snapshot — a parse failure is not a silent no-signal tick.
		malformedStat := sampler(func(fs *filesystem.MockFileSystem) {
			fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
				switch path {
				case base + "/cpu.stat":
					return []byte("usage_usec abc\n"), nil
				case base + "/cpu.max":
					return []byte("200000 100000"), nil
				case base + "/cpu.pressure":
					return []byte("some avg10=1.00 avg60=2.00 avg300=3.00 total=0\n"), nil
				case base + "/cpuset.cpus.effective":
					return []byte("0-1"), nil
				case "/proc/stat":
					return []byte(procStat), nil
				default:
					return stat, nil
				}
			}
		})
		_, err = malformedStat.Read(ctx)
		Expect(err).NotTo(BeNil())

		// A present-but-unparsable COUNTER value is equally an unparsable
		// cpu.stat and must fail the whole snapshot (R3's stale "unavailable on
		// parse-failure" is superseded by R5 spec 4). The nr_throttled branch
		// was untested before this pin — a guard that silently treated a corrupt
		// counter as an unavailable field would reintroduce the corrupted-cpu.stat
		// ships-as-trusted defect.
		for _, corrupt := range []string{
			"usage_usec 5000000\nnr_periods abc\nnr_throttled 0\n",
			"usage_usec 5000000\nnr_periods 0\nnr_throttled abc\n",
		} {
			cc := sampler(func(fs *filesystem.MockFileSystem) {
				fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
					switch path {
					case base + "/cpu.stat":
						return []byte(corrupt), nil
					case base + "/cpu.max":
						return []byte("200000 100000"), nil
					case base + "/cpu.pressure":
						return []byte("some avg10=1.00 avg60=2.00 avg300=3.00 total=0\n"), nil
					case base + "/cpuset.cpus.effective":
						return []byte("0-1"), nil
					case "/proc/stat":
						return []byte(procStat), nil
					default:
						return stat, nil
					}
				}
			})
			_, err = cc.Read(ctx)
			Expect(err).NotTo(BeNil(),
				"a corrupt cpu.stat counter (%q) must fail the whole snapshot, not be read as unavailable", corrupt)
		}
	})
})
