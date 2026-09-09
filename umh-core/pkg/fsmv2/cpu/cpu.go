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
	// reportedReads holds every {op, outcome} already reported, so a failure
	// repeating each tick reports once. Startup and Poll share this one map;
	// two maps would re-report a startup failure on the first tick.
	reportedReads sync.Map // map[cpuhealth.ReadResult]struct{}
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

	// Called before the error return below: Read fills Sample.Reads even when it
	// errors, so the read that broke is named either way.
	cores, quota := limitsFromSample(sample)
	reportFailedReads(ctx, sample, err, cores, quota, d)

	if err != nil {
		return CPUStatus{}, err
	}

	env := cpuhealth.DeriveEnvironment(sample)
	verdict, details := cpuhealth.Decide(d.engine, sample, env)

	return CPUStatus{
		Verdict: verdict,
		Message: cpuhealth.ComposeMessage(verdict, details),
		Details: details,
	}, nil
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

	cores, quota := containerOrHostLimit(context.Background(), sampler, d)
	table := cpuhealth.Table(cores, quota)
	d.engine, d.engineErr = diagnosis.NewEngine(table)

	return d
}

// containerOrHostLimit decides which limit cpuhealth judges CPU use against:
// the container's own resource limit, or the host's capacity. cpuhealth needs
// that answer in advance, because the table is built from it once and never
// rebuilt.
//
// NewDeps calls this before setting d.engine, so d.engine is nil here.
func containerOrHostLimit(ctx context.Context, s cpuhealth.Sampler, d *CPUDeps) (cores, quota float64) {
	smp, err := s.Read(ctx)
	cores, quota = limitsFromSample(smp)

	reportFailedReads(ctx, smp, err, cores, quota, d)

	return cores, quota
}

// limitsFromSample reads the capacity figures off one sample.
func limitsFromSample(smp cpuhealth.Sample) (cores, quota float64) {
	if lc, ok := smp.LogicalCpus.Get(); ok {
		cores = lc
	}

	if q, ok := smp.Quota.Get(); ok && q > 0 {
		quota = q
	}

	return cores, quota
}

// reportedReadOps are the reads that get a Sentry event when they fail: each
// one carries a fact the verdict needs. The other three ops are listed as
// fields on somebody else's event and never get one of their own.
var reportedReadOps = map[cpuhealth.ReadOp]struct{}{
	cpuhealth.OpProcStat:    {},
	cpuhealth.OpProcCpuinfo: {},
	cpuhealth.OpCPUStat:     {},
	cpuhealth.OpCPUMax:      {},
	cpuhealth.OpCPUPressure: {},
	cpuhealth.OpCpusetCPUs:  {},
}

// excusedReads are the failures that report nothing, because the file is
// legitimately absent on some kernels: a kernel without PSI serves no
// cpu.pressure at all. A cpu.pressure that exists and will not open does
// report.
var excusedReads = map[cpuhealth.ReadResult]struct{}{
	{Op: cpuhealth.OpCPUPressure, Outcome: cpuhealth.ReadENOENT}: {},
}

// readOpPaths is the file each reported read opens, a field not a message part.
var readOpPaths = map[cpuhealth.ReadOp]string{
	cpuhealth.OpProcStat:    "/proc/stat",
	cpuhealth.OpProcCpuinfo: "/proc/cpuinfo",
	cpuhealth.OpCPUStat:     cgroupBase + "/cpu.stat",
	cpuhealth.OpCPUMax:      cgroupBase + "/cpu.max",
	cpuhealth.OpCPUPressure: cgroupBase + "/cpu.pressure",
	cpuhealth.OpCpusetCPUs:  cgroupBase + "/cpuset.cpus.effective",
}

const (
	// The message is one of these prefixes, the op and the outcome, and nothing
	// else: Sentry groups on it, so a path or a count would mint an issue per
	// value. read_failed means the sample survived without one signal.
	readFailedPrefix = "cpu::read_failed::"
	// sampleFailedPrefix means the failure voided the whole sample.
	sampleFailedPrefix = "cpu::sample_failed::"
	readFailedSep      = "::"
)

// reportFailedReads emits one Sentry event per failed read.
//
// ReadNotAttempted never reports: one failure stops several later reads, so
// reporting those would turn one root cause into several issues. The verb
// belongs to the read it is reported under, and only cpu.stat can void a
// sample, so a cpu.pressure failing in the same tick stays read_failed.
func reportFailedReads(ctx context.Context, smp cpuhealth.Sample, readErr error, cores, quota float64, d *CPUDeps) {
	// Shutdown is not a failure. filesystem.DefaultService.ReadFile checks the
	// context, so once it is done every read fails and a graceful shutdown would
	// emit an event per read on every instance.
	if ctx.Err() != nil {
		return
	}

	for _, r := range smp.Reads {
		if _, reported := reportedReadOps[r.Op]; !reported {
			continue
		}

		if r.Outcome == cpuhealth.ReadOK || r.Outcome == cpuhealth.ReadNotAttempted {
			continue
		}

		if _, excused := excusedReads[r]; excused {
			continue
		}

		if _, reportedBefore := d.reportedReads.LoadOrStore(r, struct{}{}); reportedBefore {
			continue
		}

		prefix := readFailedPrefix
		if r.Op == cpuhealth.OpCPUStat && readErr != nil {
			prefix = sampleFailedPrefix
		}

		d.GetLogger().SentryWarn(deps.FeatureSupportCPU, d.GetHierarchyPath(),
			prefix+string(r.Op)+readFailedSep+string(r.Outcome),
			readFailureFields(smp, r.Op, cores, quota)...)
	}
}

// readFailureFields is one failed-read event's evidence.
func readFailureFields(smp cpuhealth.Sample, failed cpuhealth.ReadOp, cores, quota float64) []deps.Field {
	fields := []deps.Field{
		deps.String("path", readOpPaths[failed]),
		deps.String("cgroup_base", cgroupBase),
		deps.String("cgroup_controllers_raw", smp.CgroupControllersRaw),
		deps.String("cpu_max_raw", smp.CPUMaxRaw),
		deps.String("cpu_stat_raw", smp.CPUStatRaw),
		deps.String("proc_self_cgroup_raw", smp.ProcSelfCgroupRaw),
		deps.Int("cgroup_base_dir_entry_count", smp.BaseDirEntryCount),
	}

	// Every sibling is reported, a never-attempted one included: the pattern
	// across the reads is what says which shape a machine is in.
	for _, r := range smp.Reads {
		if r.Op == failed {
			continue
		}

		fields = append(fields, deps.String(string(r.Op)+"_read", string(r.Outcome)))
	}

	if hostCpus, ok := smp.HostCpus.Get(); ok {
		fields = append(fields, deps.Float64("host_cpus", hostCpus))
	}

	// capacity_cores is what the table would be built against. Zero means
	// neither read answered.
	capacity := cores
	if quota > 0 {
		capacity = quota
	}

	if capacity > 0 {
		fields = append(fields, deps.Float64("capacity_cores", capacity))
	}

	return fields
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
