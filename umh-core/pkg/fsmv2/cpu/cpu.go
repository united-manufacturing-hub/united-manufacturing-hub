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
	// WorkerType names this worker in config, in CSE storage, and in Ref.
	WorkerType = "cpu"

	// InstanceName names the child in Ref.
	InstanceName = "cpu"

	// FilesystemDepsKey is the register.SetDeps key under which a caller
	// publishes the filesystem.Service the sampler reads the cgroup files
	// through. NewDeps looks it up per instance at spawn time.
	//
	// Publishing is optional: with nothing published here NewDeps falls back to
	// the real filesystem, rather than failing the way the transport pull worker
	// does when its deps are missing. Production wants the real filesystem and
	// should not have to publish one to get it.
	//
	// The price falls on a caller who meant to publish a fixture and forgot. On
	// Linux the real cgroup files read fine, so that caller gets a verdict
	// computed from the real machine, and a loose assertion such as one
	// expecting "healthy" on an idle box passes on numbers it never staged. An
	// assertion that names something only the published filesystem can produce
	// does not have that hole.
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
	// It also sets a staleness bound. simple.Register publishes it as this
	// worker's observation interval, and pkg/fsmv2/adapter calls an observation
	// stale at three times that (staleAfterFor). Nothing reads the CPU worker
	// through that adapter yet, so today the number only moves the cadence.
	pollInterval = 1 * time.Second
)

// Ref identifies the CPU monitor child by the (WorkerType, InstanceName) pair
// above. The configworker reconciles exactly one such child, upserting it under
// this pair behind USE_FSMV2_CPU, and a reader fetches its status back under
// the same pair through fsmv2client.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

// CPUConfig is deliberately empty. A config that carried values would make the
// configworker re-upsert and respawn the child every tick, and each respawn
// discards every 60s window the engine had warmed.
type CPUConfig struct{}

// CPUStatus is the result of one CPU-health observation. It carries judgements
// and counts, never a raw measurement such as a CPU-utilisation percentage.
//
// The struct is not itself a wire shape; two of its five fields are. Verdict and
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
	// NoInstrument). Its reader is the scenario harness, which asserts on it to
	// separate capability from readability: a signal stays capable after the file
	// it reads disappears, because the box still has the instrument. Nothing in
	// the Management Console reads it.
	SignalsCapable int `json:"signalsCapable"`
	// SignalsMeasured is how many capable signals have produced a first
	// measurement since this worker started. Nothing reads it.
	SignalsMeasured int `json:"signalsMeasured"`

	// RefusingAdmission reports whether admission is currently refused: a
	// capable signal has not first-measured (measured < capable) within the
	// admission window. Nothing consumes it; the contract below binds whatever
	// first does. Consume the flag; do not re-derive the count comparison,
	// which drops the window bound. It is meaningful only on a successful read
	// — an errored/empty Poll yields this false ('no determination'), which a
	// consumer must not read as 'admission open'.
	RefusingAdmission bool `json:"refusingAdmission"`
}

// CPUDeps is the per-instance state Poll reads and mutates.
//
// TDeps must be *CPUDeps, never the value: Poll takes d by value, so a value
// CPUDeps would hand each tick its own copy and lose every mutation to a plain
// field. The failure would be partial rather than total, and so quiet — the map
// and the pointers below share their contents across a copy, while adm's window
// anchor and its report latch would reset on every tick.
//
// Nothing enforces this. The spec that did was deleted along with the counter it
// watched, knowing that cost. pkg/fsmv2/simple's own isolation spec is not a
// substitute: it binds a pointer as its TDeps and checks that one instance's
// mutations stay out of another's, so it never exercises the value case this
// paragraph forbids.
type CPUDeps struct {
	*deps.BaseDependencies

	// sampler reads the cgroup. Behind the interface it is a pointer, and the
	// counter baselines each rate is derived from live in the sources that
	// pointer owns, so both survive the tick.
	sampler cpuhealth.Sampler
	// engine owns every (signal, instrument) window and per-signal latch. It is
	// nil when NewEngine failed at construction (engineErr is then set).
	engine *diagnosis.Engine[cpuhealth.Sample]
	// engineErr records a NewEngine failure. NewDeps cannot fail, so a table
	// that will not build has to surface at the next Poll instead, which reports
	// it could not measure. The alternative is Decide on a nil engine, which
	// panics at the supervisor: simple has no recover around Poll, and recovery
	// is at the collector.
	engineErr error

	// everMeasured keeps a signal that has measured once counting as measured
	// through a later read outage. Set the first tick a signal reads Ready, and
	// never cleared while the worker lives; a respawn builds a new worker, so it
	// starts empty again.
	everMeasured map[string]bool

	// adm is the admission window's state: its anchor and its report latch.
	// admission.go has what the window is for.
	adm admission

	// table is the declaration Poll walks: engine.Select needs the Signal values,
	// which are only reachable through the table the engine was built from.
	table diagnosis.Table[cpuhealth.Sample]
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

	// After Decide, never before it, and on the same env. evidenceCounts says why.
	capable, measured, unmeasured := d.evidenceCounts(env)

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
	}, nil
}

// evidenceCounts walks the signal table and answers three questions about this
// tick: how many signals this box can answer at all, how many of those have
// ever answered, and which capable ones never have.
//
// Capable means the signal is not NoInstrument — the box has some instrument
// that could answer it. Measured means everMeasured is set, which happens the
// first tick the signal reads Ready and never reverses. So a signal counts as
// capable-but-unmeasured only if nothing has ever read it successfully, which
// is the shortfall the admission window is about.
//
// It must run after Decide, on the same tick and the same env. Select reports a
// window's readiness without ageing it, so its answer is trustworthy only
// following an Observe on that tick, and Decide is what Observes.
func (d *CPUDeps) evidenceCounts(env diagnosis.Environment) (capable, measured int, unmeasured []string) {
	unmeasured = []string{}

	for _, s := range d.table.Signals {
		_, _, _, availability := d.engine.Select(s, env)
		if availability == diagnosis.NoInstrument {
			continue // no instrument on this box: not capable, cannot refuse
		}

		if availability == diagnosis.Ready {
			d.everMeasured[s.Name] = true
		}

		if d.everMeasured[s.Name] {
			measured++
		} else {
			unmeasured = append(unmeasured, s.Name)
		}

		capable++
	}

	return capable, measured, unmeasured
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
func NewDeps(_ deps.Identity, bd *deps.BaseDependencies) *CPUDeps {
	fs := register.GetDeps[filesystem.Service](FilesystemDepsKey)
	if fs == nil {
		fs = filesystem.NewDefaultService()
	}

	sampler := cpuhealth.NewLinuxSampler(fs, cgroupBase)

	d := &CPUDeps{
		BaseDependencies: bd,
		sampler:          sampler,
	}

	// The table and engine are built once, here, from the startup snapshot. Both
	// cores and quota are startup facts; a quota change at runtime needs a
	// rebuilt table, which this worker does not do. The table is held because
	// Poll walks table.Signals for per-signal Availability.
	cores, quota := startupCapacity(context.Background(), sampler, bd)
	d.table = cpuhealth.Table(cores, quota)
	d.engine, d.engineErr = diagnosis.NewEngine(d.table)
	d.everMeasured = make(map[string]bool)

	return d
}

// startupCapacity takes the one snapshot cpuhealth.Table is called with. Table
// fixes the signal set as it builds: a positive core count adds the
// host-capacity signal, and a positive quota adds the container-limit signal.
//
// It returns those two numbers: the cores the cgroup may use, and its quota.
// Either is zero when the snapshot did not carry it.
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
