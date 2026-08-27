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
// with the pkg/cpuhealth library. It owns the sampler; the judgement is
// cpuhealth.Decide's.
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
	// WorkerType names this worker in config, in CSE storage, and in Ref.
	WorkerType = "cpu"

	// InstanceName names the child in Ref.
	InstanceName = "cpu"

	// FilesystemDepsKey is the register.SetDeps key under which a caller
	// publishes the filesystem.Service the sampler reads the cgroup files
	// through. Publish before the instance spawns: a caller that meant to
	// publish a fixture and forgot gets no error, and that instance silently
	// reads the real machine instead. NewDeps does the lookup.
	//
	// The key is not WorkerType: the typed deps registry keys on the string
	// alone, so two payloads cannot share one key. Same convention as
	// configworker.ConfigManagerDepsKey.
	FilesystemDepsKey = WorkerType + ".filesystem"

	// ClockDepsKey is the register.SetDeps key under which a caller publishes
	// the clock.Clock the sampler stamps every Sample from. NewDeps looks it up
	// per instance at spawn time — the instance then samples on that clock for
	// the rest of its life, so a publish after the spawn reaches nothing — and
	// falls back to the real clock when nothing is published. Same key
	// convention as FilesystemDepsKey.
	ClockDepsKey = WorkerType + ".clock"

	// cgroupBase is the cgroup v2 mount point whose CPU controller files the
	// sampler reads (cpu.stat, cpu.max, cpu.pressure, cpuset.cpus.effective).
	cgroupBase = "/sys/fs/cgroup"

	// pollInterval is how often the worker samples the cgroup. simple.Register
	// also publishes it as this worker's observation interval, and
	// pkg/fsmv2/adapter calls an observation stale at three times it.
	pollInterval = 1 * time.Second
)

// Ref is the pair the configworker upserts this child under behind
// USE_FSMV2_CPU, and that a reader fetches its status back under through
// fsmv2client.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

// CPUConfig is empty: the CPU worker takes no configuration.
type CPUConfig struct{}

// CPUStatus is the result of one CPU-health observation by this worker.
//
// No Go code outside this package reads a CPUStatus today. Its json tags still
// name fields in a stored document: simple.Status merges them into the top
// level of the persisted observation, so renaming one is a storage-format
// change.
//
// The measured numbers arrive as one whole cpuhealth.Details rather than field
// by field, so the wire shape under "details" is that type's own: a field added
// there reaches the wire with no change here.
type CPUStatus struct {
	// Verdict is the cpuhealth.State string Decide produced this tick ("healthy"
	// or "degraded"), and empty when the tick could not measure.
	Verdict string `json:"verdict"`

	// Message is the human-readable text Poll composed by calling
	// cpuhealth.ComposeMessage: a headline such as
	// "CPU healthy. This instance is using 0.0 of 2 cores (0% of its limit) and
	// can use 1.8 more before it is marked degraded.", then a Technical Details
	// table of the rules that can degrade this instance's CPU. A box under a
	// CPU limit is judged against two ceilings at once, its own limit and the
	// machine, and the table carries a headroom line for each. The
	// "CPU: starting up." message an instance shows for its first two ticks
	// carries the table too, and a line whose window has not reduced yet says
	// so instead of stating a figure. Only a failed cgroup read renders without
	// a table at all.
	Message string `json:"message"`

	// Details is the measured evidence behind the verdict, filled on every tick
	// that could measure. It is a named field, not an embed, so its keys nest
	// under "details" instead of flattening into the top level simple.Status
	// merges "reason" and "degraded" into alongside "verdict" and "message".
	Details cpuhealth.Details `json:"details"`
}

// CPUDeps is the per-instance state Poll reads and mutates.
//
// TDeps must be *CPUDeps, never the value: simple.MonitorSpec passes TDeps to
// Poll by value, so binding CPUDeps would hand each tick its own copy and lose
// the window anchor and the report latch below, while the map and the pointers
// kept working across the copy. Nothing enforces this.
type CPUDeps struct {
	*deps.BaseDependencies

	// sampler reads the cgroup. Behind the interface it is a pointer holding the
	// counter baselines every rate is derived from, so they survive the tick.
	sampler cpuhealth.Sampler
	// engine owns every (signal, instrument) window and per-signal latch. It is
	// nil when NewEngine failed at construction (engineErr is then set).
	engine *diagnosis.Engine[cpuhealth.Sample]
	// engineErr records a NewEngine failure. NewDeps cannot fail, so a table
	// that will not build has to surface at the next Poll instead, which reports
	// it could not measure.
	engineErr error

	// everMeasured keeps a signal that has measured once counting as measured
	// through a later read outage. Set the first tick a signal reads Ready, and
	// never cleared while the worker lives.
	everMeasured map[string]bool

	// admissionState is the admission window's anchor and its report latch.
	// admission.go has what the window is for.
	admissionState admission

	// table is held because engine.Select needs the Signal values, which are
	// only reachable through the table the engine was built from.
	table diagnosis.Table[cpuhealth.Sample]
}

