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

// S2 R3 (D5): the sampler reads both throttle counters (nr_periods,
// nr_throttled) out of the SAME cpu.stat bytes it reads for usage — never a
// second cpu.stat read — and marks either counter unavailable when it is absent
// from cpu.stat or fails to parse, never a trusted 0. cpu.stat is primary: a
// READ failure there fails the whole sample, the first time Read's error is
// live. Drives the real sampler over a fake filesystem so every branch is
// reachable; the mock counts cpu.stat reads so the single-read contract is
// asserted too.
package cpuhealth_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("throttle counters", func() {
	const base = "/sys/fs/cgroup"

	// newSampler returns a sampler whose cpu.stat is served by data/err, and a
	// pointer counting how many times cpu.stat was actually read, so "read
	// exactly once per snapshot" is asserted by the mock rather than inferred.
	// cpu.pressure and cpu.max are best-effort; a quota is supplied so the
	// session is on the limited arm where throttling is meaningful.
	newSampler := func(stat []byte, statErr error) (cpuhealth.Sampler, *int) {
		reads := 0
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				reads++
				return stat, statErr
			case base + "/cpu.max":
				return []byte("200000 100000\n"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		return cpuhealth.NewCgroupSampler(fs, base), &reads
	}

	It("reads both throttle counters from the same cpu.stat, marks either unavailable when absent or unparsable, and fails the whole sample when cpu.stat cannot be read", func() {
		ctx := context.Background()

		// Both counters present. The throttle facts and the usage total all come
		// off ONE cpu.stat read: consecutive asserts in this same snapshot, and
		// the mock counting exactly one read.
		sampler, reads := newSampler([]byte("usage_usec 5000000\nnr_periods 1000\nnr_throttled 50\n"), nil)
		s, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(*reads).To(Equal(1), "cpu.stat must be read exactly once per snapshot")

		periods, ok := s.NrPeriods.Get()
		Expect(ok).To(BeTrue(), "a present nr_periods must be a readable counter")
		Expect(periods).To(Equal(1000.0))
		throttled, ok := s.NrThrottled.Get()
		Expect(ok).To(BeTrue(), "a present nr_throttled must be a readable counter")
		Expect(throttled).To(Equal(50.0))

		// D5: an ABSENT counter is unavailable — never a trusted 0. A no-quota
		// container's cpu.stat has no nr_periods line at all, so it must not
		// ship as "0 periods, trusted". The present counter still reads.
		sampler, _ = newSampler([]byte("usage_usec 1000\nnr_throttled 0\n"), nil)
		s, err = sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, ok = s.NrPeriods.Get()
		Expect(ok).To(BeFalse(), "an absent nr_periods must be unavailable, not a trusted 0")
		throttled, ok = s.NrThrottled.Get()
		Expect(ok).To(BeTrue(), "the present counter must still read when its pair is absent")
		Expect(throttled).To(Equal(0.0))

		// A counter that FAILS TO PARSE now fails the WHOLE sample, per S2 R5:
		// an unparseable cpu.stat is a hard failure, not an absent no-signal. The
		// ABSENT-key case above stays unavailable (a no-quota container has no
		// nr_periods line), but a present value that cannot be read as a number is
		// a corrupt cpu.stat.
		sampler, _ = newSampler([]byte("usage_usec 1000\nnr_periods not-a-number\nnr_throttled 3\n"), nil)
		_, err = sampler.Read(ctx)
		Expect(err).To(HaveOccurred(), "an unparsable cpu.stat value must fail the whole sample")

		// cpu.stat is PRIMARY: a read failure there fails the WHOLE sample — the
		// first time Read's error is live — rather than silently dropping the
		// counters as if they were an absent no-signal.
		sampler, _ = newSampler(nil, errors.New("permission denied"))
		_, err = sampler.Read(ctx)
		Expect(err).To(HaveOccurred(), "a cpu.stat read failure must fail the whole sample")
	})
})
