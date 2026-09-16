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

// What this worker reports to Sentry when a read fails: which reads earn an
// event, under which verb, and what each event carries.

package fsmv2cpu

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

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
	{Op: cpuhealth.OpCPUPressure, Outcome: cpuhealth.ReadMissing}: {},
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

// readFailure is one read whose outcome earns a Sentry event.
type readFailure struct {
	Op      cpuhealth.ReadOp
	Outcome cpuhealth.ReadOutcome
	Verb    string
}

// failedReads returns the reads on smp that earn a Sentry event, in the order
// Read performed them. Whether an event was already sent for one is not asked
// here; that rule belongs to reportFailedReads.
//
// ReadNotAttempted earns nothing: one failure stops several later reads, so
// reporting those would turn one root cause into several issues.
func failedReads(smp cpuhealth.Sample) []readFailure {
	var failures []readFailure

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

		failures = append(failures, readFailure{Op: r.Op, Outcome: r.Outcome, Verb: verbFor(r)})
	}

	return failures
}

// verbFor says what a failed read cost. Only cpu.stat carries the usage
// counters, so only a cpu.stat that could not be read or parsed leaves the
// tick with no measurement. A cpu.stat that read fine and held no usage figure
// still yields a usable sample, and a cpu.pressure failing in the same tick
// cost one signal, so both stay read_failed.
func verbFor(r cpuhealth.ReadResult) string {
	if r.Op == cpuhealth.OpCPUStat && r.Outcome != cpuhealth.ReadEmpty {
		return sampleFailedPrefix
	}

	return readFailedPrefix
}

// reportFailedReads emits one Sentry event per failed read on smp. A sample
// whose reads all succeeded yields no failures and no events, which is why
// Poll calls this on every tick rather than only when Read returns an error.
//
// A failure that repeats every tick reports once: the first event names the
// problem and the rest would cost an issue each while adding nothing.
func (d *CPUDeps) reportFailedReads(ctx context.Context, smp cpuhealth.Sample) {
	// Shutdown is not a failure. filesystem.DefaultService.ReadFile checks the
	// context, so once it is done every read fails and a graceful shutdown would
	// emit an event per read on every instance.
	if ctx.Err() != nil {
		return
	}

	cores, quota := limitsFromSample(smp)

	for _, f := range failedReads(smp) {
		seen := cpuhealth.ReadResult{Op: f.Op, Outcome: f.Outcome}
		if _, reportedBefore := d.reportedReads.LoadOrStore(seen, struct{}{}); reportedBefore {
			continue
		}

		d.GetLogger().SentryWarn(deps.FeatureSupportCPU, d.GetHierarchyPath(),
			f.Verb+string(f.Op)+readFailedSep+string(f.Outcome),
			readFailureFields(smp, f.Op, cores, quota)...)
	}
}

// readFailureFields is what one failed-read event carries besides its message.
func readFailureFields(smp cpuhealth.Sample, failed cpuhealth.ReadOp, cores, quota float64) []deps.Field {
	fields := []deps.Field{
		deps.String("path", cpuhealth.PathOf(smp.Base, failed)),
		deps.String("cgroup_base", smp.Base),
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
