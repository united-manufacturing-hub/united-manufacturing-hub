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
	"io"
	"io/fs"
	"os"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// recorded is one Sentry-bound call, as the call site made it.
type recorded struct {
	Msg    string
	Fields map[string]any
}

// recordingLogger wraps a hook-wrapped FSMLogger, so the SAME emission is
// visible on two channels: this recording (which can count and can read
// fields) and the hook's debouncer (which proves interception).
//
// Two channels are needed because neither is sufficient. ShouldCapture answers
// only "was this ONE fingerprint seen", and it RECORDS on every call that
// returns true, so it can be asked once per fingerprint and can never yield a
// count. And the raw fields land on the Sentry event, built after the point the
// debouncer is consulted, so no debouncer query can see them.
type recordingLogger struct {
	deps.FSMLogger

	events *[]recorded
}

// With MUST re-wrap. NewBaseDependencies stores logger.With(String("worker",
// ...)), so a wrapper that inherits With from its embedded logger is thrown
// away at construction and records nothing. That failure is silent and total:
// every recording assertion passes on an empty slice, including the
// zero-events-on-a-healthy-container one, which would then hold with no
// implementation at all.
func (l recordingLogger) With(fields ...deps.Field) deps.FSMLogger {
	return recordingLogger{FSMLogger: l.FSMLogger.With(fields...), events: l.events}
}

func (l recordingLogger) SentryWarn(f deps.Feature, hierarchyPath, msg string, fields ...deps.Field) {
	kv := map[string]any{}
	for _, fld := range fields {
		kv[fld.Key] = fld.Value
	}

	*l.events = append(*l.events, recorded{Msg: msg, Fields: kv})
	l.FSMLogger.SentryWarn(f, hierarchyPath, msg, fields...)
}

const evidenceControllers = "cpuset cpu io memory hugetlb pids rdma\n"

// healthyContainer is the content a working container serves, measured live on
// 2026-09-03. The healthy control is a machine we have seen, not one invented.
func healthyContainer() map[string][]byte {
	return map[string][]byte{
		cgroupBase + "/cpu.stat":              []byte("usage_usec 11457863754\nnr_periods 338962\nnr_throttled 903\n"),
		cgroupBase + "/cpu.max":               []byte("200000 100000\n"),
		cgroupBase + "/cpu.pressure":          []byte("some avg10=0.00 avg60=0.00 avg300=0.00 total=1\n"),
		cgroupBase + "/cpuset.cpus.effective": []byte("0-7\n"),
		cgroupBase + "/cgroup.controllers":    []byte(evidenceControllers),
		"/proc/self/cgroup":                   []byte("0::/\n"),
		"/proc/stat":                          []byte("cpu  1 0 1 1 1 0 1 0 0 0\ncpu0 1 0 1 1 1 0 1 0 0 0\n"),
		"/proc/cpuinfo":                       []byte("flags\t\t: fpu hypervisor\n"),
	}
}

// reportFS serves the fixture. The embedded Service is left nil deliberately,
// matching stubFilesystem in this package: a sampler that grows a third kind of
// call panics here rather than passing quietly on a method this fixture never
// meant to answer.
type reportFS struct {
	filesystem.Service

	files     map[string][]byte
	overrides map[string]error
	reads     *int
}

func (f reportFS) ReadFile(_ context.Context, p string) ([]byte, error) {
	*f.reads++
	if err, ok := f.overrides[p]; ok {
		return nil, err
	}
	if c, ok := f.files[p]; ok {
		return c, nil
	}

	return nil, &fs.PathError{Op: "open", Path: p, Err: syscall.ENOENT}
}

func (f reportFS) ReadDir(_ context.Context, _ string) ([]os.DirEntry, error) {
	*f.reads++

	return nil, nil
}

