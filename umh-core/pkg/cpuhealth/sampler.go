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

package cpuhealth

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"runtime"
	"strconv"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Sampler reads cgroup CPU usage observations.
//
// Not safe for concurrent use; Sample must be called from a single goroutine
// (the reconcile loop).
type Sampler interface {
	Sample(ctx context.Context) (Sample, error)
}

// cgroupSampler reads cgroup v2 cpu.stat usage_usec through the injected
// filesystem.Service and reports container-relative UsageCores as the counter
// delta over wall-clock. It also reads the non-throttle CPU-health signals
// (cpu.max quota, cpu.pressure avg60, /proc/cpuinfo virtualization, /proc/stat
// steal + host-busy) so the non-throttle causes in Decide are populated
// instead of left at zero. cpu.stat is the primary signal: a read/parse failure
// there fails the whole Sample. The other signals are best-effort: a read
// failure on any one of them zeros that field (and its readability flag) and
// the Sample is still returned.
type cgroupSampler struct {
	lastTime     time.Time
	lastStatTime time.Time
	fs           filesystem.Service
	basePath     string

	lastUsage int64

	// /proc/stat baseline for steal + host-busy deltas.
	lastStatTotal float64
	lastStatSteal float64
	lastStatBusy  float64
	hasBaseline   bool

	hasStatBaseline bool

	// /proc/cpuinfo virtualization flag (read on the first SUCCESSFUL Sample,
	// then cached; virtualization does not change at runtime). A transient
	// read failure leaves virtualizedDone=false so the next Sample retries,
	// rather than permanently caching Virtualized=false.
	virtualized     bool
	virtualizedDone bool
	// dmiDone tracks the DMI product_name read (ARM64 fallback for the x86-only
	// /proc/cpuinfo "hypervisor" flag). Cached after the first successful read.
	dmiDone bool
}

// NewCgroupSampler returns a Sampler that reads cpu.stat from basePath.
func NewCgroupSampler(fs filesystem.Service, basePath string) Sampler {
	return &cgroupSampler{fs: fs, basePath: basePath}
}

// Sample reads cpu.stat, parses usage_usec, and returns a Sample. On the first
// read (or after a counter reset) it stores a baseline and reports
// UsageCores == 0. It also populates the non-throttle signals (Quota,
// PressureAvg60/PsiAvailable, Virtualized, StealFraction, HostBusyCores,
// LogicalCpus); a read failure on any of those is best-effort and does not fail
// the Sample. cpu.stat is primary: its failure propagates as an error.
func (s *cgroupSampler) Sample(ctx context.Context) (Sample, error) {
	data, err := s.fs.ReadFile(ctx, s.basePath+"/cpu.stat")
	if err != nil {
		return Sample{}, err
	}

	usage, err := parseUsageUsec(data)
	if err != nil {
		return Sample{}, err
	}

	now := time.Now()
	sample := Sample{Timestamp: now}

	// Parse the throttle counters (nr_periods/nr_throttled) from the same
	// cpu.stat data already read for usage_usec, so the caller does not need
	// a second cpu.stat read. NrPeriodsAvailable=true marks a clean parse.
	nrPeriods, nrThrottled := parseCPUStatCounters(data)
	sample.NrPeriods = nrPeriods
	sample.NrThrottled = nrThrottled
	sample.NrPeriodsAvailable = true

	if !s.hasBaseline || usage < s.lastUsage {
		s.lastUsage = usage
		s.lastTime = now
		s.hasBaseline = true
	} else {
		elapsed := now.Sub(s.lastTime).Seconds()
		if elapsed > 0 {
			sample.UsageCores = float64(usage-s.lastUsage) / 1e6 / elapsed
			s.lastUsage = usage
			s.lastTime = now
		}
	}

	// Quota (cpu.max), non-primary; nil when absent/unreadable/uncapped.
	if q, ok := s.readCPUMax(ctx); ok {
		sample.Quota = q
	}

	// PSI (cpu.pressure some avg60), non-primary; PsiAvailable=false on
	// absence/error.
	s.readPressure(ctx, &sample)

	// Virtualization (/proc/cpuinfo hypervisor flag), cached after first read.
	s.readVirtualized(ctx, &sample)

	// /proc/stat steal + host-busy deltas, non-primary.
	s.readProcStat(ctx, now, &sample)

	// LogicalCpus is a stdlib call, not an I/O read.
	sample.LogicalCpus = float64(runtime.NumCPU())

	return sample, nil
}

