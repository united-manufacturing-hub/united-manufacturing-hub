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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// errRefusedByStub is what the stub filesystem below returns from every read.
// No real filesystem words a failure this way, so finding it in the error a
// Poll returns identifies WHICH filesystem the sampler read, rather than merely
// showing that some read failed.
var errRefusedByStub = errors.New("stub filesystem: every read refused")

// stubFilesystem refuses every read. The sampler reads files and does nothing
// else, so the embedded Service is left nil: it satisfies the interface at
// compile time, and a sampler that ever grew a second kind of call would panic
// here rather than pass quietly on a method this stub never meant to answer.
type stubFilesystem struct {
	filesystem.Service
}

func (stubFilesystem) ReadFile(context.Context, string) ([]byte, error) {
	return nil, errRefusedByStub
}

var _ = Describe("the filesystem the CPU worker reads", func() {
	newBaseDeps := func() (deps.Identity, *deps.BaseDependencies) {
		id := deps.Identity{ID: "cpu-filesystem-injection", WorkerType: WorkerType}

		return id, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, id)
	}

	It("samples through a published filesystem rather than the real one", func() {
		// The registry outlives the spec and SetDeps overwrites, so publishing
		// without clearing would hand this stub to every later spec here.
		register.SetDeps[filesystem.Service](FilesystemDepsKey, stubFilesystem{})
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		// An unreadable cpu.stat fails the whole sample, and the sampler wraps
		// the filesystem's own error into what Poll returns.
		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).To(HaveOccurred(),
			"the published stub refuses cpu.stat, so the sample must fail")
		Expect(err).To(MatchError(errRefusedByStub),
			"only the published stub words a failure this way; the real filesystem never does")
	})

	It("falls back to the real filesystem when nothing was published", func() {
		Expect(register.GetDeps[filesystem.Service](FilesystemDepsKey)).To(BeNil(),
			"precondition: no earlier spec may have left a filesystem in the registry")

		id, bd := newBaseDeps()
		d := NewDeps(id, bd)

		Expect(d.sampler).NotTo(BeNil(), "an unpublished filesystem still yields a sampler")
		Expect(d.engineErr).NotTo(HaveOccurred(), "the table builds either way")

		// Poll errors on a host with no cgroup v2 mount and succeeds on one that
		// has it, so this must hold for a nil error too. errors.Is is used rather
		// than NotTo(MatchError): MatchError rejects a nil actual even under
		// NotTo, which passes on a host without cgroups and fails on one with.
		//
		// Read the assertion for no more than it is worth. It has teeth only
		// against a fallback that kept serving a previously published
		// filesystem. Asserting the fallback positively would mean asserting
		// against the real disk, whose contents this spec does not control.
		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(errors.Is(err, errRefusedByStub)).To(BeFalse(),
			"with nothing published the sampler must reach the real filesystem")
	})
})
