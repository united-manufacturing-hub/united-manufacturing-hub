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
	// publishes the filesystem.Service the sampler reads the cgroup files
	// through. NewDeps looks it up per instance at spawn time.
	//
	// The key is deliberately not WorkerType, which is where a worker's own
	// typed deps payload goes: the typed deps registry keys on the string
	// alone, so two payloads cannot share one key. Same convention, and the
	// same reason, as configworker.ConfigManagerDepsKey.
	FilesystemDepsKey = WorkerType + ".filesystem"

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
//
// The struct is not itself a wire shape; two of its six fields are. Verdict and
// Message fill the Category and the Message of the models.Health a container
// monitor reports for CPU. What reads that Health lives outside this package:
// the Management Console frontend, and
// ProtocolConverterService.IsResourceLimited, which quotes the message into its
// reason for refusing a new bridge.
type CPUStatus struct {
	// Verdict is the ranked judgement Decide produced this tick, as the
	// cpuhealth.State string ("healthy" or "degraded"). Empty when the tick
	// could not measure. Those two values are the ones that map onto
	// models.Active and models.Degraded, and that health category is what the
	// Management Console colours its CPU reading from.
	Verdict string `json:"verdict"`

	// Message is ComposeMessage's output for this tick's verdict and signals. It
	// is the sentence a customer reads: the Management Console renders a health
	// message verbatim, and IsResourceLimited prefixes it with "CPU degraded: "
	// as the reason a new bridge was refused. Both use it as text, so the
	// wording is the whole contract.
	Message string `json:"message"`

	// SignalsCapable is how many CPU signals this box can answer (not
	// NoInstrument). It has no consumer: nothing outside this package decides,
	// displays or alerts on it.
	SignalsCapable int `json:"signalsCapable"`
	// SignalsMeasured is how many capable signals have produced a first
	// measurement since this worker started. It has no consumer either.
	SignalsMeasured int `json:"signalsMeasured"`

	// RefusingAdmission reports whether admission is currently refused: a
	// capable signal has not first-measured (measured < capable) within the
	// admission window. Nothing consumes it; the contract below binds whatever
	// first does. Consume the flag; do not re-derive the count comparison,
	// which drops the window bound. It is meaningful only on a successful read
	// — an errored/empty Poll yields this false ('no determination'), which a
	// consumer must not read as 'admission open'.
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

	// adm is the admission window's state: its anchor and its report latch.
	// admission.go has what the window is for.
	adm admission
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

	refusing, shortfallAtDeadline := d.adm.decide(sample.Timestamp, measured, capable)

	// Say once that admission opened on a source which should answer and never
	// did. Once per worker, not once per tick — that is reportOnce, and it is
	// the reason this gate is not part of the decision above. A
	// WARN, not an error: there is nothing for an operator to act on — the box
	// simply cannot fully see its own CPU, so paging on-call would be noise.
	//
	// The message is a FIXED event name, never interpolated: sentry's
	// BuildFingerprint groups on the log entry's message verbatim, so a
	// Sprintf carrying signal names and counts would give every distinct
	// combination its own Sentry issue. Every dynamic value rides in the
	// structured fields, which is also how every other worker reports
	// (transport's "persistent_auth_failure", pull's "pending_buffer_overflow").
	if shortfallAtDeadline && d.adm.reportOnce() {
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
// The sampler reads through whichever filesystem.Service a caller published
// under FilesystemDepsKey, and through the real filesystem when nothing was
// published. The lookup happens here, per instance, at spawn time rather than
// at init(), so a caller that publishes before the spawn decides which files
// that instance sees for the rest of its life.
//
// Falling back rather than failing differs from the transport pull worker,
// which errors when its deps are missing, and the difference is deliberate:
// production wants the real filesystem and should not have to publish one to
// get it. The price falls on a caller who meant to publish a fixture and
// forgot. On Linux the real cgroup files read fine, so that caller gets a
// verdict computed from the real machine, and a loose assertion such as one
// expecting "healthy" on an idle box passes on numbers it never staged. An
// assertion that names something only the published filesystem can produce
// does not have that hole.
func NewDeps(id deps.Identity, bd *deps.BaseDependencies) *CPUDeps {
	fs := register.GetDeps[filesystem.Service](FilesystemDepsKey)
	if fs == nil {
		fs = filesystem.NewDefaultService()
	}

	return newDepsWithSampler(id, bd, cpuhealth.NewLinuxSampler(fs, cgroupBase))
}

// newDepsWithSampler builds CPU's per-instance deps around an explicit sampler.
// NewDeps is its only caller and supplies the cgroup sampler; the split keeps
// the choice of sampler separate from the startup-snapshot construction below.
// The specs beside this file do not call it: they build CPUDeps directly around
// a stub Sampler, which skips the startup snapshot as well.
func newDepsWithSampler(id deps.Identity, bd *deps.BaseDependencies, sampler cpuhealth.Sampler) *CPUDeps {
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
