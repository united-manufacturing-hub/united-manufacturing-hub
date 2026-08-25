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

// Package fsmv2cpu is the fsmv2 simple monitor that polls a cgroup's CPU health
// with the pkg/cpuhealth library. It owns the sampler, because the sampler
// holds per-tick baselines and the worker owns the tick. Decide stays a pure
// library function in pkg/cpuhealth, so the recording gate keeps driving it
// directly rather than through the worker.
package fsmv2cpu

import (
	"context"
	"strings"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "cpu"

	// InstanceName is the fixed dynamic-child name for the single per-instance
	// CPU monitor, matching how the configworker reconciles a singleton child.
	InstanceName = "cpu"

	// FilesystemDepsKey is the register.SetDeps key under which a caller
	// publishes the filesystem.Service the sampler reads through. It is
	// distinct from WorkerType, which is where a worker's own typed deps
	// payload goes, because the typed deps registry keys on the string alone
	// and two payloads cannot share a key. Same convention, and the same
	// reason, as configworker.ConfigManagerDepsKey.
	FilesystemDepsKey = WorkerType + ".filesystem"

	// ClockDepsKey is the register.SetDeps key under which a caller publishes
	// the clock.Clock the sampler stamps each Sample from. Separate from
	// FilesystemDepsKey for the reason that key is separate from WorkerType:
	// the registry keys on the string alone, so two payloads cannot share one.
	//
	// The two keys belong together. Every rate the sampler publishes is a
	// counter delta divided by the gap between two Sample timestamps, so a
	// caller staging counters on one clock while the sampler stamps from
	// another gets rates neither of them describes. A fakebox.Box advances its
	// counters and its own clock in one Tick for exactly that reason; publish
	// its FS under FilesystemDepsKey and its Clock under this key, and the
	// numbers it was told come back out.
	ClockDepsKey = WorkerType + ".clock"

	// cgroupBase is the cgroup v2 mount point whose CPU controller files the
	// sampler reads (cpu.stat, cpu.max, cpu.pressure, cpuset.cpus.effective).
	// The specs in this package that serve those files from a fixture read this
	// same constant, so the served paths and the looked-for paths cannot drift
	// apart and leave every read failing. Elsewhere the fixture declares its
	// own base (pkg/fsmv2/examples names cpuScenarioBase), which has to be kept
	// equal to this by hand.
	cgroupBase = "/sys/fs/cgroup"

	// pollInterval is how often the worker samples the cgroup.
	//
	// It also fixes the staleness bound on the read side. A reader that goes
	// through pkg/fsmv2/adapter treats an observation as stale once it is older
	// than three times the worker's registered poll interval (staleAfterFor),
	// and simple.Register publishes this constant as that interval. Changing
	// this number therefore moves the sampling cadence and the staleness bound
	// together. Nothing reads the CPU worker through that adapter today, so
	// only the sampling cadence has an effect so far.
	pollInterval = 1 * time.Second

	// admissionWindow is how long a fresh worker may refuse admission while
	// a capable signal has still not first-measured. Once this much sample time
	// has passed since the worker's first sample, admission opens even if the
	// counts are unchanged. The synthetic-clock tests step the sample clock in
	// whole seconds, so the window is a whole number of them.
	admissionWindow = 10 * time.Second
)

// Ref is the (WorkerType, Name) pair identifying the CPU monitor child, shared
// by the configworker that upserts it (gated behind USE_FSMV2_CPU) and the seam
// that reads it back.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

// CPUConfig is the worker's config: the CPU child is upserted with an empty
// config, so this is a deliberately empty struct. A config that carried values
// would make the configworker re-upsert and respawn the child on every tick,
// dropping every 60s window.
type CPUConfig struct{}

