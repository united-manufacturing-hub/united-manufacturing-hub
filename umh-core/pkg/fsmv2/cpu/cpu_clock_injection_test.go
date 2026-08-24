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
	"time"

	"github.com/benbjohnson/clock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("the clock the CPU worker stamps from", func() {
	// A machine with nothing wrong with it. What this file is about is which
	// clock times the reads, so the condition only has to be readable.
	steadyMachine := fakebox.Condition{
		Cores:      4,
		UsageCores: 1,
		HostBusy:   0.5,
		PsiPresent: true,
	}

	newBaseDeps := func() (deps.Identity, *deps.BaseDependencies) {
		id := deps.Identity{ID: "cpu-clock-injection", WorkerType: WorkerType}

		return id, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, id)
	}

	// publish puts a box's filesystem and clock in the process-global deps
	// registry and takes both back out when the spec ends. The registry
	// outlives the spec and SetDeps overwrites, so a spec that published
	// without clearing would hand its own machine to every later spec here.
	publish := func(box *fakebox.Box) {
		register.SetDeps[filesystem.Service](FilesystemDepsKey, box.FS())
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)

		register.SetDeps[clock.Clock](ClockDepsKey, box.Clock())
		DeferCleanup(register.ClearDeps, ClockDepsKey)
	}

	It("stamps each sample from a published clock rather than the wall clock", func() {
		box := fakebox.NewBox(cgroupBase, steadyMachine)
		publish(box)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		first, err := d.sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// A Box's clock starts at a fixed instant in 2020, years from any
		// plausible wall clock, so this separates the two clocks rather than
		// merely observing that some instant was written.
		Expect(first.Timestamp).To(Equal(box.Clock().Now()))
		Expect(first.Timestamp).To(BeTemporally("<", time.Now().Add(-24*time.Hour)),
			"a wall-clock stamp would be today's date, not the fixture's")

		// One Tick moves the box's counters and its clock together, so the
		// second stamp has to be exactly one tick later. Equality with Now()
		// alone would also hold for a sampler that read the clock once and
		// cached it, which is a different way of being wrong.
		box.Tick(7 * time.Second)

		second, err := d.sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Timestamp).To(Equal(first.Timestamp.Add(7 * time.Second)))
	})

	It("derives the stated rate, because the counters and the stamps share one clock", func() {
		// This is what publishing the clock buys, and the reason the two keys
		// have to be filled together. UsageCores is a rate: the sampler divides
		// the usage-counter delta by the gap between two stamps. Tick moves both
		// by the same amount, so the figure the condition states is the figure
		// that comes back. Stamping from the wall clock instead divides a tick's
		// worth of counter by however long the two Read calls happened to be
		// apart, which on this line is microseconds.
		box := fakebox.NewBox(cgroupBase, steadyMachine)
		publish(box)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		_, err := d.sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())

		box.Tick(time.Second)

		smp, err := d.sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())

		usage, ok := smp.UsageCores.Get()
		Expect(ok).To(BeTrue(), "a second read after a tick has a baseline to rate against")
		Expect(usage).To(BeNumerically("~", steadyMachine.UsageCores, 0.001))
	})

	It("falls back to the wall clock when nothing was published", func() {
		Expect(register.GetDeps[clock.Clock](ClockDepsKey)).To(BeNil(),
			"precondition: no earlier spec may have left a clock in the registry")

		// Only the filesystem is published, so the read succeeds on every host
		// and the stamp is the only thing left to look at.
		box := fakebox.NewBox(cgroupBase, steadyMachine)
		register.SetDeps[filesystem.Service](FilesystemDepsKey, box.FS())
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		smp, err := d.sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(smp.Timestamp).To(BeTemporally("~", time.Now(), time.Minute),
			"with no clock published the sampler must stamp from wall time, not from the box's 2020 epoch")
	})
})