// Poll samples the cgroup once and reports the verdict Decide judged. On a
// NewEngine construction error or a non-nil Read error it stores no verdict and
// reports it could not measure, never a healthy zero. One absent field (e.g.
// Pressure) on a nil error is not a failure: it reports what Decide produced.
func Poll(ctx context.Context, d *CPUDeps, _ CPUConfig) (CPUStatus, error) {
	if d.engineErr != nil {
		return CPUStatus{}, d.engineErr
	}

	sample, err := d.sampler.Read(ctx)
	if err != nil {
		return CPUStatus{}, err
	}

	env := cpuhealth.DeriveEnvironment(sample)
	verdict, details := cpuhealth.Decide(d.engine, sample, env)

	// After Decide, never before it, and on the same env. evidenceCounts says why.
	capable, measured, unmeasured := d.evidenceCounts(env)

	atDeadline := d.admissionState.shortfallAtDeadline(sample.Timestamp, measured, capable)

	// The message is a FIXED event name, never interpolated: sentry's
	// BuildFingerprint groups on the log entry's message verbatim, so a Sprintf
	// carrying signal names and counts would give every distinct combination its
	// own Sentry issue. Dynamic values ride in the structured fields.
	if atDeadline && d.admissionState.reportOnce() {
		d.GetLogger().SentryWarn(deps.FeatureSupportCPU, d.GetHierarchyPath(),
			"cpu_admission_deadline_never_measured_signal",
			deps.String("never_measured_signals", strings.Join(unmeasured, ", ")),
			deps.Int("signals_measured", measured),
			deps.Int("signals_capable", capable),
			deps.Duration("admission_window", admissionWindow))
	}

	message := cpuhealth.ComposeMessage(verdict, details)

	// Same fixed-event-name rule as the SentryWarn above: dynamic values ride
	// in the structured fields, never in the message.
	d.GetLogger().Debug("cpu_reading",
		deps.String("verdict", string(verdict.State)),
		deps.String("message", message),
	)

	return CPUStatus{
		Verdict: string(verdict.State),
		Message: message,
		Details: details,
	}, nil
}

// evidenceCounts answers three questions about this tick: how many signals this
// box can answer at all (capable — the signal is not NoInstrument), how many of
// those have ever answered (measured — everMeasured is set, which happens the
// first tick the signal reads Ready and never reverses), and which capable ones
// never have.
//
// It must run after Decide, on the same tick and the same env. Select reports a
// window's readiness without ageing it, so its answer is trustworthy only
// following an Observe on that tick, and Decide is what Observes.
func (d *CPUDeps) evidenceCounts(env diagnosis.Environment) (capable, measured int, unmeasured []string) {
	unmeasured = []string{}

	for _, s := range d.table.Signals {
		_, _, _, availability := d.engine.Select(s, env)
		if availability == diagnosis.NoInstrument {
			continue
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
// A failed startup read yields cores=0, quota=0, which drops the two capacity
// signals from this instance's table for its whole lifetime; a later
// successful read does not restore them (ENG-5752).
//
// The sampler reads through the filesystem.Service published under
// FilesystemDepsKey and stamps from the clock.Clock published under
// ClockDepsKey; each key's doc states the lookup timing and the fallback.
func NewDeps(_ deps.Identity, bd *deps.BaseDependencies) *CPUDeps {
	fs := register.GetDeps[filesystem.Service](FilesystemDepsKey)
	if fs == nil {
		fs = filesystem.NewDefaultService()
	}

	clk := register.GetDeps[clock.Clock](ClockDepsKey)
	if clk == nil {
		clk = clock.New()
	}

	sampler := cpuhealth.NewLinuxSamplerWithClock(fs, cgroupBase, clk)

	d := &CPUDeps{
		BaseDependencies: bd,
		sampler:          sampler,
	}

	cores, quota := containerOrHostLimit(context.Background(), sampler, bd)
	d.table = cpuhealth.Table(cores, quota)
	d.engine, d.engineErr = diagnosis.NewEngine(d.table)
	d.everMeasured = make(map[string]bool)

	return d
}

// containerOrHostLimit decides which limit cpuhealth judges CPU use against:
// the container's own resource limit, or the host's capacity. cpuhealth needs
// that answer in advance, because the table is built from it once and never
// rebuilt.
//
// It reads one snapshot and returns the cores the cgroup may use and its quota.
// A positive quota means the container has a CPU limit, so cpuhealth judges
// against that limit. A zero quota means the container has no limit of its own,
// so the host's capacity governs. Zero says unlimited, not that there is no CPU
// to use.
//
// Either return is also zero when the startup snapshot did not carry it, which
// thins the table for the instance's whole lifetime. NewDeps has the
// consequence.
func containerOrHostLimit(ctx context.Context, s cpuhealth.Sampler, bd *deps.BaseDependencies) (cores, quota float64) {
	smp, err := s.Read(ctx)
	if err != nil {
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

// healthFromStatus turns one poll's status into the worker's own health, so a
// degraded CPU verdict degrades the worker instead of leaving it reporting
// healthy. simple calls it after every good poll, and never after a failed one.
func healthFromStatus(_ CPUConfig, status CPUStatus) simple.Health {
	if status.Verdict == string(cpuhealth.StateDegraded) {
		return simple.Degraded(status.Message)
	}

	return simple.Healthy(status.Message)
}

// monitorSpec is this worker's whole definition. It is a package value rather
// than a literal inside init() so a spec can call exactly what the framework
// calls, wiring included.
var monitorSpec = simple.MonitorSpec[CPUConfig, CPUStatus, *CPUDeps]{
	WorkerType: WorkerType,
	Interval:   pollInterval,
	NewDeps:    NewDeps,
	Poll:       Poll,
	Health:     healthFromStatus,
}

func init() {
	simple.Register(monitorSpec)
}
