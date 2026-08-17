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

// Host signals. The sampler reads the host-busy cores and the CPU-steal
// fraction off the FIRST aggregate /proc/stat "cpu " line, then publishes
// neither on the first read (that read only fixes a baseline). Busy cores are an
// instantaneous rate derived from the busy-jiffy delta across two reads divided
// by USER_HZ (=100) into seconds and by the interval's elapsed seconds, with
// busy excluding idle, iowait, steal, guest and guest_nice. The steal fraction
// is the interval's steal-jiffy delta over the interval's total-jiffy delta,
// where the denominator sums fields 0..7 only — guest and guest_nice are folded
// into user and nice by the kernel, so counting them would double-count and
// understate steal on exactly the guest-heavy hosts where it matters. The
// trailing space in "cpu " is what keeps the aggregate line from being confused
// with cpu0/cpu1.
//
// The fixture advances between reads like a live host, and every exclusion is
// load-bearing:
//
//	read1: cpu  100 20 80 250 55 0 0 10 1000 200
//	       cpu  200 40 160 500 50 0 35 30 500 200   (read2)
//	            user nice sys idle iow irq sfr steal guest gst_nice
//
//	read1: busy = user+nice+sys+irq+softirq = 100+20+80+0+0   = 200 jiffies.
//	       denom = fields 0..7 = 100+20+80+250+55+0+0+10      = 515.
//	       steal = 10.
//	read2: busy = 200+40+160+0+35 = 435 jiffies.
//	       denom = 200+40+160+500+50+0+35+30 = 1015.
//	       steal = 30.
//
//	Δbusy  = 435-200 = 235 → HostBusy = 235/USER_HZ/elapsed cores/sec.
//	Δdenom = 1015-515 = 500, Δsteal = 30-10 = 20 → Steal = 20/500 = 0.04.
//
// Guest (read2) deliberately falls 1000→500: if the denominator wrongly summed
// fields 0..9 it would fold guest+guest_nice in, cancelling to Δdenom=0 and
// publishing no Steal at all. The cpu0/cpu1 lines that follow the aggregate are
// the trap: if the parse matched the bare "cpu" prefix instead of "cpu " it
// would take "cpu0" first and read busy=0 on both reads, so Δbusy=0 and neither
// signal survives. An unchanged cumulative counter therefore proves the deltas
// are real, not a re-read of the same snapshot.
package cpuhealth_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("host signals", func() {
	const base = "/sys/fs/cgroup"

	stat := []byte("usage_usec 5000000\nuser_usec 4000000\nsystem_usec 1000000\nnr_periods 0\nnr_throttled 0\n")

	// procStats serves a per-call sequence of /proc/stat snapshots, so each Read
	// sees a later cumulative counter like a live host advancing between reads.
	// The final snapshot's busy/steal FALL (a host restart): it exists to pin the
	// reset guard, not the delta.
	newSampler := func(procStats [][]byte) cpuhealth.Sampler {
		i := 0
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return stat, nil
			case base + "/cpu.max":
				return []byte("max 100000\n"), nil
			case "/proc/stat":
				if i >= len(procStats) {
					i = len(procStats) - 1
				}
				b := procStats[i]
				i++
				return b, nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		return cpuhealth.NewLinuxSampler(fs, base)
	}

	// Each aggregate line keeps the cpu0/cpu1 trap and a guest/guest_nice pair
	// whose values must NOT enter the busy sum or the steal denominator.
	read1 := []byte("cpu  100 20 80 250 55 0 0 10 1000 200\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\nintr 0\n")
	read2 := []byte("cpu  200 40 160 500 50 0 35 30 500 200\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\nintr 0\n")
	reset := []byte("cpu  20 10 40 200 40 0 30 5 1000 200\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\nintr 0\n")

	It("publishes host busy cores and steal as interval deltas from the first 'cpu ' line, nothing on the baseline read, and nothing across a host reset", func() {
		ctx := context.Background()
		// baseline busy=200/steal=10/denom=515; read2 busy=435/steal=30/denom=1015;
		// reset busy=100/steal=5 (falling).
		s := newSampler([][]byte{read1, read2, reset})

		// The FIRST /proc/stat read publishes no host signal: it only fixes a
		// baseline, so HostBusy and Steal are both absent rather than a 0. A 0
		// would read as "host is idle", and the first tick after every restart
		// has no host reading at all.
		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, ok := first.HostBusy.Get()
		Expect(ok).To(BeFalse(), "the baseline read must publish no host-busy reading")
		_, ok = first.Steal.Get()
		Expect(ok).To(BeFalse(), "the baseline read must publish no steal reading")

		// With the baseline fixed, the second read's HostBusy is the interval's
		// busy-jiffy delta over the interval's elapsed seconds, divided by
		// USER_HZ into cores: Δbusy 235 / 100 / elapsed. Elapsed is read from the
		// snapshots' own Timestamps so the arithmetic is exact. The cpu0/cpu1
		// lines survive only because the trailing space in "cpu " skipped them.
		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		elapsed := second.Timestamp.Sub(first.Timestamp).Seconds()
		Expect(elapsed).To(BeNumerically(">", 0), "two reads must span a positive elapsed time")
		busy, ok := second.HostBusy.Get()
		Expect(ok).To(BeTrue(), "a read after the baseline must publish host-busy cores")
		Expect(busy).To(BeNumerically("~", 235.0/100.0/elapsed, 1e-12*235.0/100.0/elapsed),
			"HostBusy must be the busy-jiffy delta over the interval divided by USER_HZ, excluding iowait/steal/guest/guest_nice")

		// Steal is the interval's steal-jiffy delta over the denominator delta.
		// Δsteal 20 over Δdenom 500 = 0.04. The guest drop 1000→500 would zero
		// the denominator (and publish no steal) if guest/guest_nice leaked in.
		steal, ok := second.Steal.Get()
		Expect(ok).To(BeTrue(), "steal is a reading, present per read, not a capability flag")
		Expect(steal).To(BeNumerically("~", 20.0/500.0, 1e-9),
			"steal must be the steal-jiffy delta over fields 0..7 only (guest/guest_nice excluded from the denominator)")

		// Read 3: the busy and steal counters FALL (a host restart). A cumulative
		// counter that falls has been reset, so no reading is published — the
		// delta across a reset is arithmetic on two origins.
		third, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, ok = third.HostBusy.Get()
		Expect(ok).To(BeFalse(), "a falling busy counter (host reset) must publish no host-busy reading")
		_, ok = third.Steal.Get()
		Expect(ok).To(BeFalse(), "a falling steal counter (host reset) must publish no steal reading")
	})
})
