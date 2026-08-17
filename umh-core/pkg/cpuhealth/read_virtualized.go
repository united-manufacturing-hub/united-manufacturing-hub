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

// Deciding whether this machine is a virtual machine, which is the fact that
// makes stolen CPU time worth measuring at all.

package cpuhealth

import (
	"context"
	"strings"
)

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
func (s *linuxSampler) readVirtualized(ctx context.Context) bool {
	if s.virtResolved {
		return s.virtualized
	}
	data, err := s.fs.ReadFile(ctx, "/proc/cpuinfo")
	if err == nil && cpuinfoHasHypervisorFlag(data) {
		s.virtualized = true
		s.virtResolved = true
		return true
	}
	// ARM64 route. A successful DMI read resolves the fact either way; a failed
	// DMI read leaves it unresolved so the next tick retries. The DMI identity
	// spans two independent sources: product_name and sys_vendor (the cloud
	// hypervisor an ARM64 product_name like "m6g.medium" never names), each read
	// separately so a failure or absence on one never breaks the other.
	pv, pok := s.dmiProductVirtualized(ctx)
	vv, vok := s.dmiVendorVirtualized(ctx)
	if (pok && pv) || (vok && vv) {
		s.virtualized = true
		s.virtResolved = true
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
	s.virtualized = false
	s.virtResolved = true
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
func (s *linuxSampler) dmiProductVirtualized(ctx context.Context) (virtualized, resolved bool) {
	data, err := s.fs.ReadFile(ctx, "/sys/class/dmi/id/product_name")
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
// "microsoft" is deliberately NOT here: sys_vendor "Microsoft Corporation" is
// ambiguous between Azure ARM guests (a VM) and Microsoft Surface machines
// (bare-metal OEM hardware), and a bare-metal Surface misread as a VM is the
// worse error. Azure ARM therefore stays an undetected known-limitation.
var dmiVendorHypervisorTokens = []string{
	"amazon",
	"google compute engine",
	"qemu",
}

// dmiVendorVirtualized is the second ARM64 DMI source. It reports whether
// /sys/class/dmi/id/sys_vendor names a cloud hypervisor, with the same
// read-failure contract as dmiProductVirtualized.
func (s *linuxSampler) dmiVendorVirtualized(ctx context.Context) (virtualized, resolved bool) {
	data, err := s.fs.ReadFile(ctx, "/sys/class/dmi/id/sys_vendor")
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
