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

// Virtualisation, including the ARM64 path. The sampler resolves a
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
		return cpuhealth.NewLinuxSampler(fs, base), &cpuinfoReads
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
		// must still match or steal stays dead on exactly the VM this fallback
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
		s := cpuhealth.NewLinuxSampler(fs, base)

		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Virtualized).To(BeFalse(),
			"a DMI read failure on tick 1 must not resolve Virtualized, so the next tick retries")

		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Virtualized).To(BeTrue(),
			"once the DMI read succeeds, the ARM64 VM must recover and resolve Virtualized=true")
	})

	It("retries an unreadable /proc/cpuinfo instead of permanently caching Virtualized=false", func() {
		ctx := context.Background()
		base := "/sys/fs/cgroup"

		// Every source is unreadable on the first tick, /proc/cpuinfo included,
		// and cpuinfo becomes readable on the second. An unreadable cpuinfo is
		// the case with the LEAST evidence of anything — it cannot even tell x86
		// from ARM64 — so it must leave the fact open, exactly as a failed DMI
		// read does. Caching false here costs the host steal attribution for the
		// whole process lifetime over a momentary read failure at startup, and
		// nothing short of a restart brings it back.
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
				if cpuinfoReads == 1 {
					return nil, errors.New("transient cpuinfo failure")
				}
				return x86VMCpuinfo, nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		s := cpuhealth.NewLinuxSampler(fs, base)

		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Virtualized).To(BeFalse(),
			"an unreadable /proc/cpuinfo is no evidence of virtualisation, so tick 1 reads false")

		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Virtualized).To(BeTrue(),
			"once /proc/cpuinfo becomes readable its hypervisor flag must be seen; a tick-1 read failure must not cache false for the process lifetime")
	})

	// The DMI vendor, which is ARM64's only working source. AWS
	// Graviton/Nitro puts its instance type (m6g.medium) in /sys/class/dmi/id/
	// product_name, GCP Tau T2A and Azure ARM put no hypervisor token there at
	// all — so lengthening the product_name token list cannot fix any of them.
	// The cloud hypervisor identity lives in a second DMI source the sampler
	// never reads today: /sys/class/dmi/id/sys_vendor. The sampler reads it as a
	// DISTINCT source — its own read failure case and its own read-once cache,
	// not an extended product_name list.
	It("resolves an ARM64 cloud VM from a second DMI source, sys_vendor, which product_name cannot catch, with its own failure handling", func() {
		ctx := context.Background()

		// newVendorSampler is the product_name harness plus a separately-controlled
		// /sys/class/dmi/id/sys_vendor. product and vendor err flags keep the
		// two DMI sources independent, so the test can prove sys_vendor's
		// distinctness (an unreadable one must not break the other) and its own
		// read-once cache.
		newVendorSampler := func(cpuinfo []byte, product string, productErr bool, vendor string, vendorErr bool) (cpuhealth.Sampler, *int) {
			vendorReads := 0
			fs := filesystem.NewMockFileSystem()
			fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
				switch path {
				case base + "/cpu.stat":
					return []byte("usage_usec 5000000\nnr_periods 0\nnr_throttled 0\n"), nil
				case base + "/cpu.max":
					return []byte("max 100000\n"), nil
				case "/proc/cpuinfo":
					return cpuinfo, nil
				case "/sys/class/dmi/id/product_name":
					if productErr {
						return nil, errors.New("unreadable")
					}
					return []byte(product), nil
				case "/sys/class/dmi/id/sys_vendor":
					vendorReads++
					if vendorErr {
						return nil, errors.New("unreadable")
					}
					return []byte(vendor), nil
				default:
					return nil, errors.New("unreadable")
				}
			}
			return cpuhealth.NewLinuxSampler(fs, base), &vendorReads
		}

		// The core case: an ARM64 cpuinfo (no hypervisor flag) whose product_name
		// names NO hypervisor token — AWS Graviton reports "m6g.medium" — but
		// whose sys_vendor names the cloud hypervisor "Amazon EC2". The guest is
		// provable only because sys_vendor is read at all; product_name alone
		// leaves steal dead on the exact box this source exists to detect.
		s, _ := newVendorSampler(armCpuinfo, "m6g.medium", false, "Amazon EC2", false)
		r, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeTrue(),
			"an ARM64 host whose DMI sys_vendor names a cloud hypervisor ('Amazon EC2') must resolve Virtualized=true even when product_name ('m6g.medium') carries no hypervisor token")

		// GCP Tau T2A and QEMU/KVM carry their vendor in sys_vendor, which no
		// product_name token list can reach; each must resolve through the same
		// second source, not just Amazon EC2. ("Microsoft Corporation" is NOT a
		// token — see the bare-metal Surface case just below.)
		for _, vendor := range []string{"Google Compute Engine", "QEMU", "Amazon EC2"} {
			s, _ = newVendorSampler(armCpuinfo, "Virtual Machine", false, vendor, false)
			r, err = s.Read(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Virtualized).To(BeTrue(),
				"an ARM64 cloud host whose DMI sys_vendor names %s must resolve Virtualized=true via the second DMI source, which product_name cannot catch", vendor)
		}

		// "Microsoft Corporation" as a sys_vendor must NOT imply a VM: it is also
		// the sys_vendor of bare-metal Microsoft Surface hardware, which is
		// indistinguishable from an Azure ARM guest by this token alone. A
		// Surface-class machine must stay Virtualized=false, not be misread as a
		// VM (which would turn the steal signal's HasVirtualization capability on
		// for bare metal). Dropping the token means Azure ARM is a documented
		// known-limitation; the bare-metal false-positive is the worse error.
		s, _ = newVendorSampler(bareMetalCpuinfo, "Surface Pro 8", false, "Microsoft Corporation", false)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeFalse(),
			"a bare-metal Microsoft Surface (sys_vendor 'Microsoft Corporation') must resolve Virtualized=false, never a VM")

		// sys_vendor is a DISTINCT source with its own failure handling. An
		// unreadable sys_vendor must not break the product_name path ...
		s, _ = newVendorSampler(armCpuinfo, "KVM", false, "", true)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeTrue(),
			"an unreadable sys_vendor must not break the product_name DMI path")

		// ... and, conversely, an unreadable product_name must not break the
		// sys_vendor path: each source stands alone.
		s, _ = newVendorSampler(armCpuinfo, "m6g.medium", true, "Amazon EC2", false)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeTrue(),
			"an unreadable product_name must not break the sys_vendor path — sys_vendor is a distinct source with its own read")

		// A bare-metal sys_vendor that names no hypervisor stays not-virtualized.
		s, _ = newVendorSampler(bareMetalCpuinfo, "PowerEdge R750", false, "Dell Inc.", false)
		r, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Virtualized).To(BeFalse(),
			"a bare-metal sys_vendor ('Dell Inc.') naming no hypervisor must resolve Virtualized=false")

		// Sticky, like product_name: sys_vendor is read once and cached, and the resolved
		// fact is re-published unchanged on the next tick.
		s, vendorReads := newVendorSampler(armCpuinfo, "m6g.medium", false, "Amazon EC2", false)
		first, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		second, err := s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Virtualized).To(BeTrue())
		Expect(second.Virtualized).To(Equal(first.Virtualized),
			"a sys_vendor-resolved fact must be cached and re-published unchanged on the next tick")
		Expect(*vendorReads).To(Equal(1),
			"sys_vendor must be read once and cached, not re-read every tick")

		// A transient sys_vendor read failure is retried, not permanently
		// cached as false.
		retryReads := 0
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
				return []byte("m6g.medium"), nil
			case "/sys/class/dmi/id/sys_vendor":
				retryReads++
				if retryReads == 1 {
					return nil, errors.New("transient sys_vendor failure")
				}
				return []byte("Amazon EC2"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		s = cpuhealth.NewLinuxSampler(fs, base)
		first, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Virtualized).To(BeFalse(),
			"a sys_vendor read failure on tick 1 must leave the fact unresolved so the next tick retries")
		second, err = s.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Virtualized).To(BeTrue(),
			"once the sys_vendor read succeeds, the ARM64 cloud VM must recover and resolve Virtualized=true")
	})
})
