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

// S2 R4c — Virtualisation, including the ARM64 path. The sampler resolves a
// capability fact — is this host a VM? — once and caches it across ticks; it is
// NOT a per-tick re-read. On x86 the evidence is the "hypervisor" flag in
// /proc/cpuinfo's flags line. ARM64 /proc/cpuinfo has a Features line and no
// flags line at all, so the primary path can never succeed there and the
// distinct ARM64 route is the DMI fallback: read
// /sys/class/dmi/id/product_name and match a known hypervisor token. Dropping
// that ARM64 path makes steal dead on every ARM64 VM, because an ARM VM is then
// never marked Virtualized and the steal instrument's Requires gate stays shut.
// An unreadable cpuinfo is no evidence of virtualisation, so it resolves false;
// the result is sticky, cached on the first successful read and re-published
// without re-reading the file.
package cpuhealth_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("CPU virtualisation", func() {
	const base = "/sys/fs/cgroup"

	// An x86 /proc/cpuinfo whose flags line carries the "hypervisor" flag.
	x86VMCpuinfo := []byte("processor\t: 0\n" +
		"vendor_id\t: GenuineIntel\n" +
		"flags\t\t: fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat hypervisor\n")

	// A bare-metal x86 /proc/cpuinfo with no hypervisor flag.
	bareMetalCpuinfo := []byte("processor\t: 0\n" +
		"vendor_id\t: GenuineIntel\n" +
		"flags\t\t: fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat\n")

	// An ARM64 /proc/cpuinfo: a Features line and NO flags line. The primary
	// path can never succeed here, which is exactly why ARM64 needs the DMI
	// fallback.
	armCpuinfo := []byte("processor\t: 0\n" +
		"model name\t: ARMv8\n" +
		"Features\t: fp asimd evtstrm aes pmull sha1 sha2 crc32 atomics fphp asimdhp\n")

	// newVirtSampler serves a cgroup whose Read succeeds, plus the caller-chosen
	// /proc/cpuinfo (cpuinfoErr makes that read fail) and the caller-chosen DMI
	// product_name (dmiErr makes that read fail). The second return is a pointer
	// to the count of /proc/cpuinfo reads, so the sticky/cached assertion can
	// reward a single read. Every other path is unreadable so nothing unexpected
	// is silently served.
	newVirtSampler := func(cpuinfo []byte, cpuinfoErr bool, dmi string, dmiErr bool) (cpuhealth.Sampler, *int) {
		cpuinfoReads := 0
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return []byte("usage_usec 5000000\nnr_periods 0\nnr_throttled 0\n"), nil
			case base + "/cpu.max":
				return []byte("max 100000\n"), nil
			case "/proc/cpuinfo":
				cpuinfoReads++
				if cpuinfoErr {
					return nil, errors.New("unreadable")
				}
				return cpuinfo, nil
			case "/sys/class/dmi/id/product_name":
				if dmiErr {
					return nil, errors.New("unreadable")
				}
				return []byte(dmi), nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		return cpuhealth.NewCgroupSampler(fs, base), &cpuinfoReads
	}

	It("resolves virtualisation from the hypervisor flag on x86, via the distinct DMI fallback on ARM64, caches it sticky without re-reading cpuinfo, and reads false on an unreadable cpuinfo", func() {
		ctx := context.Background()

		// x86: a /proc/cpuinfo flags line carrying "hypervisor" names a guest.
		s, _ := newVirtSampler(x86VMCpuinfo, false, "", true)
		r, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeTrue(),
			"an x86 /proc/cpuinfo with the hypervisor flag must resolve Virtualized=true")

		// x86 bare metal: no hypervisor flag, so the host is not a VM.
		s, _ = newVirtSampler(bareMetalCpuinfo, false, "", true)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeFalse(),
			"a bare-metal x86 /proc/cpuinfo with no hypervisor flag must resolve Virtualized=false")

		// ARM64 is a real and distinct path. Its cpuinfo has a Features line and
		// no flags line, so the x86 primary path alone can never succeed there —
		// the DMI product_name fallback is the only route. A product_name naming
		// a known hypervisor resolves the very thing the x86 flag would have:
		// dropping this ARM64 path makes steal dead on every ARM64 VM.
		s, _ = newVirtSampler(armCpuinfo, false, "KVM", false)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeTrue(),
			"an ARM64 host whose DMI product_name names a hypervisor must resolve Virtualized=true via the distinct ARM64 fallback")

		// SMBIOS product_name casing is firmware-controlled, not reliably Title
		// Case; a real ARM64 guest can report it any casing, so a lowercase value
		// must still match or steal stays dead on exactly the VM this rung
		// exists to detect.
		s, _ = newVirtSampler(armCpuinfo, false, "kvm", false)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeTrue(),
			"a lowercase DMI product_name must still resolve Virtualized=true, because SMBIOS casing is not reliably Title Case")

		// The distinctness cuts the other way too: ARM64 cpuinfo alone is no
		// evidence — no flags line means no hypervisor flag to find — so without
		// the DMI fallback the host stays undetected and steal stays dead.
		s, _ = newVirtSampler(armCpuinfo, false, "", true)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeFalse(),
			"ARM64 /proc/cpuinfo alone (unreadable DMI) must not resolve Virtualized, proving the x86 flags path cannot cover ARM64")

		// Sticky: the fact is resolved once and cached across ticks. Two Reads
		// serve the same value and /proc/cpuinfo is read exactly once — the mock
		// rewards a single cpuinfo read, not a per-tick re-read.
		s, cpuinfoReads := newVirtSampler(x86VMCpuinfo, false, "", true)
		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Virtualized).To(BeTrue())
		Expect(second.Virtualized).To(Equal(first.Virtualized),
			"virtualisation is a cached startup fact and must be re-published unchanged on the next tick")
		Expect(*cpuinfoReads).To(Equal(1),
			"/proc/cpuinfo must be read once and cached, not re-read every tick")

		// An unreadable cpuinfo is no evidence of virtualisation: false.
		s, _ = newVirtSampler(nil, true, "", true)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeFalse(),
			"an unreadable /proc/cpuinfo leaves virtualisation undetected")
	})

	It("resolves Virtualized on ARM64 from hypervisor vendor tokens beyond kvm", func() {
		ctx := context.Background()

		// The DMI fallback must not be kvm-only: an ARM64 VM provisioned under
		// VMware, Xen, Hyper-V or VirtualBox reports its own vendor string and
		// would otherwise never be marked Virtualized, leaving the steal
		// instrument's HasVirtualization capability shut. Each of these resolves
		// via the same ARM64 cpuinfo (no flags line) + DMI product_name route.
		for _, vendor := range []string{"VMware7,1", "Xen", "VirtualBox", "QEMU", "Hyper-V Virtual Machine"} {
			s, _ := newVirtSampler(armCpuinfo, false, vendor, false)
			r, err := s.Read(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Virtualized).To(BeTrue(),
				"an ARM64 host whose DMI product_name names %s must resolve Virtualized=true, not just kvm", vendor)
		}
	})

	It("caches a resolved-false too: a bare-metal machine whose DMI read succeeds with a non-matching product_name stays not-virtualized across reads", func() {
		ctx := context.Background()

		// The DMI read SUCCEEDS but names no hypervisor (a genuinely bare-metal
		// host like a Dell). This is the resolved-and-cached-false path
		// (`return false, true`), distinct from the unreadable-DMI case every
		// other assertion hits. A regression that treated a resolved false as
		// unresolved and re-read cpuinfo every tick would break the sticky
		// contract this test pins: the fact must be resolved once and cached.
		s, cpuinfoReads := newVirtSampler(bareMetalCpuinfo, false, "Dell Inc.", false)
		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Virtualized).To(BeFalse(),
			"a readable non-hypervisor DMI product_name on bare metal must resolve Virtualized=false")
		Expect(second.Virtualized).To(Equal(first.Virtualized),
			"a resolved false is still a resolved fact and must be cached, not re-derived")
		Expect(*cpuinfoReads).To(Equal(1),
			"a resolved false must also cache; /proc/cpuinfo is read once, not every tick")
	})

	It("retries a failed DMI read instead of permanently caching Virtualized=false", func() {
		ctx := context.Background()
		base := "/sys/fs/cgroup"

		// The DMI product_name read fails on the first tick (a transient error on
		// a real ARM64 VM) and succeeds on the second. The ARM64 cpuinfo never
		// carries the hypervisor flag, so only DMI can prove the guest. The fact
		// must stay unresolved across the first tick and be recovered on the
		// second — a permanently cached false would leave steal dead forever.
		dmiReads := 0
		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case base + "/cpu.stat":
				return []byte("usage_usec 5000000\nnr_periods 0\nnr_throttled 0\n"), nil
			case base + "/cpu.max":
				return []byte("max 100000\n"), nil
			case "/proc/cpuinfo":
				return armCpuinfo, nil
			case "/sys/class/dmi/id/product_name":
				dmiReads++
				if dmiReads == 1 {
					return nil, errors.New("transient dmi failure")
				}
				return []byte("KVM"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		s := cpuhealth.NewCgroupSampler(fs, base)

		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Virtualized).To(BeFalse(),
			"a DMI read failure on tick 1 must not resolve Virtualized, so the next tick retries")

		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Virtualized).To(BeTrue(),
			"once the DMI read succeeds, the ARM64 VM must recover and resolve Virtualized=true")
	})
})
