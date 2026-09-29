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
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
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

	// FilesystemDepsKey is the register.SetGlobalDeps key under which a caller
	// publishes the filesystem.Service the sampler reads the cgroup files
	// through. Publish before the instance spawns: a caller that meant to
	// publish a fixture and forgot gets no error, and that instance silently
	// reads the real machine instead. NewDeps does the lookup.
	//
	// A key holds one value, so each payload gets its own key rather than
	// WorkerType. configworker.ConfigManagerDepsKey follows the same convention.
	// A value in the worker's dependency map wins over what is published here:
	// the same literal, cpu.filesystem, also names the map key
	// (FilesystemKey), which NewDeps reads before this global.
	FilesystemDepsKey = WorkerType + ".filesystem"

	// cgroupBase is the cgroup mount point: the v2 hierarchy itself, or on v1
	// the directory that holds the controller mounts.
	cgroupBase = "/sys/fs/cgroup"

	// PollInterval is how often the worker samples the cgroup. simple.Register
	// also publishes it as this worker's observation interval, and
	// pkg/fsmv2/adapter calls an observation stale at three times it.
	PollInterval = 1 * time.Second

	// MaxObservationAge is the oldest a reading may be and still count as
	// Fresh for fsmv2client.GetFresh. The container monitor and the CPU
	// scenarios both use it, so the two cannot drift; one slow or missed poll
	// cannot flip the instance to degraded.
	MaxObservationAge = 3 * PollInterval
)

// Ref is the pair the configworker upserts this child under behind
// USE_FSMV2_CPU, and that a reader fetches its status back under through
// fsmv2client.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

// FilesystemKey names the filesystem.Service the sampler reads the cgroup
// files through. NewDeps does the lookup, then the global published under
// FilesystemDepsKey, and falls back to filesystem.NewDefaultService(). The
// same literal, cpu.filesystem, also names that global slot
// (FilesystemDepsKey); the map key here is the one read first.
var FilesystemKey = config.NewDependencyKey[filesystem.Service]("cpu.filesystem")

// ClockKey names the clock.Clock the sampler stamps every Sample from. NewDeps
// does the lookup and falls back to clock.New() when the map holds nothing
// under it: a scenario that meant to publish a mock clock and forgot gets no
// error, and its samples are stamped from wall time instead.
var ClockKey = config.NewDependencyKey[clock.Clock]("cpu.clock")

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
// TDeps is *CPUDeps rather than the value because simple.MonitorSpec passes
// TDeps to Poll by value: state held directly in a field, rather than behind a
// pointer, would die with that copy.
type CPUDeps struct {
	*deps.BaseDependencies

	// fs is the filesystem the sampler reads through, kept beside it so the
	// injection specs can name which source NewDeps resolved it to: the map
	// entry, the published global, or the real machine.
	fs filesystem.Service

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
	// reportedReads holds every {operation, outcome} already reported, so a failure
	// repeating each tick reports once. Startup and Poll share this one map;
	// two maps would re-report a startup failure on the first tick.
	reportedReads sync.Map // map[cpuhealth.ReadResult]struct{}
}

// Poll samples the cgroup once and reports the verdict Decide judged. On a
// NewEngine construction error or a non-nil Read error it stores no verdict,
// publishes no gauges, and reports it could not measure, never a healthy zero.
// One absent field (e.g. Pressure) on a nil error is not a failure: it reports
// what Decide produced.
func Poll(ctx context.Context, d *CPUDeps, _ CPUConfig) (CPUStatus, error) {
	if d.engineErr != nil {
		return CPUStatus{}, d.engineErr
	}

	sample, err := d.sampler.Read(ctx)

	// Called before the error return below: Read fills Sample.Troubleshooting.Reads even when it
	// errors, so the read that broke is named either way.
	d.reportFailedReads(ctx, sample)

	if err != nil {
		return CPUStatus{}, err
	}

	env := cpuhealth.DeriveEnvironment(sample)
	verdict, details := cpuhealth.Decide(d.engine, sample, env)

	recordMetrics(d.MetricsRecorder(), sample.Timestamp, details)

	return CPUStatus{
		Verdict: verdict,
		Message: cpuhealth.ComposeMessage(verdict, details),
		Details: details,
	}, nil
}

