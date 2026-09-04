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

	reportFailedReads(smp, cores, quota, bd)

	return cores, quota
}

// reportedReadOps are the reads whose failure mints a Sentry event: each one
// carries a fact the verdict is computed from, so a failed read of it leaves a
// measurement missing.
//
// The three evidence ops are deliberately absent. They exist to be carried on
// an event, never to produce one. OpCPUStat is absent for a different reason: a
// cpu.stat failure fails the whole sample, so containerOrHostLimit's own
// warning below already reports it, and minting a second event here would split
// one cause across two Sentry issues.
var reportedReadOps = map[cpuhealth.ReadOp]struct{}{
	cpuhealth.OpProcStat:    {},
	cpuhealth.OpProcCpuinfo: {},
	cpuhealth.OpCPUMax:      {},
	cpuhealth.OpCPUPressure: {},
	cpuhealth.OpCpusetCPUs:  {},
}

// excusedReads are the (op, outcome) pairs that report nothing even though the
// read yielded no value, because that pair is a normal platform difference
// rather than a fault.
//
// A kernel built without PSI serves no cpu.pressure at all, so its absence is
// the expected state on those machines. (OpCPUPressure, ReadEACCES) is
// deliberately not excused: a cpu.pressure that exists and cannot be opened is
// a real failure.
var excusedReads = map[cpuhealth.ReadResult]struct{}{
	{Op: cpuhealth.OpCPUPressure, Outcome: cpuhealth.ReadENOENT}: {},
}

// readOpPaths is the file each reported read opens. The path is reported as a
// field: in the message it would mint one Sentry issue per distinct path.
var readOpPaths = map[cpuhealth.ReadOp]string{
	cpuhealth.OpProcStat:    "/proc/stat",
	cpuhealth.OpProcCpuinfo: "/proc/cpuinfo",
	cpuhealth.OpCPUMax:      cgroupBase + "/cpu.max",
	cpuhealth.OpCPUPressure: cgroupBase + "/cpu.pressure",
	cpuhealth.OpCpusetCPUs:  cgroupBase + "/cpuset.cpus.effective",
}

const (
	// readFailedPrefix opens the message of every failed-read event. The whole
	// message is this prefix, the op and the outcome, joined by readFailedSep
	// and nothing else: the message is a Sentry grouping component, so a path,
	// an error text or a count in it would mint a new issue per distinct value.
	readFailedPrefix = "cpu::read_failed::"
	// readFailedSep joins the op and the outcome in that message.
	readFailedSep = "::"
)

// reportFailedReads emits one Sentry event per read that failed, carrying the
// evidence needed to tell one failure shape from another without logging into
// the machine.
//
// ReadNotAttempted never reports. A read that did not happen has no failure to
// name, and one failure stops several later reads — a cpu.stat failure stops
// four — so reporting those would turn one root cause into five issues.
func reportFailedReads(smp cpuhealth.Sample, cores, quota float64, bd *deps.BaseDependencies) {
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

		bd.GetLogger().SentryWarn(deps.FeatureSupportCPU, bd.GetHierarchyPath(),
			readFailedPrefix+string(r.Op)+readFailedSep+string(r.Outcome),
			readFailureFields(smp, r.Op, cores, quota)...)
	}
}

// readFailureFields is the evidence carried on one failed-read event: the file
// that failed, the raw text of every evidence read, what each other read
// produced this tick, and the two numbers the failure has consequences for.
func readFailureFields(smp cpuhealth.Sample, failed cpuhealth.ReadOp, cores, quota float64) []deps.Field {
	fields := []deps.Field{
		deps.String("path", readOpPaths[failed]),
		deps.String("cgroup_base", cgroupBase),
		deps.String("cgroup_controllers_raw", smp.ControllersRaw),
		deps.String("cpu_max_raw", smp.CPUMaxRaw),
		deps.String("cpu_stat_raw", smp.CPUStatRaw),
		deps.String("proc_self_cgroup_raw", smp.ProcSelfCgroupRaw),
		deps.Int("cgroup_base_entry_count", smp.BaseEntryCount),
	}

	// What every OTHER read produced on the same tick. One outcome on its own
	// rarely says which shape a machine is in; the pattern across the reads
	// does, which is why a sibling that was never attempted is reported too.
	for _, r := range smp.Reads {
		if r.Op == failed {
			continue
		}

		fields = append(fields, deps.String(string(r.Op)+"_read", string(r.Outcome)))
	}

	if hostCpus, ok := smp.HostCpus.Get(); ok {
		fields = append(fields, deps.Float64("host_cpus", hostCpus))
	}

	// capacity_cores is what the table would be built against: the quota when
	// it names a positive limit, else the cpuset's count. Zero means neither
	// read answered, so it is omitted rather than reported as a measured zero.
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
