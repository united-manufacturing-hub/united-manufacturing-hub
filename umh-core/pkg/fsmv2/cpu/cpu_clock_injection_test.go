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

// Specs for the clock seam at the worker boundary; ClockDepsKey's doc is the
// contract. The consequence these specs exploit: a mock pinned to
// 2020-03-14T15:09:26Z can only appear in a Sample by reading the published
// clock, since a sampler still stamping from time.Now() cannot produce that
// instant.
package fsmv2cpu

import (
	"context"
	"errors"
	"time"

	"github.com/benbjohnson/clock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("the clock the CPU worker samples on", func() {
	// readableFS serves every cgroup file the sampler reads, from the same
	// cgroupBase the sampler is constructed with, so a read succeeds and the
	// assertions land on the timestamp rather than an error path. Unserved
	// paths — /proc/cpuinfo and the two DMI identity files — answer with an
	// error the sampler tolerates, so the virtualisation fact stays unresolved.
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

	It("samples on a published clock rather than the real one", func() {
		pinned := time.Date(2020, time.March, 14, 15, 9, 26, 0, time.UTC)
		clk := clock.NewMock()
		clk.Set(pinned)

		// The registry is process-global and outlives this spec, so both keys
		// are cleared afterwards: no later spec may inherit this clock or this
		// filesystem.
		register.SetDeps[filesystem.Service](FilesystemDepsKey, readableFS())
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)
		register.SetDeps[clock.Clock](ClockDepsKey, clk)
		DeferCleanup(register.ClearDeps, ClockDepsKey)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		sample, err := d.sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred(),
			"the published filesystem serves every cgroup file, so the read must succeed")
		Expect(sample.Timestamp).To(Equal(pinned),
			"a sampler still stamping from time.Now() cannot produce 2020-03-14T15:09:26Z")
	})

	It("falls back to the real clock when nothing was published", func() {
		Expect(register.GetDeps[clock.Clock](ClockDepsKey)).To(BeNil(),
			"precondition: no earlier spec may have left a clock in the registry")

		register.SetDeps[filesystem.Service](FilesystemDepsKey, readableFS())
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		Expect(d.sampler).NotTo(BeNil(), "an unpublished clock still yields a sampler")
		Expect(d.engineErr).NotTo(HaveOccurred(), "the table builds either way")

		// A wall-clock window rather than a non-zero check: clock.New().Now()
		// is time.Now(), so a real-clock stamp necessarily lands in [before,
		// after], while a clock pinned to any fixed instant — including a
		// fresh clock.NewMock(), which starts at the epoch — falls outside it.
		// A wrong clock that happens to return a current wall instant still
		// passes; that one is indistinguishable from the real clock. A nil
		// clock handed through would have panicked at the Read below.
		before := time.Now()
		sample, err := d.sampler.Read(context.Background())
		after := time.Now()
		Expect(err).NotTo(HaveOccurred(),
			"the published filesystem serves every cgroup file, so the read must succeed")
		Expect(sample.Timestamp).To(BeTemporally(">=", before),
			"the real clock stamps an instant at or after the read started")
		Expect(sample.Timestamp).To(BeTemporally("<=", after),
			"the real clock stamps an instant at or before the read finished")
	})

	It("binds the clock at spawn, so a publish after the spawn cannot re-stamp the sampler", func() {
		Expect(register.GetDeps[clock.Clock](ClockDepsKey)).To(BeNil(),
			"precondition: no earlier spec may have left a clock in the registry")

		register.SetDeps[filesystem.Service](FilesystemDepsKey, readableFS())
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		// Only after the spawn does the clock appear. A sampler that re-read
		// the registry on every Read would now stamp from this mock.
		pinned := time.Date(2020, time.March, 14, 15, 9, 26, 0, time.UTC)
		clk := clock.NewMock()
		clk.Set(pinned)
		register.SetDeps[clock.Clock](ClockDepsKey, clk)
		DeferCleanup(register.ClearDeps, ClockDepsKey)

		before := time.Now()
		sample, err := d.sampler.Read(context.Background())
		after := time.Now()
		Expect(err).NotTo(HaveOccurred(),
			"the published filesystem serves every cgroup file, so the read must succeed")
		Expect(sample.Timestamp).To(BeTemporally(">=", before),
			"the sampler keeps the clock it was built with, so a publish after the spawn cannot re-stamp it")
		Expect(sample.Timestamp).To(BeTemporally("<=", after),
			"the sampler keeps the clock it was built with, so a publish after the spawn cannot re-stamp it")
		Expect(sample.Timestamp).NotTo(Equal(pinned),
			"a pinned instant means the sampler looked the clock up per read instead of at spawn")
	})
})