// CPUStatus is the result of one CPU-health observation. It carries judgements
// and counts, never a raw measurement such as a CPU-utilisation percentage.
type CPUStatus struct {
	// Verdict is the ranked judgement Decide produced this tick, as the
	// cpuhealth.State string ("healthy" or "degraded"). Empty when the tick
	// could not measure.
	Verdict string `json:"verdict"`

	// Message is ComposeMessage's output for this tick's verdict and signals.
	Message string `json:"message"`

	// SignalsCapable is how many CPU signals this box can answer (not
	// NoInstrument).
	SignalsCapable int `json:"signalsCapable"`
	// SignalsMeasured is how many capable signals have produced a first
	// measurement since this worker started.
	SignalsMeasured int `json:"signalsMeasured"`

	// RefusingAdmission reports whether admission is currently refused: a
	// capable signal has not first-measured (measured < capable) within the
	// 10s admission window. Consume the flag; do not re-derive the count
	// comparison, which drops the window bound. It is meaningful only on a
	// successful read — an errored/empty Poll yields this false ('no
	// determination'), which a consumer must not read as 'admission open'.
	RefusingAdmission bool `json:"refusingAdmission"`

	// Polls is how many observations this worker has completed.
	Polls uint64 `json:"polls"`
}

// CPUDeps is the per-instance state Poll reads and mutates.
//
// TDeps must be *CPUDeps, never the value: Poll takes d by value, and copying a
// value CPUDeps would silently lose every non-pointer field's mutation on the
// copy (the engine is a pointer and would survive, but nothing else is). The
// pointer keeps every field shared across ticks.
type CPUDeps struct {
	*deps.BaseDependencies

	// sampler reads the cgroup. The sampler holds the per-tick baselines; it is
	// a *cgroupSampler behind the interface, so it is shared across ticks.
	sampler cpuhealth.Sampler
	// engine owns every (signal, instrument) window and per-signal latch. It is
	// nil when NewEngine failed at construction (engineErr is then set).
	engine *diagnosis.Engine[cpuhealth.Sample]
	// table is the declaration Poll walks: engine.Select needs the Signal values,
	// which are only reachable through the table the engine was built from.
	table diagnosis.Table[cpuhealth.Sample]
	// engineErr records a NewEngine failure from NewDeps. NewDeps cannot fail, so
	// a table that will not build is reported through Poll: the worker stores no
	// verdict and reports it could not measure, instead of calling Decide on a
	// nil engine, which panics at the supervisor (simple has no recover around
	// Poll).
	engineErr error

	// polls counts completed observations. It is the non-pointer mutation the
	// two-tick spec guards.
	polls uint64

	// firstFilled records, per signal name, whether it has ever reduced to a
	// Ready value since this worker started. Set the first tick that signal's
	// Availability is Ready; never cleared while the worker lives. A respawn
	// builds a new worker and a new engine, so it clears then. This bit is what
	// keeps a signal that already measured counting as measured through a later
	// read outage.
	firstFilled map[string]bool

	// startedAt anchors the 10s admission window: the first sample timestamp
	// the worker ever sees. Elapsed sample time is measured from it, so the
	// window is driven by the sample clock, not the wall clock.
	startedAt time.Time

	// admissionReported records whether a capable signal that never first-measured
	// has already been reported at the 10s window deadline. The report fires once
	// per worker, never once per tick.
	admissionReported bool
}

