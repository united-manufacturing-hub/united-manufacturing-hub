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

// The host source: everything this package reads machine-wide, independent
// of any particular cgroup — /proc/stat, /proc/cpuinfo, and the DMI identity
// files. Distinct from cgroupSource, which reads one cgroup's own accounting
// files under its base.

package cpuhealth

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// userHz matches the kernel's USER_HZ: the tick rate dividing /proc/stat jiffies
// into seconds, hence cores. It is hardcoded rather than queried because the
// Linux userspace ABI fixes USER_HZ at 100 regardless of the kernel's internal
// CONFIG_HZ, and the shipped binary builds with CGO_ENABLED=0 (see the
// Dockerfile), so sysconf(_SC_CLK_TCK) is not reachable to ask instead.
const userHz = 100.0

// hostSource reads the machine-wide files: /proc/stat, /proc/cpuinfo,
// /sys/class/dmi/id/product_name and /sys/class/dmi/id/sys_vendor. It owns
// the two facts that persist across ticks for this host — the
// host-busy/steal baseline and the sticky virtualisation fact — so it is
// constructible and testable independently of cgroupSource.
type hostSource struct {
	fs filesystem.Service

	hostBase hostBaseline

	// virtualized is the sticky virtualisation fact, resolved on the first
	// successful /proc/cpuinfo read and re-published without re-reading. An
	// unreadable cpuinfo resolves false but is not cached, so a later readable
	// one is still considered.
	virtualized  bool
	virtResolved bool
}

// newHostSource returns a hostSource reading via fs.
func newHostSource(fs filesystem.Service) *hostSource {
	return &hostSource{fs: fs}
}

// hostBaseline is the previous tick's /proc/stat edges (busy, steal, denominator
// jiffy totals) and its read timestamp, from which hostRates derives the
// instantaneous host-busy rate and per-interval steal fraction. have is false
// until a first successful read fixes the baseline; a falling counter (a host
// restart) re-baselines instead of publishing a nonsense value.
type hostBaseline struct {
	busy, steal, denom float64
	time               time.Time
	have               bool
}

// hostRates derives this tick's HostBusy rate and Steal fraction from busy,
// steal and denom against the baseline this source owns, then updates the
// baseline for the next tick. ts is the composer's single per-tick Timestamp,
// passed in rather than read via time.Now(): this rate and the cgroup
// source's usage rate must be measured against the same instant, or Decide's
// attribution would compare a machine-wide mean against a cgroup mean taken
// at a different moment.
func (h *hostSource) hostRates(ts time.Time, busy, steal, denom float64) (hostBusy, stealFrac diagnosis.Reading) {
	hostBusy = diagnosis.Unknown()
	stealFrac = diagnosis.Unknown()
	if h.hostBase.have {
		// HostBusy: busy-jiffy delta ÷ USER_HZ ÷ elapsed seconds; skipped on
		// a counter reset or zero elapsed time.
		if busy >= h.hostBase.busy {
			if elapsed := ts.Sub(h.hostBase.time).Seconds(); elapsed > 0 {
				hostBusy = diagnosis.Known((busy - h.hostBase.busy) / userHz / elapsed)
			}
		}
		// The steal fraction is the interval's steal-jiffy delta over the
		// interval's total-jiffy delta. A falling steal counter (a reset)
		// and a non-positive denominator delta (proc/stat did not advance,
		// or is all zeros) publish nothing, never a NaN/Inf reading.
		dDenom := denom - h.hostBase.denom
		if steal >= h.hostBase.steal && dDenom > 0 {
			stealFrac = diagnosis.Known((steal - h.hostBase.steal) / dDenom)
		}
	}
	h.hostBase = hostBaseline{busy: busy, steal: steal, denom: denom, time: ts, have: true}
	return hostBusy, stealFrac
}

