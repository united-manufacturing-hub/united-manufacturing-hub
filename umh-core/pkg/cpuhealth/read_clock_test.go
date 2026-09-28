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

// The injected clock. NewLinuxSamplerWithClock builds a sampler that stamps
// every Sample from the clock it was handed, so a caller that moves the clock
// moves every stamp with it. A sampler still calling time.Now() stamps wall
// time, which cannot equal an instant pinned to 2020-03-14T15:09:26Z — the
// pinned-instant assertion below can only pass by reading the injected clock.
package cpuhealth_test

import (
	"context"
	"errors"
	"time"

	"github.com/benbjohnson/clock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("sampler-injected clock", func() {
	const base = "/sys/fs/cgroup"

	// Read tolerates the errors unserved paths return, so the sample succeeds
	// and the assertions land on the timestamp rather than an error path.
	readableFS := func() filesystem.Service {
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return []byte("usage_usec 5000000\nuser_usec 4000000\nsystem_usec 1000000\nnr_periods 10\nnr_throttled 2\n"), nil
			case base + "/cpu.max":
				return []byte("200000 100000"), nil
			case base + "/cpu.pressure":
				return []byte("some avg10=1.00 avg60=2.00 avg300=3.00 total=0\n"), nil
			case base + "/cpuset.cpus.effective":
				return []byte("0-1"), nil
			case "/proc/stat":
				return []byte("cpu  100 0 300 5000 0 0 10 0 0 0\ncpu0 50 0 150 2500 0 0 5 0 0 0\ncpu1 50 0 150 2500 0 0 5 0 0 0\n"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}

		return fs
	}

	It("stamps every Sample from the clock it was built with", func() {
		ctx := context.Background()
		clk := clock.NewMock()
		start := time.Date(2020, time.March, 14, 15, 9, 26, 0, time.UTC)
		clk.Set(start)

		s := cpuhealth.NewLinuxSamplerWithClock(readableFS(), base, clk)

		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Timestamp).To(Equal(start),
			"the first Sample must carry the mock's instant, not wall time")

		const d = 7 * time.Second
		clk.Add(d)

		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Timestamp.Sub(first.Timestamp)).To(Equal(d),
			"the second Sample must land exactly d after the first on the injected clock")
	})

	// Both rate derivations divide their counter deltas by the elapsed time
	// between two Samples. Every other rate test derives that elapsed from the
	// samples' own Timestamps, which is circular here: this spec advances the
	// mock clock and serves higher counters, so the elapsed the rates divide
	// by can only be the injected clock's d. A sampler stamping from time.Now()
	// would divide the same deltas by microseconds of wall time.
	It("derives both rates from the injected clock's elapsed, not wall time", func() {
		ctx := context.Background()
		clk := clock.NewMock()
		start := time.Date(2020, time.March, 14, 15, 9, 26, 0, time.UTC)
		clk.Set(start)

		// First and second cpu.stat: usage_usec rises 5000000 -> 12000000.
		// First and second /proc/stat: the busy jiffies (user+nice+sys+irq+
		// softirq) rise 410 -> 820. Every other read answers with an error
		// Read tolerates.
		var statCalls, procStatCalls int

		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				if statCalls > 0 {
					return []byte("usage_usec 12000000\nuser_usec 9000000\nsystem_usec 3000000\nnr_periods 20\nnr_throttled 4\n"), nil
				}

				statCalls++

				return []byte("usage_usec 5000000\nuser_usec 4000000\nsystem_usec 1000000\nnr_periods 10\nnr_throttled 2\n"), nil
			case "/proc/stat":
				if procStatCalls > 0 {
					return []byte("cpu  200 0 600 5000 0 0 20 0 0 0\ncpu0 100 0 300 2500 0 0 10 0 0 0\ncpu1 100 0 300 2500 0 0 10 0 0 0\n"), nil
				}

				procStatCalls++

				return []byte("cpu  100 0 300 5000 0 0 10 0 0 0\ncpu0 50 0 150 2500 0 0 5 0 0 0\ncpu1 50 0 150 2500 0 0 5 0 0 0\n"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}

		s := cpuhealth.NewLinuxSamplerWithClock(fs, base, clk)

		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, ok := first.UsageCores.Get()
		Expect(ok).To(BeFalse(), "the baseline read must publish no usage rate")

		const d = 7 * time.Second
		clk.Add(d)

		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())

		// UsageCores: the usage_usec delta 7000000 over 1e6 over the injected
		// clock's 7s is exactly one core.
		usage, ok := second.UsageCores.Get()
		Expect(ok).To(BeTrue(), "a rising counter over the injected clock's span must publish a usage rate")
		Expect(usage).To(Equal(7000000.0/1e6/d.Seconds()),
			"UsageCores must divide the counter delta by the injected clock's elapsed, not wall time")

		// HostBusy: the busy-jiffy delta 410 over USER_HZ (100) over the same
		// 7s.
		busy, ok := second.HostBusy.Get()
		Expect(ok).To(BeTrue(), "a read after the baseline must publish host-busy cores")
		Expect(busy).To(Equal((820.0-410.0)/100.0/d.Seconds()),
			"HostBusy must divide the busy-jiffy delta by the injected clock's elapsed, not wall time")
	})
})