// NewDeps builds CPU's per-instance deps. It constructs a cgroup sampler
// (precedent: pkg/fsm/container/machine.go), takes one startup snapshot through
// it, and builds the table and engine. NewDeps cannot fail — it returns TDeps
// and nothing else — so a startup snapshot whose read fails yields cores=0,
// quota=0, which silently drops the quota-dependent signals (throttling,
// limit-saturation) from the table for this instance's whole lifetime; a later
// read that succeeds does NOT restore them. Only a table that will not build
// (engineErr) makes Poll report it could not measure — a read failure at
// construction yields a healthy first verdict from a permanently thinned
// table, which is why the startup read error is logged.
//
// The filesystem the sampler reads through is whichever one a caller published
// under FilesystemDepsKey before the worker was spawned, and the clock it stamps
// from is whichever one was published under ClockDepsKey. NewDeps runs per
// instance at spawn time, not at init(), so a caller that publishes first
// decides which files this instance sees and which clock times them.
//
// A caller staging counters on a fixture clock has to publish both. Every rate
// the sampler reports is a counter delta over the gap between two Sample
// timestamps, so staging the counters on one clock while the sampler stamps
// from another yields rates that are neither the staged ones nor the machine's.
//
// A caller who stages no counters needs only the filesystem, and the specs
// beside this file that read a Box once do exactly that.
//
// Nothing published means the real filesystem. That differs from the transport
// pull worker, which errors when its deps are missing, and the difference is
// deliberate: production wants the real filesystem and should not have to
// publish one to get it. The price is paid by a caller who meant to publish a
// fixture and forgot. On Linux the real cgroup files read fine, so that caller
// gets a real verdict computed from the real machine — and a loose assertion,
// such as one expecting "healthy" on an idle box, passes on numbers it never
// staged. An assertion that names something only the published filesystem could
// produce does not have that hole.
func NewDeps(id deps.Identity, bd *deps.BaseDependencies) *CPUDeps {
	fs := register.GetDeps[filesystem.Service](FilesystemDepsKey)
	if fs == nil {
		fs = filesystem.NewDefaultService()
	}

	clk := register.GetDeps[clock.Clock](ClockDepsKey)
	if clk == nil {
		clk = clock.New()
	}

	return NewDepsWithSampler(id, bd, cpuhealth.NewLinuxSamplerWithClock(fs, cgroupBase, clk))
}

// NewDepsWithSampler builds CPU's per-instance deps around an explicit sampler.
// Production NewDeps uses a real cgroup sampler; the dev scenario and tests pass
// a sampler backed by a mock filesystem, which is the only way a Poll can be
// driven without touching a real /sys. The table and engine are built once from
// a startup snapshot, exactly as NewDeps does — the two constructors share this
// path so a mock-backed deps behaves identically to a real one.
func NewDepsWithSampler(id deps.Identity, bd *deps.BaseDependencies, sampler cpuhealth.Sampler) *CPUDeps {
	d := &CPUDeps{
		BaseDependencies: bd,
		sampler:          sampler,
	}

	// The table and engine are built once, at construction, from the startup
	// snapshot. Both cores and quota are startup facts; a quota change at
	// runtime needs a rebuilt table, which this worker does not do. A table that
	// will not build leaves the engine nil and sets engineErr, which Poll
	// reports as could-not-measure rather than letting Decide panic on a nil
	// engine (simple has no recover around Poll; recovery is at the collector).
	// The table is held so Poll can walk table.Signals for per-signal Availability.
	cores, quota := startupCapacity(context.Background(), sampler, bd)
	d.table = cpuhealth.Table(cores, quota)
	d.engine, d.engineErr = diagnosis.NewEngine(d.table)
	d.firstFilled = make(map[string]bool)

	return d
}