var _ = Describe("a failed cgroup read is reported to Sentry", func() {
	// build wires the real construction path: a published failing filesystem, a
	// hook-wrapped logger, and NewDeps. Nothing here pre-sets a "read failed"
	// state — the condition arrives the way production produces it, through a
	// filesystem that refuses.
	build := func(overrides map[string]error) (*[]recorded, *fsmv2sentry.SentryHook, *int) {
		events := &[]recorded{}
		reads := 0

		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(io.Discard), zapcore.DebugLevel)
		hook := fsmv2sentry.NewSentryHook(5 * 60 * 1e9)
		// Mirror cmd/main.go: the hook wraps the core, NewFSMLogger sits outside.
		hooked := deps.NewFSMLogger(zap.New(core).WithOptions(zap.WrapCore(hook.Wrap)).Sugar())

		var svc filesystem.Service = reportFS{files: healthyContainer(), overrides: overrides, reads: &reads}
		register.SetDeps[filesystem.Service](FilesystemDepsKey, svc)
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)

		id := deps.Identity{ID: "cpu-report", WorkerType: WorkerType}
		bd := deps.NewBaseDependencies(recordingLogger{FSMLogger: hooked, events: events}, nil, id)

		_ = NewDeps(id, bd)

		// Without this the whole suite is host-dependent: NewDeps silently falls
		// back to the real filesystem when nothing was published, and cpu.go's
		// own comment warns about exactly that.
		Expect(reads).To(BeNumerically(">", 0), "the published fixture was never consulted")

		return events, hook, &reads
	}

	msgs := func(events *[]recorded) []string {
		out := []string{}
		for _, e := range *events {
			out = append(out, e.Msg)
		}

		return out
	}

	It("reports nothing at all from a healthy container", func() {
		events, _, _ := build(nil)

		Expect(msgs(events)).To(BeEmpty(),
			"the standing expectation is zero events attributable to this feature on a healthy instance")
	})

	It("reports exactly one event for one failed read, naming the file and the cause", func() {
		cpuset := cgroupBase + "/cpuset.cpus.effective"
		events, _, _ := build(map[string]error{
			cpuset: &fs.PathError{Op: "open", Path: cpuset, Err: syscall.ENOENT},
		})

		Expect(msgs(events)).To(ConsistOf("cpu::read_failed::cpuset_cpus_effective::enoent"),
			"one failed read is one event; ConsistOf also fails if a sibling read reported")
	})

	It("carries the surrounding evidence on the event, unparsed", func() {
		cpuset := cgroupBase + "/cpuset.cpus.effective"
		events, _, _ := build(map[string]error{
			cpuset: &fs.PathError{Op: "open", Path: cpuset, Err: syscall.ENOENT},
		})

		Expect(*events).To(HaveLen(1))
		f := (*events)[0].Fields

		Expect(f).To(HaveKeyWithValue("cgroup_controllers_raw", evidenceControllers),
			"the controller list is the discriminator and must arrive verbatim")
		Expect(f).To(HaveKeyWithValue("path", cpuset),
			"the failing path belongs in a field, never in the message")
		Expect(f).To(HaveKey("cpu_stat_read"), "the sibling read outcomes are the pattern that diagnoses this")
	})

	It("is intercepted by the Sentry hook, not merely emitted", func() {
		// An observer-core assertion would pass on a logger with no hook at
		// all: the entry is always emitted, and the question is whether
		// anything captured it.
		cpuset := cgroupBase + "/cpuset.cpus.effective"
		_, hook, _ := build(map[string]error{
			cpuset: &fs.PathError{Op: "open", Path: cpuset, Err: syscall.ENOENT},
		})

		// errorTypes is EMPTY on purpose, so this fingerprint has three
		// components rather than four. These events carry no error: the type
		// chain would be fully derivable from the outcome already in the
		// message, so it adds no grouping information, and every event would
		// carry the same stack trace from this one call site.
		want := strings.Join(fsmv2sentry.BuildFingerprint(
			zapcore.WarnLevel, string(deps.FeatureSupportCPU),
			"cpu::read_failed::cpuset_cpus_effective::enoent",
			"",
		), "|")

		Expect(hook.Debouncer().ShouldCapture(want)).To(BeFalse(),
			"the hook already recorded this fingerprint, so it intercepted the entry")
	})

	It("says nothing when cpu.pressure is absent, since a kernel without PSI is normal", func() {
		psi := cgroupBase + "/cpu.pressure"
		events, _, _ := build(map[string]error{
			psi: &fs.PathError{Op: "open", Path: psi, Err: syscall.ENOENT},
		})

		Expect(msgs(events)).To(BeEmpty(),
			"cpu.pressure + enoent is on the suppression list: absence is correct, not a failure")
	})

	It("still reports cpu.pressure when it is present but unreadable", func() {
		psi := cgroupBase + "/cpu.pressure"
		events, _, _ := build(map[string]error{
			psi: &fs.PathError{Op: "open", Path: psi, Err: syscall.EACCES},
		})

		Expect(msgs(events)).To(ConsistOf("cpu::read_failed::cpu_pressure::eacces"),
			"only ENOENT is excused for pressure; a present-but-unreadable file is a real failure")
	})

	It("reports one event, not two, when a failure stops a later read from happening", func() {
		// /proc/stat failing means the cpuset read never happens. Reporting that
		// as a cpuset failure would name a file nobody opened, and minting an
		// event for it would turn one root cause into two issues.
		events, _, _ := build(map[string]error{
			"/proc/stat": &fs.PathError{Op: "open", Path: "/proc/stat", Err: syscall.EACCES},
		})

		Expect(msgs(events)).To(ConsistOf("cpu::read_failed::proc_stat::eacces"))
	})

	It("never puts a path or a raw value in the message", func() {
		// The message is a Sentry grouping component. A path in it would mint a
		// new issue per distinct path, which is the failure the fixed vocabulary
		// exists to prevent.
		cpuset := cgroupBase + "/cpuset.cpus.effective"
		events, _, _ := build(map[string]error{
			cpuset: &fs.PathError{Op: "open", Path: cpuset, Err: syscall.ENOENT},
		})

		// Assert the set is non-empty FIRST. A loop over nothing passes, so
		// without this the spec is green both when every message is clean and
		// when the feature does not exist at all.
		Expect(msgs(events)).NotTo(BeEmpty(), "nothing was emitted, so this spec would pass vacuously")

		for _, m := range msgs(events) {
			Expect(m).NotTo(ContainSubstring("/"), "message %q carries a path", m)
			Expect(m).To(HavePrefix("cpu::"), "message %q is outside the declared vocabulary", m)
		}
	})
})
