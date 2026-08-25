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
// carries the baselines every rate is derived from and the worker owns the tick
// they advance on. It owns no judgement: cpuhealth.Decide is a plain function
// over one sample, and the worker only hands it a sample and reports what came
// back.
package fsmv2cpu

import (
	"context"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
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

	// cgroupBase is the cgroup v2 mount point whose CPU controller files the
	// sampler reads (cpu.stat, cpu.max, cpu.pressure, cpuset.cpus.effective).
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

// Ref is the (WorkerType, Name) pair identifying the CPU monitor child. The
// configworker upserts the child under it, gated behind USE_FSMV2_CPU, and a
// reader fetches that child's status back under the same pair through
// fsmv2client.
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
	// admission window. Consume the flag; do not re-derive the count
	// comparison, which drops the window bound. It is meaningful only on a
	// successful read — an errored/empty Poll yields this false ('no
	// determination'), which a consumer must not read as 'admission open'.
	RefusingAdmission bool `json:"refusingAdmission"`

	// Polls is how many observations this worker has completed.
	Polls uint64 `json:"polls"`
}

// CPUDeps is the per-instance state Poll reads and mutates.
//
// TDeps must be *CPUDeps, never the value: Poll takes d by value, so a value
// CPUDeps would hand each tick its own copy and lose every mutation to a plain
// field. The failure would be partial rather than total, and so quiet — the map
// and the pointers below share their contents across a copy, and only the plain
// fields would reset. The pointer keeps all of them shared.
type CPUDeps struct {
	*deps.BaseDependencies

	// sampler reads the cgroup. Behind the interface it is a pointer, and the
	// counter baselines each rate is derived from live in the sources that
	// pointer owns, so both survive the tick.
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

	// polls counts completed observations. It is the plain-field mutation the
	// two-tick spec in cpu_test.go guards.
	polls uint64

	// firstFilled records, per signal name, whether it has ever reduced to a
	// Ready value since this worker started. Set the first tick that signal's
	// Availability is Ready; never cleared while the worker lives. A respawn
	// builds a new worker and a new engine, so it clears then. This bit is what
	// keeps a signal that already measured counting as measured through a later
	// read outage.
	firstFilled map[string]bool

	// startedAt anchors the admission window: the first sample timestamp
	// the worker ever sees. Elapsed sample time is measured from it, so the
	// window is driven by the sample clock, not the wall clock.
	startedAt time.Time

	// admissionReported records whether a capable signal that never first-measured
	// has already been reported at the admission-window deadline. The report fires
	// once per worker, never once per tick.
	admissionReported bool
}

// NewDeps builds CPU's per-instance deps. It constructs a cgroup sampler
// (precedent: pkg/fsm/container/machine.go), takes one startup snapshot through
// it, and builds the table and engine.
//
// NewDeps cannot fail — it returns TDeps and nothing else — so a startup
// snapshot whose read fails yields cores=0, quota=0. cpuhealth.Table adds its
// host-capacity signal only for a positive core count, and its container-limit
// signal only for a positive quota, so a failed startup read drops both from
// this instance's table for its whole lifetime; a later read that succeeds does
// NOT restore them. Of the two things that can go wrong here — the startup read
// and building the engine — only a failure to build (engineErr) makes Poll
// report it could not measure. A failed startup read yields a healthy first
// verdict from a permanently thinned table instead, which is why it is logged.
//
// The sampler reads the real cgroup files through the real filesystem. A caller
// that needs Poll driven off something else builds its own sampler and passes it
// to NewDepsWithSampler.
func NewDeps(id deps.Identity, bd *deps.BaseDependencies) *CPUDeps {
	return NewDepsWithSampler(id, bd, cpuhealth.NewLinuxSampler(filesystem.NewDefaultService(), cgroupBase))
}

