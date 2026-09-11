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

// The Linux sampler: one tick's cgroup-plus-machine read, built from the
// previous tick's numbers wherever a rate is derived. Read is a composer over
// two sources — cgroupSource (cgroup_source.go) and hostSource
// (host_source.go) — neither of which can see the other's files.

package cpuhealth

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Environment capabilities. They are startup facts about this host, distinct
// from per-tick readability, and are folded into the Environment the engine
// selects instruments with.
const (
	// HasVirtualization means the host is a virtual machine.
	HasVirtualization diagnosis.Capability = "cpuhealth.HasVirtualization"
	// HasLimit means the cgroup names a positive CPU quota — see Sample.Quota
	// for what sets it (docker run --cpus, a Kubernetes CPU limit, a Compose
	// cpus: entry).
	HasLimit diagnosis.Capability = "cpuhealth.HasLimit"
	// HasPressureStats means the kernel ever published PSI (the sticky
	// PsiAvailable). The pressure instrument Requires it, so selection resolves
	// a host whose kernel never reported PSI to NoInstrument, never AllAbsent.
	HasPressureStats diagnosis.Capability = "cpuhealth.HasPressureStats"
	// HasLimitedVisibility means neither HasLimit nor HasPressureStats holds:
	// no quota to judge our own budget against, and no PSI to read the harm
	// off. It is the same condition Details.LimitedVisibility reports, named
	// the same, and it is the gate on the usage-fraction instrument — see
	// hostCpuFullSignal for why that arm needs it.
	HasLimitedVisibility diagnosis.Capability = "cpuhealth.HasLimitedVisibility"
)

// NewLinuxSampler returns a Sampler reading via fs from base.
func NewLinuxSampler(fs filesystem.Service, base string) Sampler {
	return &linuxSampler{
		cgroup: newCgroupSource(fs, base),
		host:   newHostSource(fs),
	}
}

// linuxSampler composes cgroupSource and hostSource into one Sample per tick.
// It holds no accounting state of its own — every sticky fact and baseline
// belongs to whichever source reads the file it is derived from — because it
// exists only to stamp the tick's single Timestamp and derive CPU scope, the
// one fact that needs both sources' reads to compute.
type linuxSampler struct {
	cgroup *cgroupSource
	host   *hostSource
}

// Read samples the cgroup and the machine once.
//
// A non-nil error means cpu.stat opened and would not parse, and this tick has
// no measurement. A cpu.stat that will not open at all is not an error: the
// three readings taken from it stay absent and the rest of the sample reads. Sample.Troubleshooting.Reads still records what every read
// produced, so diagnose a failed read from there, not from the error.
func (s *linuxSampler) Read(ctx context.Context) (Sample, error) {
	var sample Sample
	sample.Troubleshooting.CgroupBase = s.cgroup.base
	sample.Troubleshooting.Reads = seedReads()

	// First because a cpu.stat failure returns before every read below it, and
	// a report of that failure needs these reads as much as any other.
	s.recordRawReads(ctx, &sample)

	// Stamped once, here, and passed to both sources: neither cgroup nor host
	// calls time.Now() itself, so both rate derivations divide by the same
	// elapsed time and Decide never compares a machine-wide mean against a
	// cgroup mean taken from a different instant.
	timestamp := time.Now()
	sample.Timestamp = timestamp

	// cpu.pressure: PSI presence is sticky once seen; this tick's read success
	// is Pressure's own Reading, absent when the read fails this tick.
	fraction, psiErr := s.cgroup.readPSI(ctx)
	if psiErr != nil {
		sample.Pressure = diagnosis.Unknown()
	} else {
		s.cgroup.psiAvailable = true
		sample.Pressure = diagnosis.Known(fraction)
	}
	sample.record(OperationCPUPressure, classifyRead(psiErr))
	sample.PsiAvailable = s.cgroup.psiAvailable

	stat, statErr := s.cgroup.readStat(ctx)
	// Assigned before the early return below: this text is what would not parse.
	sample.Troubleshooting.CPUStatRaw = stat.Raw
	statReadOutcome := statOutcome(stat, statErr)
	sample.record(OperationCPUStat, statReadOutcome)
	if statReadOutcome == ReadUnparsable {
		// A cpu.stat that opens and does not parse is corrupt, and every number
		// derived from it would be a guess. A cpu.stat that will not open is a
		// different thing: the three readings below stay absent and the sample
		// carries on, so a host keeping its CPU accounting elsewhere is not
		// degraded over a file it was never going to have.
		return sample, fmt.Errorf("parse %s/cpu.stat: %w", s.cgroup.base, statErr)
	}
	// A cancelled tick fails every read, which is the same shape as a host with
	// none of these files. Without this the sample reports the second.
	if cancelErr := ctx.Err(); cancelErr != nil {
		return sample, cancelErr
	}

	sample.NrPeriods = stat.Periods
	sample.NrThrottled = stat.Throttled
	sample.UsageUsec = stat.Usage
	sample.UsageCores = s.cgroup.advanceUsageRate(timestamp, stat.Usage)

	// Host signals: the first /proc/stat read fixes a baseline and publishes
	// neither; a read after that publishes this tick's instantaneous host-busy
	// rate and per-interval steal fraction, each derived from the delta of two
	// consecutive reads. A falling cumulative counter (a host restart) is a
	// reset: the baseline is re-established and nothing is published this tick.
	// The same read carries the machine's CPU count, from which the snapshots'
	// CPU scope is derived.
	busy, steal, denominator, machine, hostErr := s.host.readHost(ctx)
	sample.record(OperationProcStat, classifyRead(hostErr))
	if hostErr != nil {
		// An unreadable machine CPU count reads ScopeUnknown — never a silent
		// ScopeHost, since a pinned idle container misread as host would have
		// its host headroom computed by subtracting a host-scoped busy figure
		// from an affinity-scoped count, the invalid subtraction the scope
		// exists to prevent. HostCpus stays absent even where readHost did count
		// the per-CPU lines: a sample whose scope could not be established must
		// not publish a machine count.
		sample.CpuScope = ScopeUnknown
	} else {
		sample.HostCpus = diagnosis.Known(machine)
		// Nested under a successful /proc/stat read so the cpuset stays
		// not_attempted when /proc/stat failed: the file was never opened, and
		// recording a failure for it would name the wrong one.
		s.recordCPUScope(ctx, &sample, machine)
		sample.HostBusy, sample.Steal = s.host.advanceHostRates(timestamp, busy, steal, denominator)
	}

	virtualized, cpuinfoOutcome := s.host.readVirtualized(ctx)
	sample.Virtualized = virtualized
	sample.record(OperationProcCpuinfo, cpuinfoOutcome)

	quota, cpuMaxOutcome := s.cgroup.readQuota(ctx)
	sample.Quota = quota.Limit
	sample.Troubleshooting.CPUMaxRaw = quota.Raw
	sample.record(OperationCPUMax, cpuMaxOutcome)

	return sample, nil
}

