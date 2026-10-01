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

package fsmv2cpu

import (
	"context"
	"errors"
	"time"

	"github.com/benbjohnson/clock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("the clock the CPU worker samples on", func() {
	// readableFS fails every read it does not serve, which the sampler tolerates.
	readableFS := func() filesystem.Service {
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case cgroupBase + "/cpu.stat":
				return []byte("usage_usec 5000000\nuser_usec 4000000\nsystem_usec 1000000\nnr_periods 10\nnr_throttled 2\n"), nil
			case cgroupBase + "/cpu.max":
				return []byte("200000 100000"), nil
			case cgroupBase + "/cpu.pressure":
				return []byte("some avg10=1.00 avg60=2.00 avg300=3.00 total=0\n"), nil
			case cgroupBase + "/cpuset.cpus.effective":
				return []byte("0-1"), nil
			case "/proc/stat":
				return []byte("cpu  100 0 300 5000 0 0 10 0 0 0\ncpu0 50 0 150 2500 0 0 5 0 0 0\ncpu1 50 0 150 2500 0 0 5 0 0 0\n"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}

		return fs
	}

	newBaseDeps := func() (deps.Identity, *deps.BaseDependencies) {
		id := deps.Identity{ID: "cpu-clock-injection", WorkerType: WorkerType}

		return id, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, id)
	}

	It("samples on the clock in its dependency map", func() {
		pinned := time.Date(2020, time.March, 14, 15, 9, 26, 0, time.UTC)
		mock := clock.NewMock()
		mock.Set(pinned)

		m := map[string]any{}
		fs := readableFS()
		config.SetDependency(m, FilesystemKey, fs)

		var clk clock.Clock = mock
		config.SetDependency(m, ClockKey, clk)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd, m)

		sample, err := d.sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred(),
			"the served cpu.stat parses, so the read cannot error")

		usage, ok := sample.UsageUsec.Get()
		Expect(ok).To(BeTrue(), "only a served cpu.stat can publish a usage figure")
		Expect(usage).To(Equal(5000000.0),
			"the usage figure comes from the map's filesystem, not a real cgroup")
		Expect(sample.Timestamp).To(Equal(pinned),
			"a sampler still stamping from time.Now() cannot produce 2020-03-14T15:09:26Z")
	})

	It("falls back to the real clock when the map holds none", func() {
		m := map[string]any{}
		fs := readableFS()
		config.SetDependency(m, FilesystemKey, fs)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd, m)

		// A clock pinned outside this window fails. A wrong clock that returns
		// the current wall time cannot be told from the real one.
		before := time.Now()
		sample, err := d.sampler.Read(context.Background())
		after := time.Now()

		Expect(err).NotTo(HaveOccurred(),
			"the served cpu.stat parses, so the read cannot error")

		usage, ok := sample.UsageUsec.Get()
		Expect(ok).To(BeTrue(), "only a served cpu.stat can publish a usage figure")
		Expect(usage).To(Equal(5000000.0),
			"the usage figure comes from the map's filesystem, not a real cgroup")
		Expect(sample.Timestamp).To(BeTemporally(">=", before),
			"the real clock stamps an instant at or after the read started")
		Expect(sample.Timestamp).To(BeTemporally("<=", after),
			"the real clock stamps an instant at or before the read finished")
	})
})
