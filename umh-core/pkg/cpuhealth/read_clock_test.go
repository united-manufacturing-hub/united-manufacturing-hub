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

// The clock seam. NewLinuxSamplerWithClock builds a sampler that stamps every
// Sample from the clock it was handed, so a caller that moves the clock moves
// every stamp with it. A sampler still calling time.Now() stamps wall time,
// which cannot equal an instant pinned to 2020-03-14T15:09:26Z — the first
// assertion below can only pass by reading the injected clock.
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
})
