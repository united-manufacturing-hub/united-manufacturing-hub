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
	// through. Publish before the instance spawns: a caller that meant to
	// publish a fixture and forgot gets no error, and that instance silently
	// reads the real machine instead. NewDeps does the lookup.
	//
	// The key is not WorkerType: the typed deps registry keys on the string
	// alone, so two payloads cannot share one key. Same convention as
	// configworker.ConfigManagerDepsKey.
	FilesystemDepsKey = WorkerType + ".filesystem"

	// cgroupBase is the cgroup v2 mount point whose CPU controller files the
	// sampler reads (cpu.stat, cpu.max, cpu.pressure, cpuset.cpus.effective).
	cgroupBase = "/sys/fs/cgroup"

	// PollInterval is how often the worker samples the cgroup. simple.Register
	// also publishes it as this worker's observation interval, and
	// pkg/fsmv2/adapter calls an observation stale at three times it.
	PollInterval = 1 * time.Second
)

// Ref is the pair the configworker upserts this child under behind
// USE_FSMV2_CPU, and that a reader fetches its status back under through
// fsmv2client.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

// CPUConfig is empty: the CPU worker takes no configuration.
type CPUConfig struct{}

// CPUStatus is the result of one CPU-health observation by this worker.
//
// container_monitor reads it typed through fsmv2client.GetFresh. The json
// tags also name fields in a stored document, so renaming one is a
// storage-format change.
type CPUStatus struct {
	// Verdict is everything Decide produced this tick: the state, the
	// attribution of the dominant cause, and the ranked causes. It is empty
	// when the tick could not measure.
	Verdict cpuhealth.Verdict `json:"verdict"`

	// Message is what cpuhealth.ComposeMessage rendered: a headline, then a
	// Technical Details table with one headroom line per ceiling the instance is
	// judged against. Only a failed cgroup read renders without a table.
	Message string `json:"message"`

	// Details is the measured evidence behind the verdict, filled on every tick
	// that could measure. It is a named field rather than an embed, so its keys
	// nest under "details" instead of colliding with the "reason" and "degraded"
	// that simple.Status flattens to the top level.
	Details cpuhealth.Details `json:"details"`
}

// CPUDeps is the per-instance state Poll reads.
//
// TDeps must be *CPUDeps, never the value: simple.MonitorSpec passes TDeps to
// Poll by value, so state a field holds directly, rather than behind a pointer,
// would die with that copy. Nothing enforces this.
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

	recordGauges(d.MetricsRecorder(), details)

	return CPUStatus{
		Verdict: verdict,
		Message: cpuhealth.ComposeMessage(verdict, details),
		Details: details,
	}, nil
}

// recordGauges publishes the measured evidence for the framework's worker-metrics
// exporter, which turns each name into umh_fsmv2_worker_<name>.
//
// This does not run on a tick that could not measure, because Poll returns
// first. That does NOT mean the gauges go absent on a failed read: the
// collector still runs (simple.CollectObservedState reports a poll failure as
// a degraded observation, not as an error), so it reloads the previous gauge
// map from CSE and re-publishes every value. A persistent read failure
// therefore freezes this whole family at its last reading, and the worker's own
// state metric is what reveals it — not anything here.
func recordGauges(m *deps.MetricsRecorder, det cpuhealth.Details) {
	// Every gauge is set on every measured tick, including one where a signal
	// was not ready, and the flags below are what say which reading to trust.
	// Skipping the set would be worse than publishing a 0: the exporter creates
	// gauges lazily and never deletes one (ExportWorkerMetrics only ever writes
	// the drained map), so an omitted gauge keeps being scraped at its previous
	// value, with nothing to mark it stale.
	m.SetGauge(deps.GaugeCPUAvgUsageCores, det.AvgUsageCores)
	m.SetGauge(deps.GaugeCPUAvgUsageFraction, det.AvgUsageFraction)
	m.SetGauge(deps.GaugeCPUThrottleRatio, det.ThrottleRatio)
	m.SetGauge(deps.GaugeCPUPressureAvg60, det.PressureAvg60)
	m.SetGauge(deps.GaugeCPUHostHeadroomCores, det.HostHeadroomCores)
	m.SetGauge(deps.GaugeCPUAvgHostBusyCores, det.AvgHostBusyCores)
	// These three carry no companion flag and need none: capacity and the host
	// CPU count are 0 only on a read that never reaches here, and 0 is not a
	// value either can legitimately take; the reserve is a fixed fraction of
	// capacity, so it is readable whenever capacity is.
	m.SetGauge(deps.GaugeCPUCapacityCores, det.CapacityCores)
	m.SetGauge(deps.GaugeCPUReserveCores, det.ReserveCores)
	m.SetGauge(deps.GaugeCPUHostCpus, det.HostCpus)

	// The readability half. cpu_throttle_ratio, cpu_pressure_avg60 and the two
	// usage gauges all report 0 for an absent or untrusted signal, so a consumer
	// needs these to tell "not throttled" from "no throttle signal".
	m.SetGauge(deps.GaugeCPUUsageRingActive, gaugeBool(det.UsageRingActive))
	m.SetGauge(deps.GaugeCPUHostBusyRingActive, gaugeBool(det.HostBusyRingActive))
	m.SetGauge(deps.GaugeCPUHostHeadroomAvailable, gaugeBool(det.HostHeadroomAvailable))
	m.SetGauge(deps.GaugeCPUThrottleSignalReady, gaugeBool(det.ThrottleSignalReady))
	m.SetGauge(deps.GaugeCPUPressureSignalReady, gaugeBool(det.PressureSignalReady))
}

// gaugeBool is the flag encoding each flag gauge's own doc comment states: 1
// for true, 0 for false. It is not carried in the exported Help string, which
// the framework hardcodes to "Worker metric: <name>".
func gaugeBool(b bool) float64 {
	if b {
		return 1
	}

	return 0
}

// NewDeps builds CPU's per-instance deps. It constructs a cgroup sampler
// (precedent: pkg/fsm/container/machine.go), takes one startup snapshot through
// it, and builds the table and engine.
//
// A failed startup read yields cores=0, quota=0, which drops the two capacity
// signals from this instance's table for its whole lifetime; a later
// successful read does not restore them (ENG-5752).
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

	cores, quota := containerOrHostLimit(context.Background(), sampler, bd)
	table := cpuhealth.Table(cores, quota)
	d.engine, d.engineErr = diagnosis.NewEngine(table)

	return d
}

// containerOrHostLimit decides which limit cpuhealth judges CPU use against:
// the container's own resource limit, or the host's capacity. cpuhealth needs
// that answer in advance, because the table is built from it once and never
// rebuilt.
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

// healthFromStatus turns one poll's verdict into the worker's own health.
// simple calls it after every good poll, and never after a failed one.
func healthFromStatus(_ CPUConfig, status CPUStatus) simple.Health {
	if status.Verdict.State == cpuhealth.StateDegraded {
		return simple.Degraded(status.Message)
	}

	return simple.Healthy(status.Message)
}

// monitorSpec is this worker's whole definition. It is a package value rather
// than a literal inside init() so a spec can call exactly what the framework
// calls, wiring included.
var monitorSpec = simple.MonitorSpec[CPUConfig, CPUStatus, *CPUDeps]{
	WorkerType: WorkerType,
	Interval:   PollInterval,
	NewDeps:    NewDeps,
	Poll:       Poll,
	Health:     healthFromStatus,
}

func init() {
	simple.Register(monitorSpec)
}