// NewDepsWithSampler builds CPU's per-instance deps around an explicit sampler,
// for a caller holding one already rather than one that wants the cgroup sampler
// NewDeps builds. The table and engine are built from a startup snapshot through
// that sampler, on the same path NewDeps takes, so deps built either way behave
// identically from Poll's side. It is also how a caller keeps Poll off the real
// /sys: the specs beside this file pass a sampler over a fixture filesystem.
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

	// The admission window is anchored on the first sample timestamp the worker
	// ever sees, so the refusal below is bounded by sample time, not wall time.
	if d.startedAt.IsZero() {
		d.startedAt = sample.Timestamp
	}

	env := cpuhealth.DeriveEnvironment(sample)
	verdict, signals := cpuhealth.Decide(d.engine, sample, env)

	// The absence-of-evidence counts: a second walk of the table, with the same
	// env, in the same tick, after Decide returns. It has to run after — Select
	// reports a window's readiness without ageing it, so it is only trustworthy
	// following an Observe on that tick, and Decide is what Observes.
	//
	// Select gives each signal's Availability. Capable means not NoInstrument:
	// something on this box can answer the signal. Measured means the signal's
	// first-fill bit is set, which happens the first tick it reads Ready and is
	// never undone, so a signal that has measured keeps counting as measured
	// through a later read outage.
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

	// Elapsed is the sample-time delta from the anchor, never a wall-clock one.
	// Production sample timestamps come from monotonic time.Now(), so it is
	// never negative there.
	elapsed := sample.Timestamp.Sub(d.startedAt)
	refusing, shortfallAtDeadline := admissionDecision(elapsed, admissionWindow, measured, capable)

	// Say once that admission opened on a source which should answer and never
	// did. Once per worker, not once per tick — that is the admissionReported
	// latch, and it is the reason this gate is not the pure decision above. A
	// WARN, not an error: there is nothing for an operator to act on — the box
	// simply cannot fully see its own CPU, so paging on-call would be noise.
	//
	// The message is a FIXED event name, never interpolated: sentry's
	// BuildFingerprint groups on the log entry's message verbatim, so a
	// Sprintf carrying signal names and counts would give every distinct
	// combination its own Sentry issue. Every dynamic value rides in the
	// structured fields, which is also how every other worker reports
	// (transport's "persistent_auth_failure", pull's "pending_buffer_overflow").
	if shortfallAtDeadline && !d.admissionReported {
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

// admissionDecision answers what the admission window says about one tick. It
// reads nothing but its arguments — no worker, no sampler, no clock.
//
// elapsed is how much sample time has passed since the worker's first sample.
// capable and measured are the tick's evidence counts: how many signals this box
// can answer at all, and how many of those have ever produced a reading. A
// shortfall is measured below capable — some signal this box can answer has
// never once answered.
//
// A shortfall does one of two things, and which one depends only on where the
// tick falls in the window:
//
//	refusing             inside the window: hold admission back and wait
//	shortfallAtDeadline  the window has closed: admit anyway, and report it
//
// So the refusal is bounded rather than fixed to the counts. A signal that never
// measures stops blocking admission once the window closes, which is what keeps
// a box that cannot fully see its own CPU from being blocked for its whole life;
// it is reported instead. The two results are the same shortfall split by the
// window, so they can never both be true, and with no shortfall neither is.
func admissionDecision(elapsed, window time.Duration, measured, capable int) (refusing, shortfallAtDeadline bool) {
	shortfall := measured < capable

	return shortfall && elapsed < window, shortfall && elapsed >= window
}

// startupCapacity derives the two startup facts cpuhealth.Table shapes itself
// from — the number of cores the cgroup may use and its positive quota — out of
// one startup snapshot. On failure both are zero, which is the table without
// either capacity signal.
func startupCapacity(ctx context.Context, s cpuhealth.Sampler, bd *deps.BaseDependencies) (cores, quota float64) {
	smp, err := s.Read(ctx)
	if err != nil {
		// Log it: a failed startup read is otherwise silent, because the first
		// Poll reports healthy from the thinned table rather than an error.
		// NewDeps has why the thinning lasts the instance's whole lifetime.
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
