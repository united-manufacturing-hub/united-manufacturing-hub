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

// The Linux sampler: one tick's read of the cgroup and the machine, the
// capability constants that read establishes, and the rate arithmetic that
// needs the previous tick to exist.

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
)

// NewLinuxSampler returns a Sampler reading via fs from base.
func NewLinuxSampler(fs filesystem.Service, base string) Sampler {
	return &linuxSampler{fs: fs, base: base}
}

// Read samples the cgroup at base from cpu.max, the container's CPU limit: a
// positive limit reads as a capacity, "max" and non-positive limits as a
// present no-limit, and an unreadable or unparsable cpu.max as absent
// no-signal. cpu.max and cpu.stat are the cgroup v2 CPU controller's files,
// documented at
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html.
func (s *linuxSampler) Read(ctx context.Context) (Sample, error) {
	var smp Sample
	smp.Timestamp = time.Now()

	// cpu.pressure: PSI presence is sticky once seen; this tick's read success
	// is Pressure's own Reading, absent when the read fails this tick.
	if frac, ok := s.readPSI(ctx); ok {
		s.psiAvailable = true
		smp.Pressure = diagnosis.Known(frac)
	} else {
		smp.Pressure = diagnosis.Unknown()
	}
	smp.PsiAvailable = s.psiAvailable

	usage, periods, throttled, statErr := s.readStat(ctx)
	if statErr != nil {
		// cpu.stat is primary: a read failure there fails the WHOLE sample,
		// never a silent drop of the throttle counters as absent no-signal.
		return smp, fmt.Errorf("read %s/cpu.stat: %w", s.base, statErr)
	}
	smp.NrPeriods = periods
	smp.NrThrottled = throttled
	smp.UsageUsec = usage
	if s.usageBase.have {
		// A rising cumulative counter over a positive elapsed time derives an
		// instantaneous rate; a falling one has been reset, so no rate.
		if u, ok := usage.Get(); ok && u >= s.usageBase.usage {
			if elapsed := smp.Timestamp.Sub(s.usageBase.time).Seconds(); elapsed > 0 {
				smp.UsageCores = diagnosis.Known((u - s.usageBase.usage) / 1e6 / elapsed)
			}
		}
	}
	if u, ok := usage.Get(); ok {
		s.usageBase = usageBaseline{usage: u, time: smp.Timestamp, have: true}
	}

	// Host signals: the first /proc/stat read fixes a baseline and publishes
	// neither; a read after that publishes this tick's instantaneous host-busy
	// rate and per-interval steal fraction, each derived from the delta of two
	// consecutive reads. A falling cumulative counter (a host restart) is a
	// reset: the baseline is re-established and nothing is published this tick.
	// The same read carries the machine's CPU count, from which the snapshots'
	// CPU scope is derived.
	if busy, steal, denom, machine, ok := s.readHost(ctx); ok {
		// CPU scope: the machine's count is kept on the snapshot, and the scope
		// compares the container's allowed cpuset against it. A readable machine
		// count whose allowed set covers it reads ScopeHost; a pinned subset
		// reads ScopeAffinity; a failed cpuset read on a known machine count is
		// likewise unknown (never a silent host).
		smp.HostCpus = diagnosis.Known(machine)
		// The cpuset read carries the logical CPU count this process may use —
		// the "2" in "pinned to 2 of 8 CPUs" — beside the scope. A failed cpuset
		// read leaves the fresh
		// sample's CpuScope as its zero value, ScopeUnknown, and LogicalCpus
		// absent: never a silent ScopeHost on a known machine count.
		if allowed, aok := s.readCpuset(ctx); aok {
			smp.LogicalCpus = diagnosis.Known(float64(allowed))
			if allowed == int(machine) {
				smp.CpuScope = ScopeHost
			} else {
				smp.CpuScope = ScopeAffinity
			}
		}
		if s.hostBase.have {
			// Busy cores over the interval is the busy-jiffy delta divided by
			// USER_HZ into seconds; the interval's elapsed time turns that into
			// a per-second rate. A falling busy counter (a reset) and a zero
			// elapsed time each publish nothing.
			if busy >= s.hostBase.busy {
				if elapsed := smp.Timestamp.Sub(s.hostBase.time).Seconds(); elapsed > 0 {
					smp.HostBusy = diagnosis.Known((busy - s.hostBase.busy) / userHz / elapsed)
				}
			}
			// The steal fraction is the interval's steal-jiffy delta over the
			// interval's total-jiffy delta. A falling steal counter (a reset)
			// and a non-positive denominator delta (proc/stat did not advance,
			// or is all zeros) publish nothing, never a NaN/Inf reading.
			dDenom := denom - s.hostBase.denom
			if steal >= s.hostBase.steal && dDenom > 0 {
				frac := (steal - s.hostBase.steal) / dDenom
				smp.Steal = diagnosis.Known(frac)
			}
		}
		s.hostBase = hostBaseline{busy: busy, steal: steal, denom: denom, time: smp.Timestamp, have: true}
	} else {
		// An unreadable machine CPU count reads ScopeUnknown — never a silent
		// ScopeHost, since a pinned idle container misread as host would have
		// its host headroom computed by subtracting a host-scoped busy figure
		// from an affinity-scoped count, the invalid subtraction the scope
		// exists to prevent. HostCpus stays absent (its zero value) here, as the
		// machine's count could not be read to populate it.
		smp.CpuScope = ScopeUnknown
	}

	smp.Virtualized = s.readVirtualized(ctx)

	smp.Quota = s.readQuota(ctx)
	return smp, nil
}

// usageBaseline is the previous tick's usage_usec edge, from which Read derives
// the instantaneous usage rate. have is false before the first successful read;
// a falling edge (a counter reset) re-baselines instead of publishing a nonsense
// rate.
type usageBaseline struct {
	usage float64
	time  time.Time
	have  bool
}

// hostBaseline is the previous tick's /proc/stat edges (busy, steal, denominator
// jiffy totals) and its read timestamp, from which Read derives the instantaneous
// host-busy rate and per-interval steal fraction. have is false until a first
// successful read fixes the baseline; a falling counter (a host restart)
// re-baselines instead of publishing a nonsense value.
type hostBaseline struct {
	busy, steal, denom float64
	time               time.Time
	have               bool
}

type linuxSampler struct {
	fs           filesystem.Service
	base         string
	psiAvailable bool

	usageBase usageBaseline
	hostBase  hostBaseline

	// virtualized is the sticky virtualisation fact, resolved on the first
	// successful /proc/cpuinfo read and re-published without re-reading. An
	// unreadable cpuinfo resolves false but is not cached, so a later readable
	// one is still considered.
	virtualized  bool
	virtResolved bool
}
