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
// previous tick's numbers wherever a rate is derived. Read combines a
// cgroupReader (cgroup_v2_source.go for v2, cgroup_v1_source.go for v1) with
// hostSource (host_source.go).

package cpuhealth

import (
	"context"
	"fmt"

	"github.com/benbjohnson/clock"

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
	return NewLinuxSamplerWithClock(fs, base, clock.New())
}

// NewLinuxSamplerWithClock returns a Sampler reading via fs from base that
// stamps every Sample from clk.
func NewLinuxSamplerWithClock(fs filesystem.Service, base string, clk clock.Clock) Sampler {
	return &linuxSampler{
		fs:     fs,
		base:   base,
		cgroup: newCgroupV2Source(fs, base),
		host:   newHostSource(fs),
		clock:  clk,
	}
}

// linuxSampler combines a cgroupReader and hostSource into one Sample per tick.
type linuxSampler struct {
	fs   filesystem.Service
	base string

	clock clock.Clock

	// cgroup is the v2 reader until reader detects v1 and replaces it, so it is never nil.
	cgroup  cgroupReader
	version cgroupVersion

	host *hostSource

	// psiAvailable is sticky: set true on the first successful cpu.pressure
	// read and never cleared, even when a later read fails. It is held here,
	// not on the cgroup reader, so it keeps its value when reader replaces the
	// v2 reader with a v1 reader.
	psiAvailable bool
}

