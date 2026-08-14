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

// Sample, the readings one tick produces, and the capabilities derived from
// them.

package cpuhealth

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// Scope reports whether a sampler's logical CPU count describes every CPU on
// the machine or only the ones its container may run on.
type Scope int

const (
	// ScopeUnknown means the machine's CPU count could not be read, so the
	// scope cannot be known. It is never a silent ScopeHost on a failing read.
	ScopeUnknown Scope = iota
	// ScopeHost means the container's allowed set covers every machine CPU.
	ScopeHost
	// ScopeAffinity means the container is pinned to a strict subset of the
	// machine's CPUs, so the logical count describes only those.
	ScopeAffinity
)

// Sample holds the CPU health readings for one cgroup.
//
// Flat by decision. A type per concept would pair a Reading with its own
// presence bool — the shape cadvisor deprecated as DeprecatedContainerStats,
// Has=false being indistinguishable from a measured zero — which Reading.Get
// prevents. It splits reads too: one cpu.stat open yields usage and both
// throttle counters; CpuScope needs /proc/stat and the cpuset. And all fields
// share one Timestamp a split would fork — prometheus/procfs keeps Stat flat.
type Sample struct {
	// Quota is present when cpu.max names a positive limit (the capacity in
	// cores), the literal "max" (a present 0.0, a definite no-limit), or a
	// non-positive numeric limit (a present 0.0, never a positive capacity).
	// It is absent no-signal when cpu.max is unreadable or unparsable.
	Quota diagnosis.Reading

	// Pressure is present when cpu.pressure's "some" line yields a readable
	// avg60 this tick (the kernel's 0..100 figure divided by 100 into the 0..1
	// fraction the marks are denominated in). It is absent when that read fails
	// this tick.
	Pressure diagnosis.Reading

	// PsiAvailable is sticky: it is set true on the first successful
	// cpu.pressure read and never cleared, even when a later read fails.
	PsiAvailable bool

	// NrPeriods and NrThrottled come from the SAME cpu.stat read that carries
	// usage. Each is present when its key is in cpu.stat and parses, and
	// unavailable (never a trusted 0) when the key is absent or unparsable.
	NrPeriods   diagnosis.Reading
	NrThrottled diagnosis.Reading

	// UsageUsec is the raw cumulative usage_usec counter from cpu.stat. It is
	// present when the key is in cpu.stat and parses, and unavailable when it
	// is absent or unparsable. The raw total is kept beside the rate so a later
	// throttle-ratio reduction still has the totals.
	UsageUsec diagnosis.Reading

	// UsageCores is the instantaneous usage rate in cores: the delta of
	// cumulative usage_usec across two consecutive reads divided by 1e6 (the
	// microsecond divisor) and by the elapsed seconds between the reads'
	// Timestamps. It is Unknown on the first read (no previous edge to subtract
	// from) and when usage_usec falls (a cumulative counter that falls has been
	// reset) — never a confident zero from no measurement.
	UsageCores diagnosis.Reading

	// HostBusy is the host's busy cores as an instantaneous rate: the delta of
	// the first aggregate "cpu " line's busy jiffies (user+nice+sys+irq+softirq)
	// between this read and the previous one, divided by USER_HZ into seconds
	// and by the interval's elapsed seconds. It is absent on the first read
	// (that read only fixes a baseline), when /proc/stat is unreadable or
	// unparsable, and when the busy counter falls (a host restart has reset it).
	HostBusy diagnosis.Reading

	// Steal is the host's CPU-steal fraction over the last poll interval: the
	// interval's steal-jiffy delta over the interval's total-jiffy (fields 0..7)
	// delta, read off the same first aggregate "cpu " line as HostBusy. It is
	// absent on the baseline read, when that line is unavailable like HostBusy,
	// when the steal counter falls (a reset), and when the denominator did not
	// advance (nothing measured, not a NaN/Inf reading).
	Steal diagnosis.Reading

	// HostCpus is the machine's CPU count: the number of per-CPU (cpu0, cpu1, …)
	// lines in /proc/stat. It is present when that file is readable, and unknown
	// when it is not.
	HostCpus diagnosis.Reading

	// LogicalCpus is the CPUs this process may use: the size of the container's
	// allowed cpuset, which under --cpuset-cpus is a strict subset of the
	// machine's count. It is the "2" in "pinned to 2 of 8 CPUs". It is present
	// when the cpuset is readable, and unknown when it is not.
	LogicalCpus diagnosis.Reading

	// CpuScope reports whether the sampler's logical CPU count describes every
	// CPU on the machine (ScopeHost), only the ones its container may run on
	// (ScopeAffinity), or ScopeUnknown when the machine's CPU count cannot be
	// read. It is never ScopeHost on an unreadable count.
	CpuScope Scope

	// Virtualized reports whether this host is a virtual machine. It is resolved
	// once and cached across ticks, not re-read every tick. On x86 the evidence
	// is the "hypervisor" flag in /proc/cpuinfo's flags line; an ARM64 cpuinfo
	// has no flags line at all, so the distinct ARM64 route is the DMI fallback
	// of /sys/class/dmi/id/product_name naming a known hypervisor. An unreadable
	// cpuinfo is no evidence and reads false.
	Virtualized bool

	// Timestamp is the time of this read. Every field off the same read carries
	// the same Timestamp.
	Timestamp time.Time
}

// Sampler reads a cgroup's CPU health signals.
type Sampler interface {
	Read(ctx context.Context) (Sample, error)
}

// DeriveEnvironment reads three facts off Sample — whether the host is
// Virtualized, whether Quota names a POSITIVE number, and whether the kernel
// has ever reported PSI — and builds the Environment the engine selects
// instruments with.
func DeriveEnvironment(s Sample) diagnosis.Environment {
	caps := make([]diagnosis.Capability, 0, 3)
	if s.Virtualized {
		caps = append(caps, HasVirtualization)
	}
	if q, ok := s.Quota.Get(); ok && q > 0 {
		caps = append(caps, HasLimit)
	}
	if s.PsiAvailable {
		caps = append(caps, HasPressureStats)
	}
	return diagnosis.NewEnvironment(caps...)
}