// readCPUMax reads <basePath>/cpu.max and returns the quota in cores
// (quota/period). It returns (nil, false) when the file is absent/unreadable
// or malformed. The "max" keyword (cpu.max = "max <period>") is detected
// explicitly and yields (&0.0, true): a non-nil zero quota, which per the
// Decide contract means uncapped (no fallback to CgroupCores). Returning nil
// for "max" would make Decide fall back to CgroupCores and could false-degrade
// an uncapped container. A non-"max" non-numeric quota still yields nil via
// ParseInt failure.
func (s *cgroupSampler) readCPUMax(ctx context.Context) (*float64, bool) {
	data, err := s.fs.ReadFile(ctx, s.basePath+"/cpu.max")
	if err != nil {
		return nil, false
	}

	fields := bytes.Fields(bytes.TrimSpace(data))
	if len(fields) < 2 {
		return nil, false
	}

	// "max" keyword = unlimited quota. Return a non-nil zero so Decide
	// treats the container as uncapped, with no CgroupCores fallback (the
	// Quota=&0 contract on Sample).
	if string(fields[0]) == "max" {
		zero := 0.0

		return &zero, true
	}

	quota, err := strconv.ParseInt(string(fields[0]), 10, 64)
	if err != nil {
		return nil, false // unparsable → uncapped/nil
	}

	period, err := strconv.ParseInt(string(fields[1]), 10, 64)
	if err != nil || period <= 0 {
		return nil, false
	}

	cores := float64(quota) / float64(period)

	return &cores, true
}

// readPressure reads <basePath>/cpu.pressure, parses the "some" line's avg60,
// and sets PressureAvg60 (raw kernel 0..100 divided by 100 → 0..1 fraction) +
// PsiAvailable=true. On absence/error/parse-failure it leaves them zero/false.
func (s *cgroupSampler) readPressure(ctx context.Context, sample *Sample) {
	data, err := s.fs.ReadFile(ctx, s.basePath+"/cpu.pressure")
	if err != nil {
		return
	}

	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("some ")) {
			continue
		}

		for _, f := range bytes.Fields(line) {
			if !bytes.HasPrefix(f, []byte("avg60=")) {
				continue
			}

			val, perr := strconv.ParseFloat(string(f[len("avg60="):]), 64)
			if perr != nil {
				return
			}

			sample.PressureAvg60 = val / 100.0
			sample.PsiAvailable = true
			sample.PsiReadable = true

			return
		}
	}
}

// readVirtualized reads /proc/cpuinfo (looking for "hypervisor" in the flags
// line), caches the result on the first SUCCESSFUL read, and sets Virtualized
// on every Sample. A read failure leaves the cache unset so the next Sample
// retries; permanently caching Virtualized=false would silently disable the
// steal cause.
//
// On ARM64 /proc/cpuinfo has no "flags" line (it exposes "Features" with no
// "hypervisor" bit), so the cpuinfo check alone misses hypervisors there. As a
// fallback, when the cpuinfo check fails AND the DMI product_name has not yet
// been read, read /sys/class/dmi/id/product_name and look for a known
// hypervisor vendor token (case-insensitive). The DMI read is also cached
// (read-once). If both fail, Virtualized=false.
func (s *cgroupSampler) readVirtualized(ctx context.Context, sample *Sample) {
	if !s.virtualizedDone {
		data, err := s.fs.ReadFile(ctx, "/proc/cpuinfo")
		if err == nil {
			s.virtualized = parseVirtualized(data)
			s.virtualizedDone = true
		}
	}

	// ARM64 fallback: the x86 "hypervisor" flag is absent on ARM64. If the
	// cpuinfo check failed to prove virtualization, try the DMI product_name
	// (which carries vendor strings like "KVM", "VMware", "Xen", "Hyper-V",
	// "VirtualBox" on both arches). Cached (read-once).
	if !s.virtualized && !s.dmiDone {
		dmiData, err := s.fs.ReadFile(ctx, "/sys/class/dmi/id/product_name")

		s.dmiDone = err == nil
		if err == nil && parseDMIVirtualized(dmiData) {
			s.virtualized = true
			s.virtualizedDone = true
		}
	}

	sample.Virtualized = s.virtualized
}

// parseVirtualized reports whether the flags line in /proc/cpuinfo contains the
// "hypervisor" flag.
func parseVirtualized(data []byte) bool {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Bytes()
		if bytes.HasPrefix(line, []byte("flags")) && bytes.Contains(line, []byte("hypervisor")) {
			return true
		}
	}

	return false
}

// dmiHypervisorTokens are the vendor substrings (lowercased) that indicate a
// hypervisor in /sys/class/dmi/id/product_name. Matched case-insensitively.
var dmiHypervisorTokens = [][]byte{
	[]byte("vmware"),
	[]byte("kvm"),
	[]byte("xen"),
	[]byte("hyper-v"),
	[]byte("virtualbox"),
	[]byte("qemu"),
}

// parseDMIVirtualized reports whether the DMI product_name body contains a known
// hypervisor vendor token. Matching is case-insensitive.
func parseDMIVirtualized(data []byte) bool {
	lower := bytes.ToLower(bytes.TrimSpace(data))
	for _, tok := range dmiHypervisorTokens {
		if bytes.Contains(lower, tok) {
			return true
		}
	}

	return false
}