// Read samples the cgroup and the machine once.
//
// A non-nil error means this tick has no measurement: cpu.stat or v1's
// cpuacct.usage opened and did not parse, or the tick was cancelled. A file
// that does not open is not an error: its readings stay absent and the rest of
// the sample is read. Sample.Troubleshooting.Reads records what every read
// produced, so diagnose a failed read from there, not from the error.
func (s *linuxSampler) Read(ctx context.Context) (Sample, error) {
	var sample Sample
	sample.Troubleshooting.CgroupBase = s.base
	sample.Troubleshooting.Reads = seedReads()

	cgroup := s.reader(ctx)
	sample.Troubleshooting.CgroupVersion = s.version.String()
	sample.Troubleshooting.ReadPaths = readPaths(cgroup)

	// First because a cpu.stat failure returns before every read below it, and
	// a report of that failure needs these reads as much as any other.
	s.recordRawReads(ctx, &sample)

	// Stamped once, here, and passed to both sources: neither cgroup nor host
	// reads a clock itself, so both rate derivations divide by the same
	// elapsed time and Decide never compares a machine-wide mean against a
	// cgroup mean taken from a different instant.
	timestamp := s.clock.Now()
	sample.Timestamp = timestamp

	// cpu.pressure: PSI presence is sticky once seen; this tick's read success
	// is Pressure's own Reading, absent when the read fails this tick.
	fraction, psiErr := cgroup.readPSI(ctx)
	if psiErr != nil {
		sample.Pressure = diagnosis.Unknown()
	} else {
		s.psiAvailable = true
		sample.Pressure = diagnosis.Known(fraction)
	}
	sample.record(OperationCPUPressure, classifyRead(psiErr), psiErr)
	sample.PsiAvailable = s.psiAvailable

	stat := cgroup.readStat(ctx)
	// Assigned before the early return below: this text is what would not parse.
	sample.Troubleshooting.CPUStatRaw = stat.Raw
	for _, read := range stat.Reads {
		sample.record(read.Operation, read.Outcome, read.Err)
	}
	// A usage or throttle file that opens and does not parse is corrupt, so the
	// tick returns an error instead of numbers derived from it. A file that does
	// not open only leaves its readings absent, because a host may not have
	// that file at all.
	if corrupt, found := firstUnparsable(stat.Reads); found {
		return sample, fmt.Errorf("parse %s: %w", corrupt.Operation, sample.Troubleshooting.ReadErrors[corrupt.Operation])
	}
	// Check whether the reading was cancelled, and if so return the cancellation
	// error. A cancelled read fails every file, which looks the same as a host
	// that has none of them, and the sample would report the second.
	if cancelErr := ctx.Err(); cancelErr != nil {
		return sample, cancelErr
	}

	sample.NrPeriods = stat.Periods
	sample.NrThrottled = stat.Throttled
	sample.UsageUsec = stat.Usage
	sample.UsageCores = cgroup.advanceUsageRate(timestamp, stat.Usage)

	// Host signals: the first /proc/stat read fixes a baseline and publishes
	// neither; a read after that publishes this tick's instantaneous host-busy
	// rate and per-interval steal fraction, each derived from the delta of two
	// consecutive reads. A falling cumulative counter (a host restart) is a
	// reset: the baseline is re-established and nothing is published this tick.
	// The same read carries the machine's CPU count, from which the snapshots'
	// CPU scope is derived.
	busy, steal, denominator, machine, hostErr := s.host.readHost(ctx)
	sample.record(OperationProcStat, classifyRead(hostErr), hostErr)
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
		s.recordCPUScope(ctx, cgroup, &sample, machine)
		sample.HostBusy, sample.Steal = s.host.advanceHostRates(timestamp, busy, steal, denominator)
	}

	virtualized, cpuinfoOutcome, cpuinfoErr := s.host.readVirtualized(ctx)
	sample.Virtualized = virtualized
	sample.record(OperationProcCpuinfo, cpuinfoOutcome, cpuinfoErr)

	quota, cpuMaxOutcome, cpuMaxErr := cgroup.readQuota(ctx)
	sample.Quota = quota.Limit
	sample.Troubleshooting.CPUMaxRaw = quota.Raw
	sample.record(OperationCPUMax, cpuMaxOutcome, cpuMaxErr)

	if cancelErr := ctx.Err(); cancelErr != nil {
		return sample, cancelErr
	}

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
func (s *linuxSampler) recordCPUScope(ctx context.Context, cgroup cgroupReader, sample *Sample, machine float64) {
	allowed, cpusetErr := cgroup.readCpuset(ctx)
	sample.record(OperationCpusetCPUs, classifyRead(cpusetErr), cpusetErr)
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

// reader returns the cgroup reader for this tick. It runs version detection on
// every tick until a version is found, because the container can start before
// its cgroup is mounted.
func (s *linuxSampler) reader(ctx context.Context) cgroupReader {
	if s.version != cgroupVersionUnresolved {
		return s.cgroup
	}

	version := detectCgroupVersion(ctx, s.fs, s.base)
	if version == cgroupV1 {
		s.cgroup = newCgroupV1Source(s.fs, s.base, locateV1Files(ctx, s.fs, s.base))
	}
	s.version = version

	return s.cgroup
}

// recordRawReads puts the reads that produce no signal on sample: file text kept
// verbatim, and the base directory kept as an entry count. They describe the
// machine on a failure report, and nothing here judges them.
func (s *linuxSampler) recordRawReads(ctx context.Context, sample *Sample) {
	controllers, controllersOutcome, controllersErr := readControllers(ctx, s.fs, s.base)
	sample.Troubleshooting.CgroupControllersRaw = controllers
	sample.record(OperationCgroupControllers, controllersOutcome, controllersErr)

	procSelf, procSelfOutcome, procSelfErr := s.host.readProcSelfCgroup(ctx)
	sample.Troubleshooting.ProcSelfCgroupRaw = procSelf
	sample.record(OperationProcSelfCgroup, procSelfOutcome, procSelfErr)

	baseEntries, baseDirOutcome, baseDirErr := readBaseDirEntryCount(ctx, s.fs, s.base)
	sample.Troubleshooting.CgroupBaseDirEntryCount = baseEntries
	sample.record(OperationCgroupBaseDir, baseDirOutcome, baseDirErr)
}

func firstUnparsable(reads []readAttempt) (readAttempt, bool) {
	for _, read := range reads {
		if read.Outcome == ReadUnparsable {
			return read, true
		}
	}

	return readAttempt{}, false
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

// record overwrites the operation's seeded entry and keeps readErr for the
// report. An operation absent from allReadOperations has no entry to overwrite
// and records nothing.
func (s *Sample) record(operation ReadOperation, outcome ReadOutcome, readErr error) {
	if readErr != nil {
		if s.Troubleshooting.ReadErrors == nil {
			s.Troubleshooting.ReadErrors = make(map[ReadOperation]error, len(allReadOperations))
		}

		// Named here rather than at each failure: the seventeen places that
		// return a content failure sit inside parse helpers that never learn
		// which file they were handed, while this one knows both.
		s.Troubleshooting.ReadErrors[operation] = pathErrorFor(s.Troubleshooting.ReadPaths[operation], readErr)
	}

	for i := range s.Troubleshooting.Reads {
		if s.Troubleshooting.Reads[i].Operation == operation {
			s.Troubleshooting.Reads[i].Outcome = outcome
			return
		}
	}
}

// readControllers returns cgroup.controllers verbatim: the controllers the
// parent delegated to the cgroup at base. Any outcome other than ReadOK means
// no text was read, and names the cause.
func readControllers(ctx context.Context, fs filesystem.Service, base string) (string, ReadOutcome, error) {
	return readRawFile(ctx, fs, pathOf(base, OperationCgroupControllers))
}

// readBaseDirEntryCount returns the number of entries in base, or -1 when the
// directory cannot be listed, never 0. A mounted cgroup v2 tree holds dozens of
// files, so a count of two or three shows the mount is not the expected one.
func readBaseDirEntryCount(ctx context.Context, fs filesystem.Service, base string) (int, ReadOutcome, error) {
	entries, err := fs.ReadDir(ctx, pathOf(base, OperationCgroupBaseDir))
	if err != nil {
		return -1, classifyRead(err), err
	}

	return len(entries), ReadOK, nil
}