// readHost reads the first aggregate "cpu " line of /proc/stat and yields the
// raw busy, steal and denominator jiffy totals. busy counts user, nice, system,
// irq and softirq jiffies (idle, iowait, steal, guest and guest_nice excluded).
// The steal denominator is the sum of fields 0..7 only, since the kernel folds
// guest and guest_nice into user and nice. Both totals are kept raw so the
// caller can derive interval deltas. machine is the number of per-CPU (cpu0,
// cpu1, …) lines in the same file, the machine's CPU count. The trailing space
// in "cpu " keeps the aggregate line from matching cpu0/cpu1.
func (h *hostSource) readHost(ctx context.Context) (busy, steal, denom, machine float64, ok bool) {
	data, err := h.fs.ReadFile(ctx, "/proc/stat")
	if err != nil {
		return 0, 0, 0, 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		// A per-CPU line is "cpu" followed by a digit; the aggregate "cpu " line
		// (space, not digit) is not one of them.
		if len(line) > 3 && strings.HasPrefix(line, "cpu") && line[3] >= '0' && line[3] <= '9' {
			machine++
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line) // fields[0] == "cpu"
		if len(fields) < 9 {
			return 0, 0, 0, machine, false
		}
		vals := make([]float64, len(fields))
		for i := 1; i < len(fields); i++ {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				return 0, 0, 0, machine, false
			}
			vals[i] = v
		}
		busy := vals[1] + vals[2] + vals[3] + vals[6] + vals[7]
		denom := vals[1] + vals[2] + vals[3] + vals[4] + vals[5] + vals[6] + vals[7] + vals[8]
		return busy, vals[8], denom, machine, true
	}
	return 0, 0, 0, machine, false
}

// readVirtualized returns the sticky virtualisation fact. A "hypervisor" flag
// in /proc/cpuinfo's flags line proves an x86 guest and caches true; a positive
// token match on either DMI source — product_name, or sys_vendor for the cloud
// hypervisors product_name never names — also caches true. The fact is cached
// false only when a decisive source was read: on x86 (flags line present) a
// readable product_name is authoritative, and on any platform a readable
// product_name together with a readable sys_vendor (both naming no hypervisor)
// is a conclusive bare-metal identity. It stays open for the next tick —
// never a permanent Virtualized=false — when no DMI source was readable, or on
// a platform without a flags line when sys_vendor is still unresolved.
func (h *hostSource) readVirtualized(ctx context.Context) bool {
	if h.virtResolved {
		return h.virtualized
	}
	data, err := h.fs.ReadFile(ctx, "/proc/cpuinfo")
	if err == nil && cpuinfoHasHypervisorFlag(data) {
		h.virtualized = true
		h.virtResolved = true
		return true
	}
	// ARM64 route. A successful DMI read resolves the fact either way; a failed
	// DMI read leaves it unresolved so the next tick retries. The DMI identity
	// spans two independent sources: product_name and sys_vendor (the cloud
	// hypervisor an ARM64 product_name like "m6g.medium" never names), each read
	// separately so a failure or absence on one never breaks the other.
	pv, pok := h.dmiProductVirtualized(ctx)
	vv, vok := h.dmiVendorVirtualized(ctx)
	if (pok && pv) || (vok && vv) {
		h.virtualized = true
		h.virtResolved = true
		return true
	}
	// Neither DMI source was readable — there is no evidence of a guest or of a
	// bare-metal identity at all, so keep the fact open and let the next tick
	// re-read rather than caching Virtualized=false for the process lifetime
	// off a momentary read failure.
	if !pok && !vok {
		return false
	}
	// product_name read resolved the fact. On a platform whose /proc/cpuinfo
	// has a flags line (x86) product_name alone is authoritative and the result
	// is cached. ARM64 has no flags line and sys_vendor is part of its identity,
	// so a still-unresolved sys_vendor keeps the fact open on the next tick
	// rather than permanently caching false. An unreadable cpuinfo cannot even
	// tell the two platforms apart, so it is treated as the weaker one and also
	// waits for sys_vendor: caching false off a momentary read failure would
	// cost this host steal attribution until the process restarts.
	if (err != nil || !cpuinfoHasFlagsLine(data)) && !vok {
		return false
	}
	h.virtualized = false
	h.virtResolved = true
	return false
}