// recordMetrics publishes the evidence for the framework's worker-metrics
// exporter, which turns each name into umh_fsmv2_worker_<name>
// (WorkerMetricsExporter.getOrCreateGauge, pkg/fsmv2/supervisor/metrics/metrics.go).
func recordMetrics(m *deps.MetricsRecorder, sampledAt time.Time, det cpuhealth.Details) {
	// GaugeCPULastSampleUnix freezes along with every gauge below when a tick
	// cannot measure: Poll returns before recordMetrics runs, and the collector
	// reloads and re-publishes the previous gauge values instead
	// (Collector.wrapNewObservation, pkg/fsmv2/supervisor/internal/collection/collector.go).
	// Its age is what reveals the freeze.
	m.SetGauge(deps.GaugeCPULastSampleUnix, float64(sampledAt.Unix()))

	m.SetGauge(deps.GaugeCPUAvgUsageCores, det.AvgUsageCores)
	m.SetGauge(deps.GaugeCPUAvgUsageFraction, det.AvgUsageFraction)
	m.SetGauge(deps.GaugeCPUThrottleRatio, det.ThrottleRatio)
	m.SetGauge(deps.GaugeCPUPressureAvg60, det.PressureAvg60)
	m.SetGauge(deps.GaugeCPUHostHeadroomCores, det.HostHeadroomCores)
	m.SetGauge(deps.GaugeCPUAvgHostBusyCores, det.AvgHostBusyCores)
	m.SetGauge(deps.GaugeCPUCapacityCores, det.CapacityCores)
	m.SetGauge(deps.GaugeCPUReserveCores, det.ReserveCores)
	m.SetGauge(deps.GaugeCPUHostCpus, det.HostCpus)

	m.SetGaugeFlag(deps.GaugeCPUUsageRingActive, det.UsageRingActive)
	m.SetGaugeFlag(deps.GaugeCPUHostBusyRingActive, det.HostBusyRingActive)
	m.SetGaugeFlag(deps.GaugeCPUHostBusyCoresAvailable, det.HostBusyCoresAvailable)
	m.SetGaugeFlag(deps.GaugeCPUHostHeadroomAvailable, det.HostHeadroomAvailable)
	m.SetGaugeFlag(deps.GaugeCPUThrottleSignalReady, det.ThrottleSignalReady)
	m.SetGaugeFlag(deps.GaugeCPUPressureSignalReady, det.PressureSignalReady)
}

// NewDeps builds CPU's per-instance deps. It constructs a cgroup sampler
// (precedent: pkg/fsm/container/machine.go) over the first filesystem that
// provides one, takes one startup snapshot through it, and builds the table
// and engine.
//
// A read that fails at startup leaves its own figure zero, which drops that
// capacity signal from this instance's table for its whole lifetime; a later
// successful read does not restore it (ENG-5752).
func NewDeps(_ deps.Identity, bd *deps.BaseDependencies, dependencies map[string]any) *CPUDeps {
	fs, ok := config.LookupDependency(dependencies, FilesystemKey)
	if !ok {
		fs = register.GlobalDeps[filesystem.Service](FilesystemDepsKey)
	}
	if fs == nil {
		fs = filesystem.NewDefaultService()
	}

	clk, ok := config.LookupDependency(dependencies, ClockKey)
	if !ok {
		clk = clock.New()
	}

	sampler := cpuhealth.NewLinuxSamplerWithClock(fs, cgroupBase, clk)

	d := &CPUDeps{
		BaseDependencies: bd,
		fs:               fs,
		sampler:          sampler,
	}

	cores, quota := containerOrHostLimit(context.Background(), sampler, d)
	table := cpuhealth.Table(cores, quota)
	d.engine, d.engineErr = diagnosis.NewEngine(table)

	return d
}

// containerOrHostLimit takes the one snapshot the table is built from, and
// reports any read that failed while taking it. Each figure comes from its own
// read and is zero when that read gave nothing; NewDeps says what a zero costs
// the instance (ENG-5752).
//
// NewDeps calls this before setting d.engine, so d.engine is nil here.
func containerOrHostLimit(ctx context.Context, sampler cpuhealth.Sampler, d *CPUDeps) (cores, quota float64) {
	// The error is discarded because it carries nothing the sample does not:
	// an unparsable cpu.stat is recorded on sample.Troubleshooting.Reads, which
	// is where reportFailedReads reads it, and a cancelled tick is a shutdown
	// rather than a failure.
	sample, _ := sampler.Read(ctx)
	d.reportFailedReads(ctx, sample)

	return limitsFromSample(sample)
}

// limitsFromSample reads the capacity figures off one sample.
func limitsFromSample(sample cpuhealth.Sample) (cores, quota float64) {
	if logicalCpus, ok := sample.LogicalCpus.Get(); ok {
		cores = logicalCpus
	}

	if limit, ok := sample.Quota.Get(); ok && limit > 0 {
		quota = limit
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