// readProcStat reads /proc/stat's first "cpu " line and computes StealFraction
// (steal jiffies delta / total jiffies delta, total excluding the guest fields)
// and HostBusyCores (busy jiffies delta EXCLUDING iowait, steal, guest,
// guest_nice ÷ USER_HZ ÷ elapsed; see parseProcStat). On the
// first read and on a counter reset it re-baselines and leaves
// HostBusyCoresAvailable false (no usable reading); on a successful delta it
// sets HostBusyCoresAvailable true and populates StealFraction + HostBusyCores.
// On absence/error/parse-failure it leaves both zero and Available false.
func (s *cgroupSampler) readProcStat(ctx context.Context, now time.Time, sample *Sample) {
	data, err := s.fs.ReadFile(ctx, "/proc/stat")
	if err != nil {
		return
	}

	total, steal, busy, ok := parseProcStat(data)
	if !ok {
		return
	}

	if !s.hasStatBaseline {
		s.lastStatTotal = total
		s.lastStatSteal = steal
		s.lastStatBusy = busy
		s.lastStatTime = now
		s.hasStatBaseline = true

		return
	}

	totalDelta := total - s.lastStatTotal
	if totalDelta <= 0 {
		// Counter reset / wrap (host reboot, /proc/stat wrap). Re-baseline
		// against the new sample and emit 0 for BOTH StealFraction and
		// HostBusyCores; a negative busy delta would publish a negative
		// HostBusyCores. The baseline update happens here (after the guard) so
		// the reset sample becomes the new baseline.
		s.lastStatTotal = total
		s.lastStatSteal = steal
		s.lastStatBusy = busy
		s.lastStatTime = now

		return
	}

	sample.HostBusyCoresAvailable = true
	sample.StealFraction = (steal - s.lastStatSteal) / totalDelta

	elapsed := now.Sub(s.lastStatTime).Seconds()
	if elapsed > 0 {
		sample.HostBusyCores = (busy - s.lastStatBusy) / 100.0 / elapsed
	}

	s.lastStatTotal = total
	s.lastStatSteal = steal
	s.lastStatBusy = busy
	s.lastStatTime = now
}

// parseProcStat parses the first "cpu " line of /proc/stat into total (sum of
// fields 0..7, EXCLUDING guest and guest_nice), steal (field 8 position, the
// steal column), and busy (user+nice+system+irq+softirq, EXCLUDING idle,
// iowait, steal, guest, guest_nice). It returns ok=false when the line is
// absent or malformed.
func parseProcStat(data []byte) (total, steal, busy float64, ok bool) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("cpu ")) {
			continue
		}

		fields := bytes.Fields(line)
		if len(fields) < 11 { // "cpu" + 10 counters
			return 0, 0, 0, false
		}

		vals := make([]float64, 10)

		for i := range 10 {
			v, perr := strconv.ParseFloat(string(fields[1+i]), 64)
			if perr != nil {
				return 0, 0, 0, false
			}

			vals[i] = v
		}

		// Indices: 0 user, 1 nice, 2 system, 3 idle, 4 iowait, 5 irq, 6 softirq,
		// 7 steal, 8 guest, 9 guest_nice.
		//
		// total sums fields 0..7 only: the kernel folds guest into user (0) and
		// guest_nice into nice (1), so adding fields 8 and 9 counts guest time
		// twice, inflating the StealFraction denominator and understating steal
		// on guest-heavy hosts.
		//
		// busy excludes idle (3), iowait (4), steal (7), guest (8), and
		// guest_nice (9). iowait is schedulable idle: a task waiting on I/O
		// occupies no CPU, so counting it as busy false-degrades I/O-heavy
		// no-limit hosts and blocks bridges while compute is genuinely spare.
		// steal is excluded because it is its own cause (folding it in
		// double-counts it); guest/guest_nice are excluded because they are
		// already inside user/nice.
		for i := range 8 {
			total += vals[i]
		}

		steal = vals[7]
		busy = vals[0] + vals[1] + vals[2] + vals[5] + vals[6]

		return total, steal, busy, true
	}

	return 0, 0, 0, false
}

// parseUsageUsec extracts the usage_usec field from a cgroup v2 cpu.stat body.
func parseUsageUsec(data []byte) (int64, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := bytes.Fields(sc.Bytes())
		if len(fields) >= 2 && bytes.Equal(fields[0], []byte("usage_usec")) {
			v, err := strconv.ParseInt(string(fields[1]), 10, 64)
			if err != nil {
				return 0, err
			}

			return v, nil
		}
	}

	if err := sc.Err(); err != nil {
		return 0, err
	}

	return 0, errors.New("cpu.stat: usage_usec not found")
}

// parseCPUStatCounters extracts nr_periods and nr_throttled from a cgroup v2
// cpu.stat body. Missing keys yield 0 (the file was read, but the keys are
// absent). It reuses the data already read for parseUsageUsec so the sampler
// does not need a second cpu.stat read.
func parseCPUStatCounters(data []byte) (nrPeriods, nrThrottled int64) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := bytes.Fields(sc.Bytes())
		if len(fields) < 2 {
			continue
		}

		val, err := strconv.ParseInt(string(fields[1]), 10, 64)
		if err != nil {
			continue
		}

		switch string(fields[0]) {
		case "nr_periods":
			nrPeriods = val
		case "nr_throttled":
			nrThrottled = val
		}
	}

	return nrPeriods, nrThrottled
}
