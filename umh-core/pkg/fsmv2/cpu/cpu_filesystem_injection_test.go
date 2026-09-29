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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// errRefusedByStub identifies WHICH filesystem the sampler read: no real
// filesystem words a failure this way.
var errRefusedByStub = errors.New("stub filesystem: every read refused")

// stubFilesystem refuses every read. The embedded Service is nil so a sampler
// growing a second kind of call panics here rather than passing quietly on a
// method this stub never meant to answer.
type stubFilesystem struct {
	filesystem.Service
}

func (stubFilesystem) ReadFile(context.Context, string) ([]byte, error) {
	return nil, errRefusedByStub
}

func (stubFilesystem) FileExists(context.Context, string) (bool, error) {
	return false, errRefusedByStub
}

// ReadDir refuses the sampler's directory listing, keeping the contract: every
// access fails, in a way no real filesystem words.
func (stubFilesystem) ReadDir(context.Context, string) ([]os.DirEntry, error) {
	return nil, errRefusedByStub
}

// stubStatMarker is a cpu.stat counter value no cgroup writes, so an error
// quoting it names the stub below as the filesystem that was read.
const stubStatMarker = "not-a-number-from-the-stub"

// markedStatFilesystem serves one cpu.stat carrying stubStatMarker and refuses
// every other read.
type markedStatFilesystem struct {
	filesystem.Service
}

func (markedStatFilesystem) ReadFile(_ context.Context, path string) ([]byte, error) {
	if path == cgroupBase+"/cpu.stat" {
		return []byte("usage_usec " + stubStatMarker + "\n"), nil
	}

	return nil, errRefusedByStub
}

func (markedStatFilesystem) ReadDir(context.Context, string) ([]os.DirEntry, error) {
	return nil, errRefusedByStub
}

func (markedStatFilesystem) FileExists(_ context.Context, path string) (bool, error) {
	return path == cgroupBase+"/cpu.stat", nil
}

var _ = Describe("the filesystem the CPU worker reads", func() {
	newBaseDeps := func() (deps.Identity, *deps.BaseDependencies) {
		id := deps.Identity{ID: "cpu-filesystem-injection", WorkerType: WorkerType}

		return id, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, id)
	}

	It("samples through the filesystem in its dependency map over the global one", func() {
		// The global stub refuses every read, so a Poll through it reports a
		// machine it cannot measure as healthy. Only the map's marked cpu.stat
		// can make the sample fail, so the error below names which one was read.
		register.SetGlobalDeps[filesystem.Service](FilesystemDepsKey, stubFilesystem{})
		DeferCleanup(register.ClearGlobalDeps, FilesystemDepsKey)

		m := map[string]any{}
		var marked filesystem.Service = markedStatFilesystem{}
		config.SetDependency(m, FilesystemKey, marked)
		Expect(m).To(HaveKey("cpu.filesystem"),
			"the dependency-map slot must be named cpu.filesystem; rename "+
				"FilesystemKey and this literal is the line that fails")

		id, bd := newBaseDeps()
		_, err := Poll(context.Background(), monitorSpec.NewDeps(id, bd, m), CPUConfig{})
		Expect(err).To(HaveOccurred(),
			"the map's filesystem serves a cpu.stat that cannot parse, so the sample must fail")
		Expect(err.Error()).To(ContainSubstring(stubStatMarker),
			"only the map's filesystem serves this counter value; the global stub refuses every read")
	})

	It("falls back to the published global when the map holds nothing under the key", func() {
		// The map holds nothing under FilesystemKey, and the published global
		// serves the marked cpu.stat, so the error below names the global as
		// the filesystem that was read.
		register.SetGlobalDeps[filesystem.Service](FilesystemDepsKey, markedStatFilesystem{})
		DeferCleanup(register.ClearGlobalDeps, FilesystemDepsKey)

		m := map[string]any{"unrelated.key": struct{}{}}
		id, bd := newBaseDeps()
		_, err := Poll(context.Background(), monitorSpec.NewDeps(id, bd, m), CPUConfig{})
		Expect(err).To(HaveOccurred(),
			"with nothing under the key the published global serves a cpu.stat that cannot parse")
		Expect(err.Error()).To(ContainSubstring(stubStatMarker),
			"only the published global serves this counter value")

		// A value of the wrong type under the literal reads as absent
		// (LookupDependency type-asserts with comma-ok), so the global serves
		// here too.
		wrongTyped := map[string]any{"cpu.filesystem": "not a filesystem"}
		_, err = Poll(context.Background(), monitorSpec.NewDeps(id, bd, wrongTyped), CPUConfig{})
		Expect(err).To(HaveOccurred(),
			"a wrong-typed value under the key reads as absent, so the published global still serves")
		Expect(err.Error()).To(ContainSubstring(stubStatMarker),
			"only the published global serves this counter value")
	})

	It("samples through a published filesystem rather than the real one", func() {
		register.SetGlobalDeps[filesystem.Service](FilesystemDepsKey, markedStatFilesystem{})
		DeferCleanup(register.ClearGlobalDeps, FilesystemDepsKey)

		id, bd := newBaseDeps()
		d := monitorSpec.NewDeps(id, bd, nil)

		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).To(HaveOccurred(),
			"the published stub serves a cpu.stat that cannot parse, so the sample must fail")
		Expect(err.Error()).To(ContainSubstring(stubStatMarker),
			"only the published stub serves this counter value; the real cgroup never does")
	})

	It("reports a filesystem that refuses every read as healthy and unmeasured, not degraded", func() {
		// A failed poll degrades the instance and blocks every bridge on it. A
		// host that keeps its CPU accounting outside this cgroup has none of
		// these files, so the poll succeeds carrying no capacity instead.
		register.SetGlobalDeps[filesystem.Service](FilesystemDepsKey, stubFilesystem{})
		DeferCleanup(register.ClearGlobalDeps, FilesystemDepsKey)

		id, bd := newBaseDeps()
		status, err := Poll(context.Background(), monitorSpec.NewDeps(id, bd, nil), CPUConfig{})
		Expect(err).NotTo(HaveOccurred(), "an unreadable cgroup must not degrade the instance")
		Expect(status.Verdict.State).To(Equal(cpuhealth.StateHealthy))
	})

	It("falls back to the real filesystem when nothing was published", func() {
		Expect(register.GlobalDeps[filesystem.Service](FilesystemDepsKey)).To(BeNil(),
			"precondition: no earlier spec may have left a filesystem in the registry")

		id, bd := newBaseDeps()
		d := monitorSpec.NewDeps(id, bd, nil)

		Expect(d.sampler).NotTo(BeNil(), "an unpublished filesystem still yields a sampler")
		Expect(d.engineErr).NotTo(HaveOccurred(), "the table builds either way")

		// Only the concrete type tells the real filesystem from a
		// refuse-everything stub: on a host without cgroup v2 both fail every
		// read.
		_, isDefault := d.fs.(*filesystem.DefaultService)
		Expect(isDefault).To(BeTrue(),
			"with nothing published the resolved filesystem must be the real one, not a stub")

		// errors.Is rather than NotTo(MatchError): MatchError rejects a nil actual
		// even under NotTo, and Poll returns nil with a cgroup v2 mount and an
		// error without one, so this must hold for both. It has teeth only
		// against a fallback still serving a previously published filesystem.
		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(errors.Is(err, errRefusedByStub)).To(BeFalse(),
			"with nothing published the sampler must reach the real filesystem")
	})
})
