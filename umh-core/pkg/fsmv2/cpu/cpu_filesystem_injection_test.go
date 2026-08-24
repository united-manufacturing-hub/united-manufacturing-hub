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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("the filesystem the CPU worker reads", func() {
	// NewDeps takes both of these, and neither carries anything the two specs
	// below distinguish, so they are built the same way for each.
	newBaseDeps := func() (deps.Identity, *deps.BaseDependencies) {
		id := deps.Identity{ID: "cpu-filesystem-injection", WorkerType: WorkerType}

		return id, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, id)
	}

	// publish puts fs in the process-global deps registry and takes it back out
	// when the spec ends. The registry outlives the spec and SetDeps overwrites,
	// so a spec that published without clearing would hand its own filesystem to
	// every later spec in this package.
	publish := func(fs filesystem.Service) {
		register.SetDeps[filesystem.Service](FilesystemDepsKey, fs)
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)
	}

	It("samples through a published filesystem rather than the real one", func() {
		// An unreadable cpu.stat fails the whole sample, and the box words the
		// failure with its own name in it. No real /sys/fs/cgroup can produce
		// that text, so the error is evidence of which filesystem was read —
		// not merely evidence that some read failed.
		box := fakebox.NewBox(cgroupBase, fakebox.Condition{
			Cores:      8,
			QuotaCores: 2,
			UsageCores: 1,
			PsiPresent: true,
			Unreadable: []string{cgroupBase + "/cpu.stat"},
		})
		publish(box.FS())

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).To(HaveOccurred(),
			"the published box refuses cpu.stat, so the sample must fail")
		Expect(err.Error()).To(ContainSubstring("fakebox: "+cgroupBase+"/cpu.stat"),
			"only the published box words a failure this way; the real filesystem never does")
	})

	It("falls back to the real filesystem when nothing was published", func() {
		Expect(register.GetDeps[filesystem.Service](FilesystemDepsKey)).To(BeNil(),
			"precondition: no earlier spec may have left a filesystem in the registry")

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		Expect(d.sampler).NotTo(BeNil(), "an unpublished filesystem still yields a sampler")
		Expect(d.engineErr).NotTo(HaveOccurred(), "the table builds either way")

		// Poll's outcome depends on whether this machine has a cgroup v2 mount,
		// so the spec asserts what holds on both: it returns rather than
		// panicking, and whatever it read, it did not read a box.
		//
		// Read that second assertion for no more than it is worth. It has teeth
		// only where the real read FAILS, as it does on macOS: err is then the
		// OS's own wording, and naming the box would be wrong. On Linux the read
		// succeeds, err is nil, and the assertion holds trivially. Asserting the
		// fallback positively would mean asserting against the real disk, whose
		// contents this spec does not control, so the gap is stated rather than
		// papered over.
		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(fmt.Sprint(err)).NotTo(ContainSubstring("fakebox"),
			"with nothing published the sampler must reach the real filesystem")
	})
})