// cpuinfoHasFlagsLine reports whether /proc/cpuinfo carries a "flags" line at
// all. Its presence marks an x86 kernel, where the DMI product_name alone is an
// authoritative bare-metal identity; an ARM64 cpuinfo has a Features line and
// no flags line, and there sys_vendor is part of the DMI identity.
func cpuinfoHasFlagsLine(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, ':'); i >= 0 {
			if strings.TrimSpace(line[:i]) == "flags" {
				return true
			}
		}
	}
	return false
}

// cpuinfoHasHypervisorFlag reports whether /proc/cpuinfo's flags line carries
// the "hypervisor" flag, the x86 evidence of a guest.
func cpuinfoHasHypervisorFlag(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, ':'); i >= 0 {
			if strings.TrimSpace(line[:i]) == "flags" {
				for _, f := range strings.Fields(line[i+1:]) {
					if f == "hypervisor" {
						return true
					}
				}
			}
		}
	}
	return false
}

// dmiHypervisorTokens are the vendor substrings (lowercased) that indicate a
// hypervisor in /sys/class/dmi/id/product_name. Matched case-insensitively,
// because SMBIOS product_name casing is firmware-controlled and not reliably
// Title Case. The set matches the range reported by the major hypervisors — an
// ARM64 VM under any of them would otherwise never be marked Virtualized and
// the steal instrument's HasVirtualization capability would stay shut.
var dmiHypervisorTokens = []string{
	"vmware",
	"kvm",
	"xen",
	"hyper-v",
	"virtualbox",
	"qemu",
}

// dmiProductVirtualized is the first ARM64 DMI source. It reports whether
// /sys/class/dmi/id/product_name names a known hypervisor. The second return is
// false when the read failed: an unreadable product_name is no evidence, and
// leaving it unresolved lets the caller (and a later tick) retry rather than
// caching Virtualized=false forever.
func (h *hostSource) dmiProductVirtualized(ctx context.Context) (virtualized, resolved bool) {
	data, err := h.fs.ReadFile(ctx, "/sys/class/dmi/id/product_name")
	return dmiTokenMatch(data, err, dmiHypervisorTokens)
}

// dmiVendorHypervisorTokens are the cloud-vendor substrings (lowercased) that
// indicate a hypervisor in /sys/class/dmi/id/sys_vendor. AWS Graviton/Nitro
// puts its instance type in product_name ("m6g.medium") and the hypervisor
// identity in sys_vendor ("Amazon EC2"); GCP Tau T2A likewise, and QEMU/KVM
// guests report sys_vendor "QEMU" with a "Standard PC" product_name that the
// product tokens never catch. sys_vendor is the second DMI source, read and
// cached independently of product_name.
//
// "microsoft" is deliberately absent; see the test guarding this token list.
var dmiVendorHypervisorTokens = []string{
	"amazon",
	"google compute engine",
	"qemu",
}

// dmiVendorVirtualized is the second ARM64 DMI source. It reports whether
// /sys/class/dmi/id/sys_vendor names a cloud hypervisor, with the same
// read-failure contract as dmiProductVirtualized.
func (h *hostSource) dmiVendorVirtualized(ctx context.Context) (virtualized, resolved bool) {
	data, err := h.fs.ReadFile(ctx, "/sys/class/dmi/id/sys_vendor")
	return dmiTokenMatch(data, err, dmiVendorHypervisorTokens)
}

// dmiTokenMatch reports whether a (possibly failed) DMI source read names a
// known hypervisor token. The second return is false when the read itself
// failed, so the caller leaves the fact unresolved and retries next tick.
func dmiTokenMatch(data []byte, err error, tokens []string) (virtualized, resolved bool) {
	if err != nil {
		return false, false
	}
	lower := strings.ToLower(strings.TrimSpace(string(data)))
	for _, tok := range tokens {
		if strings.Contains(lower, tok) {
			return true, true
		}
	}
	return false, true
}