// recordCPUScope says whether this container may use the whole machine or a
// pinned subset of it. Host headroom subtracts a machine-wide busy figure from
// a CPU count, and that subtraction is only valid when both are on the same
// scale, so a container pinned to 2 of 8 CPUs must not have its headroom
// computed against 2.
//
// A cpuset covering the machine reads ScopeHost and a pinned subset reads
// ScopeAffinity. The same read carries LogicalCpus, the "2" in "pinned to 2 of
// 8 CPUs". A failed cpuset read reads ScopeUnknown with LogicalCpus absent,
// never a silent ScopeHost on a known machine count.
func (s *linuxSampler) recordCPUScope(ctx context.Context, sample *Sample, machine float64) {
	allowed, cpusetErr := s.cgroup.readCpuset(ctx)
	sample.record(OperationCpusetCPUs, classifyRead(cpusetErr))
	if cpusetErr != nil {
		sample.LogicalCpus = diagnosis.Unknown()
		sample.CpuScope = ScopeUnknown

		return
	}

	sample.LogicalCpus = diagnosis.Known(float64(allowed))
	if allowed == int(machine) {
		sample.CpuScope = ScopeHost

		return
	}

	sample.CpuScope = ScopeAffinity
}

// recordRawReads puts the reads that produce no signal on sample: file text kept
// verbatim, and the base directory kept as an entry count. They describe the
// machine on a failure report, and nothing here judges them.
func (s *linuxSampler) recordRawReads(ctx context.Context, sample *Sample) {
	controllers, controllersOutcome := s.cgroup.readControllers(ctx)
	sample.Troubleshooting.CgroupControllersRaw = controllers
	sample.record(OperationCgroupControllers, controllersOutcome)

	procSelf, procSelfOutcome := s.host.readProcSelfCgroup(ctx)
	sample.Troubleshooting.ProcSelfCgroupRaw = procSelf
	sample.record(OperationProcSelfCgroup, procSelfOutcome)

	baseEntries, baseDirOutcome := s.cgroup.readBaseDirEntryCount(ctx)
	sample.Troubleshooting.CgroupBaseDirEntryCount = baseEntries
	sample.record(OperationCgroupBaseDir, baseDirOutcome)
}

// statOutcome reports a successful read with no usage figure as ReadEmpty,
// since ReadOK would claim a value never produced. A zero-byte file and a
// valueless usage_usec line both land there; the raw text separates them.
func statOutcome(stat statRead, err error) ReadOutcome {
	if err != nil {
		return classifyRead(err)
	}

	if _, ok := stat.Usage.Get(); !ok {
		return ReadEmpty
	}

	return ReadOK
}

// seedReads returns one ReadNotAttempted entry per operation, in
// allReadOperations order.
func seedReads() []ReadResult {
	reads := make([]ReadResult, len(allReadOperations))
	for i, spec := range allReadOperations {
		reads[i] = ReadResult{Operation: spec.Operation, Outcome: ReadNotAttempted}
	}
	return reads
}

// record overwrites the operation's seeded entry. An operation absent from
// allReadOperations has no entry to overwrite and records nothing.
func (s *Sample) record(operation ReadOperation, outcome ReadOutcome) {
	for i := range s.Troubleshooting.Reads {
		if s.Troubleshooting.Reads[i].Operation == operation {
			s.Troubleshooting.Reads[i].Outcome = outcome
			return
		}
	}
}
