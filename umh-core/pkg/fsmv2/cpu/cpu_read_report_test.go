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

// recordingLogger wraps a hook-wrapped FSMLogger so one emission is visible on
// two channels: this recording, which counts and reads fields, and the hook's
// debouncer, which proves interception. Neither alone suffices. ShouldCapture
// answers only "was this ONE fingerprint seen" and RECORDS on every true call,
// so it can be asked once per fingerprint and never yields a count. The raw
// fields land on the Sentry event, built after the debouncer is consulted.
type recordingLogger struct {
	deps.FSMLogger

	events *[]recorded
}

// With MUST re-wrap. NewBaseDependencies stores logger.With(String("worker",
// ...)), so a wrapper inheriting With from its embedded logger is thrown away at
// construction and records nothing. Silently and totally: every recording
// assertion passes on an empty slice, the zero-events-on-a-healthy-container one
// included, which would then hold with no implementation at all.
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

// healthyContainer is what a working container serves, measured live on
// 2026-09-03, so the healthy control is a machine we have seen.
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

// reportFS serves the fixture. The embedded Service is nil deliberately, as in
// stubFilesystem: a sampler growing a third kind of call panics here rather than
// passing quietly on a method this fixture never meant to answer.
type reportFS struct {
	filesystem.Service

	files     map[string][]byte
	overrides map[string]error
	// Consulted before overrides, so a spec can start failing a read partway
	// through a run rather than only from the first.
	errFn func(path string) error
	reads *int
}

func (f reportFS) ReadFile(ctx context.Context, p string) ([]byte, error) {
	*f.reads++

	// Honour the context, as filesystem.DefaultService does: it checks before
	// reading, so on shutdown every in-flight read fails. Ignoring it would make
	// a shutdown indistinguishable from a healthy container, and the shutdown
	// spec would assert nothing.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if f.errFn != nil {
		if err := f.errFn(p); err != nil {
			return nil, err
		}
	}

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

// buildReport wires the real construction path: a published fixture, a
// hook-wrapped logger, NewDeps. Nothing pre-sets a "read failed" state — the
// condition arrives as production produces it, through a filesystem that refuses.
func buildReport(overrides map[string]error, fileOverrides map[string][]byte, errFn func(string) error) (*[]recorded, *fsmv2sentry.SentryHook, *int, *CPUDeps) {
	events := &[]recorded{}
	reads := 0

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard), zapcore.DebugLevel)
	hook := fsmv2sentry.NewSentryHook(5 * 60 * 1e9)
	// Mirror cmd/main.go: the hook wraps the core, NewFSMLogger sits outside.
	hooked := deps.NewFSMLogger(zap.New(core).WithOptions(zap.WrapCore(hook.Wrap)).Sugar())

	var svc filesystem.Service = reportFS{files: withFiles(fileOverrides), overrides: overrides, errFn: errFn, reads: &reads}
	register.SetDeps[filesystem.Service](FilesystemDepsKey, svc)
	DeferCleanup(register.ClearDeps, FilesystemDepsKey)

	id := deps.Identity{ID: "cpu-report", WorkerType: WorkerType}
	bd := deps.NewBaseDependencies(recordingLogger{FSMLogger: hooked, events: events}, nil, id)

	d := NewDeps(id, bd)

	// Without this the suite is host-dependent: NewDeps silently falls back to
	// the real filesystem when nothing was published.
	Expect(reads).To(BeNumerically(">", 0), "the published fixture was never consulted")

	return events, hook, &reads, d
}

// withFiles replaces named files in the healthy container, for a read that
// succeeds while its CONTENT is the problem.
func withFiles(fileOverrides map[string][]byte) map[string][]byte {
	files := healthyContainer()
	for path, content := range fileOverrides {
		files[path] = content
	}

	return files
}

func build(overrides map[string]error) (*[]recorded, *fsmv2sentry.SentryHook, *int) {
	events, hook, reads, _ := buildReport(overrides, nil, nil)

	return events, hook, reads
}

// buildPollable hands back the deps as well, for specs that drive Poll rather
// than only construction.
func buildPollable(errFn func(string) error) (*[]recorded, *CPUDeps) {
	events, _, _, d := buildReport(nil, nil, errFn)

	return events, d
}

func buildWithFiles(fileOverrides map[string][]byte) *[]recorded {
	events, _, _, _ := buildReport(nil, fileOverrides, nil)

	return events
}

func msgs(events *[]recorded) []string {
	out := []string{}
	for _, e := range *events {
		out = append(out, e.Msg)
	}

	return out
}

var _ = Describe("a failed cgroup read is reported to Sentry", func() {
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
		// An observer-core assertion passes on a logger with no hook at all: the
		// entry is always emitted; the question is whether anything caught it.
		cpuset := cgroupBase + "/cpuset.cpus.effective"
		_, hook, _ := build(map[string]error{
			cpuset: &fs.PathError{Op: "open", Path: cpuset, Err: syscall.ENOENT},
		})

		// errorTypes is EMPTY on purpose: these events carry no error, the type
		// chain is derivable from the outcome already in the message, and every
		// event would carry the same stack trace from this one call site.
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
		// /proc/stat failing means the cpuset read never happens: reporting it
		// would name a file nobody opened and split one root cause into two.
		events, _, _ := build(map[string]error{
			"/proc/stat": &fs.PathError{Op: "open", Path: "/proc/stat", Err: syscall.EACCES},
		})

		Expect(msgs(events)).To(ConsistOf("cpu::read_failed::proc_stat::eacces"))
	})

	It("never puts a path or a raw value in the message", func() {
		// The message is a Sentry grouping component: a path in it mints an issue
		// per path, which the fixed vocabulary exists to prevent.
		cpuset := cgroupBase + "/cpuset.cpus.effective"
		events, _, _ := build(map[string]error{
			cpuset: &fs.PathError{Op: "open", Path: cpuset, Err: syscall.ENOENT},
		})

		// Non-empty FIRST: a loop over nothing passes, so without this the spec
		// is green both when every message is clean and when nothing exists.
		Expect(msgs(events)).NotTo(BeEmpty(), "nothing was emitted, so this spec would pass vacuously")

		for _, m := range msgs(events) {
			Expect(m).NotTo(ContainSubstring("/"), "message %q carries a path", m)
			Expect(m).To(HavePrefix("cpu::"), "message %q is outside the declared vocabulary", m)
		}
	})
})