// Poll samples the cgroup once and reports the verdict Decide judged. On any
// failure — a NewEngine construction error, or a non-nil Read error — the
// worker stores no verdict and reports it could not measure, never a healthy
// zero. On a nil error with one field absent (e.g. Pressure) it reports the
// verdict Decide produced, because a signal that cannot be read is the
// readability path working rather than a failure.
func Poll(ctx context.Context, d *CPUDeps, _ CPUConfig) (CPUStatus, error) {
	if d.engineErr != nil {
		return CPUStatus{}, d.engineErr
	}

	sample, err := d.sampler.Read(ctx)
	if err != nil {
		return CPUStatus{}, err
	}

	// Anchor the 10s admission window on the first sample timestamp the worker
	// ever sees. The refusal is bounded by sample time, not wall time.
	if d.startedAt.IsZero() {
		d.startedAt = sample.Timestamp
	}

	env := cpuhealth.DeriveEnvironment(sample)
	verdict, signals := cpuhealth.Decide(d.engine, sample, env)

	// The absence-of-evidence counts, from the same walk Decide used: the
	// SAME env, the same tick, after Decide returns. engine.Select returns each
	// signal's Availability; capable means not NoInstrument (something on this
	// box can answer it), measured means its first-fill bit is set. The bit is
	// set the first tick that signal is Ready and never cleared, so a signal
	// that has measured keeps counting as measured through a later read outage.
	capable, measured := 0, 0
	unmeasured := []string{}
	for _, s := range d.table.Signals {
		_, _, _, availability := d.engine.Select(s, env)
		if availability == diagnosis.NoInstrument {
			continue // no instrument on this box: not capable, cannot refuse
		}
		if availability == diagnosis.Ready {
			d.firstFilled[s.Name] = true
		}
		if d.firstFilled[s.Name] {
			measured++
		} else {
			unmeasured = append(unmeasured, s.Name)
		}
		capable++
	}

	d.polls++

	// The refusal is bounded: it holds only while a capable signal has not
	// first-measured AND the admission window has not elapsed. Past the window,
	// admission opens even if counts are unchanged. Elapsed is the sample-time
	// delta from the anchor; production sample timestamps come from monotonic
	// time.Now(), so it is never negative.
	elapsed := sample.Timestamp.Sub(d.startedAt)
	overDeadline := elapsed >= admissionWindow
	refusing := measured < capable && elapsed < admissionWindow

	// Once the 10s window has elapsed and a capable signal has still never
	// first-measured, admission opens even though a source that should answer
	// has stayed silent. Raise exactly one SentryWarn naming every signal that
	// never measured — never once per tick (the admissionReported latch), and
	// never on a box no instrument can answer (capable==0 keeps it silent). A
	// WARN, not an error: there is nothing for an operator to act on — the box
	// simply cannot fully see its own CPU, so paging on-call would be noise.
	//
	// The message is a FIXED event name, never interpolated: sentry's
	// BuildFingerprint groups on the log entry's message verbatim, so a
	// Sprintf carrying signal names and counts would give every distinct
	// combination its own Sentry issue. Every dynamic value rides in the
	// structured fields, which is also how every other worker reports
	// (transport's "persistent_auth_failure", pull's "pending_buffer_overflow").
	if overDeadline && measured < capable && !d.admissionReported {
		d.admissionReported = true
		d.GetLogger().SentryWarn(deps.FeatureSupportCPU, d.GetHierarchyPath(),
			"cpu_admission_deadline_never_measured_signal",
			deps.String("never_measured_signals", strings.Join(unmeasured, ", ")),
			deps.Int("signals_measured", measured),
			deps.Int("signals_capable", capable),
			deps.Duration("admission_window", admissionWindow))
	}

	return CPUStatus{
		Verdict:           string(verdict.State),
		Message:           cpuhealth.ComposeMessage(verdict, signals),
		SignalsCapable:    capable,
		SignalsMeasured:   measured,
		RefusingAdmission: refusing,
		Polls:             d.polls,
	}, nil
}

// startupCapacity derives the two startup facts NewEngine consumes — the number
// of cores the cgroup may use and its positive quota — from a startup snapshot.
// On failure both are zero: no cores, no quota, which is the no-limit table.
func startupCapacity(ctx context.Context, s cpuhealth.Sampler, bd *deps.BaseDependencies) (cores, quota float64) {
	smp, err := s.Read(ctx)
	if err != nil {
		// A startup read failure pins cores=0/quota=0 for the instance's whole
		// lifetime (the quota signals drop from the table and are never
		// restored). This is silent otherwise — the first Poll would report
		// healthy from the thinned table — so log it. (See NewDeps.)
		bd.GetLogger().SentryWarn(deps.FeatureSupportCPU, bd.GetHierarchyPath(),
			"cpu: startup cgroup snapshot failed; quota signals omitted", deps.Err(err))
	}

	if lc, ok := smp.LogicalCpus.Get(); ok {
		cores = lc
	}
	if q, ok := smp.Quota.Get(); ok && q > 0 {
		quota = q
	}

	return cores, quota
}

func init() {
	simple.Register(simple.MonitorSpec[CPUConfig, CPUStatus, *CPUDeps]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		NewDeps:    NewDeps,
		Poll:       Poll,
	})
}
