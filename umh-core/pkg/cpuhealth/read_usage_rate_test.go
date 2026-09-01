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

// Usage as a rate. The sampler derives UsageCores as an instantaneous
// rate from the change in cpu.stat's cumulative usage_usec across two
// consecutive reads: the microsecond delta divided by 1e6 (the microsecond
// divisor), divided by the elapsed time between the reads.
// It publishes NO rate on the first read (no previous edge to subtract from) and
// NO rate when usage_usec falls (a cumulative counter that falls has been
// reset), and it keeps the raw cumulative UsageUsec on the snapshot beside the
// rate so a later throttle-ratio reduction still has the totals.
//
// This is an integration test: the real sampler is driven over a fake
// filesystem serving changing cpu.stat bytes, so the statefulness across Read
// calls — the previous usage_usec edge and its timestamp — is exercised, not
// mocked. Elapsed time is taken from the returned snapshots' own Timestamps
// (every field off the same read carries the same Timestamp), which makes
// the delta/elapsed arithmetic exact whether the sampler reads the real clock
// or an injected one.
package cpuhealth_test

import (
	"context"
	"errors"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("usage as a rate", func() {
	const base = "/sys/fs/cgroup"

	// newSampler serves cpu.stat from a per-call list so each Read sees a
	// different usage_usec, like a live counter moving between reads.
	newSampler := func(usages []uint64) cpuhealth.Sampler {
		i := 0
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				if i >= len(usages) {
					i = len(usages) - 1
				}
				u := usages[i]
				i++
				return []byte(
					"usage_usec " + strconv.FormatUint(u, 10) + "\n" +
						"user_usec 0\nsystem_usec 0\nnr_periods 1000\nnr_throttled 5\n",
				), nil
			case base + "/cpu.max":
				return []byte("200000 100000\n"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		return cpuhealth.NewLinuxSampler(fs, base)
	}

	It("publishes UsageCores as the usage_usec delta over elapsed time, Unknown on the first read and on a reset, and keeps the raw UsageUsec", func() {
		ctx := context.Background()

		// Usage rises 5s -> 9s of CPU over the two reads. Neither the first read
		// nor a later reset has a rate; the counter stays on the snapshot.
		sampler := newSampler([]uint64{5_000_000, 9_000_000, 4_000_000})

		// Read 1: the first read after start has no previous edge to subtract
		// from, so UsageCores must be Unknown — a Known(0) would be a confident
		// zero from no measurement. The raw counter is kept on the snapshot.
		s1, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		u1, ok := s1.UsageUsec.Get()
		Expect(ok).To(BeTrue(), "the raw usage_usec must be a present reading on the first read")
		Expect(u1).To(Equal(5000000.0))
		_, ok = s1.UsageCores.Get()
		Expect(ok).To(BeFalse(),
			"the first read after start must publish no usage rate (Unknown), never a measured zero")

		// Read 2: usage_usec has risen to 9s over a known elapsed time. The rate
		// in cores is the microsecond delta divided by 1e6 (the microsecond
		// divisor) divided by the elapsed seconds. Elapsed is read from the
		// snapshots' own Timestamps.
		s2, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		u2, ok := s2.UsageUsec.Get()
		Expect(ok).To(BeTrue(), "the raw usage_usec must be a present reading every read")
		Expect(u2).To(Equal(9000000.0))

		elapsed := s2.Timestamp.Sub(s1.Timestamp).Seconds()
		Expect(elapsed).To(BeNumerically(">", 0), "two reads must span a positive elapsed time")
		expectedRate := float64(9_000_000-5_000_000) / 1e6 / elapsed
		rate, ok := s2.UsageCores.Get()
		Expect(ok).To(BeTrue(), "a rising counter over a positive elapsed time must publish a usage rate")
		Expect(rate).To(BeNumerically("~", expectedRate, expectedRate*1e-9),
			"UsageCores must be the usage_usec delta divided by 1e6 divided by elapsed seconds")

		// Read 3: usage_usec FALLS (4s < 9s). A cumulative counter that falls
		// has been reset, so no rate — the delta across a reset is arithmetic on
		// two origins. The raw counter is still kept.
		s3, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		u3, ok := s3.UsageUsec.Get()
		Expect(ok).To(BeTrue(), "the raw usage_usec must be present after a reset too")
		Expect(u3).To(Equal(4000000.0))
		_, ok = s3.UsageCores.Get()
		Expect(ok).To(BeFalse(), "a falling usage_usec (counter reset) must publish no usage rate")
	})
})
