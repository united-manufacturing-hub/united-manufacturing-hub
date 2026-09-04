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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// errRefusedByStub identifies WHICH filesystem the sampler read: no real
// filesystem words a failure this way.
var errRefusedByStub = errors.New("stub filesystem: every read refused")

// stubFilesystem refuses every read. The embedded Service is left nil so that a
// sampler which grew a second kind of call would panic here rather than pass
// quietly on a method this stub never meant to answer.
type stubFilesystem struct {
	filesystem.Service
}

func (stubFilesystem) ReadFile(context.Context, string) ([]byte, error) {
	return nil, errRefusedByStub
}

// ReadDir was added when the sampler grew a directory listing. The nil embed
// above is a tripwire for exactly that, and it fired: before this method the
// new call panicked here rather than passing quietly, which is what the comment
// on stubFilesystem promises. Refusing the listing keeps the stub's contract —
// every access fails, and it fails in a way no real filesystem words.
func (stubFilesystem) ReadDir(context.Context, string) ([]os.DirEntry, error) {
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

		// errors.Is rather than NotTo(MatchError), because MatchError rejects a
		// nil actual even under NotTo: Poll returns nil on a host with a cgroup
		// v2 mount and an error on one without, and this must hold for both.
		// The assertion has teeth only against a fallback that kept serving a
		// previously published filesystem.
		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(errors.Is(err, errRefusedByStub)).To(BeFalse(),
			"with nothing published the sampler must reach the real filesystem")
	})
})
