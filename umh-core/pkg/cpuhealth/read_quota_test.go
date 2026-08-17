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

// The sampler distinguishes the three outcomes a cpu.max file can name — a set
// limit, the literal "max" (uncapped), and an unreadable file — as three
// distinct Quota readings, never lets a non-positive limit be used as a
// positive capacity, and treats an unreadable cpu.max as no-signal rather than
// as a definite no-limit.
//
// The sampler is driven over a fake filesystem so unparsable, non-positive and
// unreadable cpu.max files are all reachable. Each outcome's Quota reading is
// asserted independently, so a change to one branch cannot hide a regression
// in another.
package cpuhealth_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("capacity", func() {
	const base = "/sys/fs/cgroup"

	// A valid primary cpu.stat.
	stat := []byte("usage_usec 5000000\nuser_usec 4000000\nsystem_usec 1000000\nnr_periods 0\nnr_throttled 0\n")

	newSampler := func(maxData []byte, maxErr error) cpuhealth.Sampler {
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return stat, nil
			case base + "/cpu.max":
				return maxData, maxErr
			default:
				return nil, errors.New("unreadable")
			}
		}

		return cpuhealth.NewLinuxSampler(fs, base)
	}

	It("distinguishes the three cpu.max outcomes and never reads a non-positive limit as a capacity", func() {
		ctx := context.Background()

		// Outcome 1 of 3: a genuinely limited container names a positive quota.
		// cpu.max "200000 100000" is 2 cores. This must be a present, positive
		// reading — the only outcome that can act as a capacity denominator.
		s, err := newSampler([]byte("200000 100000\n"), nil).Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		quota, ok := s.Quota.Get()
		Expect(ok).To(BeTrue(), "a set limit must be a present reading")
		Expect(quota).To(Equal(2.0))

		// Outcome 2 of 3: the literal "max" means uncapped. It must still be a
		// PRESENT reading (a definite no-limit), staying distinct from an
		// unreadable file, but it must not read as a positive limit, so it can
		// never be a capacity denominator either.
		s, err = newSampler([]byte("max 100000\n"), nil).Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		quota, ok = s.Quota.Get()
		Expect(ok).To(BeTrue(), "uncapped 'max' must be a present, definite reading")
		Expect(quota).To(Equal(0.0))

		// Outcome 3 of 3: an unreadable cpu.max is NO-SIGNAL, not no-limit. A
		// genuinely limited container whose cpu.max cannot be read must not
		// silently move into the definite no-limit judgement that 'max' and 0
		// produce — otherwise its limit, and the throttling that hangs off it,
		// would be judged away. The reading must be ABSENT.
		s, err = newSampler(nil, errors.New("permission denied")).Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, ok = s.Quota.Get()
		Expect(ok).To(BeFalse(), "an unreadable cpu.max must be no-signal (absent), not a definite no-limit")

		// A non-positive limit (0 or negative) is never a valid denominator. It
		// must stay a definite no-limit — a present reading, like "max", distinct
		// from the absence an unreadable file produces — but never surface as a
		// positive quota, the value downstream code would put in a division.
		for _, content := range [][]byte{[]byte("0 100000\n"), []byte("-100 100000\n")} {
			s, err = newSampler(content, nil).Read(ctx)
			Expect(err).NotTo(HaveOccurred())
			q, present := s.Quota.Get()
			Expect(present).To(BeTrue(),
				"cpu.max %q must be a present, definite no-limit, not conflated with no-signal", string(content))
			Expect(q).To(Equal(0.0),
				"cpu.max %q must never be read as a positive limit/denominator", string(content))
		}

		// An unparsable cpu.max (garbage quota, or too few fields) reads as
		// no-signal — the same absent family as an unreadable file, never a
		// definite no-limit.
		for _, content := range [][]byte{[]byte("abc 100000\n"), []byte("onlyonefield\n")} {
			s, err = newSampler(content, nil).Read(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, present := s.Quota.Get()
			Expect(present).To(BeFalse(),
				"unparsable cpu.max %q must be absent no-signal", string(content))
		}
	})
})
