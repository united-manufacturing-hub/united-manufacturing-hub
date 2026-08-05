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

	// pollInterval is the poll cadence. It sets both the poll cadence and, at
	// 3x, the seam's maxAge downstream; the two are one decision (SPEC §9 P3 R1).
	pollInterval = 1 * time.Second
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

// CPUStatus is the result of one CPU-health observation.
//
// R1 settles the field set. It reports the verdict (not a raw measurement) and
// the customer-visible message; the two counts stay zero until R4 fills them.
type CPUStatus struct {
	// Verdict is the ranked judgement Decide produced this tick, as the
	// cpuhealth.State string ("healthy" or "degraded"). Empty when the tick
	// could not measure.
	Verdict string `json:"verdict"`

	// Message is ComposeMessage's output for this tick's verdict and signals.
	Message string `json:"message"`

	// SignalsCapable is how many CPU signals this box can answer (not
	// NoInstrument). Filled by R4; zero until then.
	SignalsCapable int `json:"signalsCapable"`
	// SignalsMeasured is how many capable signals have produced a first
	// measurement since this worker started. Filled by R4; zero until then.
	SignalsMeasured int `json:"signalsMeasured"`

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
	// table is the declaration R4 walks: engine.Select needs the Signal values,
	// which are only reachable through the table the engine was built from.
	table diagnosis.Table[cpuhealth.Sample]
	// engineErr records a NewEngine failure from NewDeps. NewDeps cannot fail, so
	// a table that will not build is reported through Poll: the worker stores no
	// verdict and reports it could not measure, instead of calling Decide on a
	// nil engine, which panics at the supervisor (simple has no recover around
	// Poll).
	engineErr error

	// polls counts completed observations. It is the non-pointer mutation the
	// R1 two-tick spec guards.
	polls uint64

	// firstFilled records, per signal name, whether it has ever reduced to a
	// Ready value since this worker started. Set the first tick that signal's
	// Availability is Ready; never cleared while the worker lives. A respawn
	// builds a new worker and a new engine, so it clears exactly when F10 says
	// it should. This bit is what keeps a signal that already measured counting
	// as measured through a later read outage (R4 spec 4).
	firstFilled map[string]bool
}

// NewDeps builds CPU's per-instance deps. It constructs a real cgroup sampler
// (precedent: pkg/fsm/container/machine.go), takes one startup snapshot through
// it, and builds the table and engine. NewDeps cannot fail — it returns TDeps
// and nothing else — so a startup snapshot whose read fails yields cores=0,
// quota=0, which silently drops the quota-dependent signals (throttling,
// limit-saturation) from the table for this instance's whole lifetime; a later
// read that succeeds does NOT restore them. Only a table that will not build
// (engineErr) makes Poll report it could not measure — a read failure at
// construction yields a healthy first verdict from a permanently thinned
// table, which is why the startup read error is logged.
func NewDeps(id deps.Identity, bd *deps.BaseDependencies) *CPUDeps {
	s := cpuhealth.NewCgroupSampler(filesystem.NewDefaultService(), "/sys/fs/cgroup")

	d := &CPUDeps{
		BaseDependencies: bd,
		sampler:          s,
	}

	// The table and engine are built once, at construction, from the startup
	// snapshot. Both cores and quota are startup facts; a quota change at
	// runtime needs a rebuilt table, which is out of P3 scope. A table that
	// will not build leaves the engine nil and sets engineErr, which Poll
	// reports as could-not-measure rather than letting Decide panic on a nil
	// engine (simple has no recover around Poll; recovery is at the collector).
	// The table is held so R4 can walk table.Signals for per-signal Availability.
	cores, quota := startupCapacity(context.Background(), s, bd)
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

	env := cpuhealth.DeriveEnvironment(sample)
	verdict, signals := cpuhealth.Decide(d.engine, sample, env)

	// The absence-of-evidence counts (R4), from the same walk Decide used: the
	// SAME env, the same tick, after Decide returns. engine.Select returns each
	// signal's Availability; capable means not NoInstrument (something on this
	// box can answer it), measured means its first-fill bit is set. The bit is
	// set the first tick that signal is Ready and never cleared, so a signal
	// that has measured keeps counting as measured through a later read outage.
	capable, measured := 0, 0
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
		}
		capable++
	}

	d.polls++

	return CPUStatus{
		Verdict:         string(verdict.State),
		Message:         cpuhealth.ComposeMessage(verdict, signals),
		SignalsCapable:  capable,
		SignalsMeasured: measured,
		Polls:           d.polls,
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
