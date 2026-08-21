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

// Where the tick's instant comes from. Sample.Timestamp is the only instant the
// diagnosis pipeline ever sees: every rate the sampler derives, and every
// sliding window and latch downstream of it, measures elapsed time as the
// difference between two of these stamps. A caller that supplies a clock
// therefore controls all of them, which is what makes a test over this package
// deterministic instead of dependent on how long the machine took.
//
// The first spec below pins two halves of that: the stamp IS the supplied
// instant, and advancing the supplied clock moves the stamp by exactly the
// amount advanced. Each half admits a sampler the other rejects. The first
// alone would pass a sampler that read the supplied clock once and the wall
// clock thereafter. The second alone would pass a sampler stamping from the
// supplied clock with a constant offset added, because a fixed offset leaves
// the difference between two stamps intact. Together they admit neither, over
// the two reads the spec makes — what a sampler does on a third read is outside
// what these two assertions pin.
//
// The second spec covers the nil clock the constructor documents as meaning the
// system clock. No caller in this repo passes nil, so the branch would otherwise
// be untested; without it the nil dereference in Read is reachable only from
// outside the repo.
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

var _ = Describe("the tick's instant", func() {
	const base = "/sys/fs/cgroup"

	// A readable cgroup: cpu.stat is the primary file, so a Read only succeeds
	// when it parses. The counters never move, because nothing here reads a
	// rate — only the Timestamp the rates would be divided by.
	stat := []byte("usage_usec 5000000\nuser_usec 4000000\nsystem_usec 1000000\nnr_periods 0\nnr_throttled 0\n")

	newSampler := func(c clock.Clock) cpuhealth.Sampler {
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return stat, nil
			case base + "/cpu.max":
				return []byte("200000 100000\n"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}

		return cpuhealth.NewLinuxSamplerWithClock(fs, base, c)
	}

	It("stamps the sample with the supplied clock's instant, and moves the stamp by exactly the amount the clock advances", func() {
		ctx := context.Background()

		// An instant far from any plausible wall clock, so a sampler reading
		// time.Now() cannot coincidentally match it.
		instant := time.Date(2020, time.March, 14, 15, 9, 26, 0, time.UTC)
		mock := clock.NewMock()
		mock.Set(instant)
		sampler := newSampler(mock)

		s1, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(s1.Timestamp).To(Equal(instant),
			"the sample must be stamped with the supplied clock's instant, not the wall clock's")

		// Advancing the supplied clock by a known amount must move the next
		// stamp by that same amount: the elapsed time every rate divides by is
		// the caller's to set, exactly.
		const advance = 10 * time.Second
		mock.Add(advance)

		s2, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(s2.Timestamp.Sub(s1.Timestamp)).To(Equal(advance),
			"advancing the supplied clock by 10s must move the stamp by exactly 10s")
	})

	It("stamps from the system clock when the caller supplies no clock", func() {
		ctx := context.Background()

		// A nil clock is documented as meaning the system clock, so the stamp
		// must land inside a window the wall clock brackets. Reading the wall
		// clock either side of the Read is the only way to bound it: the system
		// clock's instant is not otherwise knowable from here.
		before := time.Now()
		s, err := newSampler(nil).Read(ctx)
		after := time.Now()

		Expect(err).NotTo(HaveOccurred())
		Expect(s.Timestamp).To(BeTemporally(">=", before),
			"a nil clock must stamp from the system clock, not from a zero time")
		Expect(s.Timestamp).To(BeTemporally("<=", after),
			"a nil clock must stamp from the system clock, not from a future time")
	})
})
