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

//go:build test

// Run with -tags=test: this file is excluded from plain go test, which
// reports green with 0 specs. CI passes -tags=test for this package.
package cpuhealth_test

import (
	"bufio"
	"context"
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// TestCgroupSampler_PopulatesAllSignals pins the cgroupSampler reading every
// CPU-health signal (cpu.pressure PSI avg60, /proc/cpuinfo virtualization,
// cpu.max quota, /proc/stat steal + host-busy) and populating the full
// Sample, not just usage_usec/nr_periods/nr_throttled: Decide reads the
// cause fields, so a sampler that leaves them zero silently disables the
// non-throttle causes.
//
// Each signal is read via the injected filesystem.Service, parsed, and set on
// the returned Sample. Pinned behaviors:
//
//  1. PSI: read <basePath>/cpu.pressure, parse the "some" line's avg60 (the
//     file is "some avg10=X avg60=Y avg300=Z total=T"). The raw kernel value is
//     0..100; DIVIDE BY 100 before assigning to PressureAvg60 (a 0..1 fraction).
//     PsiAvailable=true when the file exists+parses; false (PressureAvg60=0)
//     when absent/unreadable. A transient read error does NOT fail the whole
//     Sample; it just leaves PsiAvailable=false.
//  2. VIRTUALIZATION: read /proc/cpuinfo, look for "hypervisor" in the flags
//     line. Set Virtualized=true if found, false otherwise. CACHED: read once on
//     the first Sample() call, reused after (virtualization doesn't change at
//     runtime); the test asserts /proc/cpuinfo is NOT re-read on the second
//     call.
//  3. QUOTA: read <basePath>/cpu.max, parse "quota period" (e.g. "200000
//     100000" = 2.0 cores). Set Quota=&quotaCores (quota/period) when a numeric
//     quota is set; Quota=&0 for "max <period>" (unlimited); Quota=nil when
//     absent/unreadable.
//  4. STEAL: read /proc/stat first "cpu " line's 10 fields; per-tick steal
//     fraction = steal_jiffies_delta / total_jiffies_delta (total = sum of ALL
//     fields). First read (no delta) → StealFraction=0. Counter reset (total
//     decreases) → re-baseline, StealFraction=0.
//  5. HOST-BUSY: from the same /proc/stat read, HostBusyCores =
//     (sum of non-idle fields EXCLUDING steal AND guest/guest_nice) ÷ USER_HZ ÷
//     elapsed_seconds. The exclusion of steal is critical (it's its own cause;
//     folding it in double-counts); guest/guest_nice are excluded because the
//     kernel double-counts them inside user/nice.
//  6. LOGICAL_CPUS: LogicalCpus = float64(runtime.NumCPU()) (stdlib call, not
//     an I/O read).
//  7. The Sample carries ALL fields. (8) A read failure on ONE non-primary
//     signal does NOT fail the whole Sample: cpu.stat failure still errors
//     (primary signal), but cpu.pressure/cpu.max/cpuinfo/proc.stat failures
//     just zero that field + set the readability flag false and the Sample is
//     still returned.
//
// This is an integration test against the real cgroupSampler via its public
// constructor + the real filesystem mock (no verdict mocking, no over-mocking).
func TestCgroupSampler_PopulatesAllSignals(t *testing.T) {
	const basePath = "/sys/fs/cgroup"
	cpuStatPath := basePath + "/cpu.stat"
	cpuMaxPath := basePath + "/cpu.max"
	cpuPressurePath := basePath + "/cpu.pressure"
	procStatPath := "/proc/stat"
	procCpuinfoPath := "/proc/cpuinfo"
	ctx := context.Background()

	// A scripted /proc/stat series with a real steal column + non-idle fields.
	// Fields: user nice system idle iowait irq softirq steal guest guest_nice.
	// Tick 0: total = 1000+1000+1000+8000+0+0+0+50+0+0 = 11050; steal=50.
	// Tick 1: steal grows to 150 (delta 100), total grows to 22050 (delta
	// 11000) → steal fraction = 100/11000 ≈ 0.00909.
	// Tick 1 non-idle (excluding steal, guest, guest_nice) delta:
	//   (2000-1000)+(1500-1000)+(2000-1000)+0+0+0 = 2500 jiffies busy.
	//   USER_HZ=100 → 2500/100 = 25 core-seconds of host busy.
	procStatTick0 := "cpu  1000 1000 1000 8000 0 0 0 50 0 0\n"
	procStatTick1 := "cpu  2000 1500 2000 16400 0 0 0 150 0 0\n"

	cpuMaxContent := "200000 100000\n" // quota=200000 period=100000 → 2.0 cores
	cpuPressureContent := "some avg10=5.00 avg60=25.00 avg300=10.00 total=12345\n" +
		"full avg10=1.00 avg60=2.00 avg300=1.00 total=1234\n"
	cpuInfoContent := "processor\t: 0\nflags\t: fpu vme de pse tsc msr hypervisor lm\n"

	// Mutate the cpu.stat usage_usec + /proc/stat per call to drive deltas.
	usageUsec := int64(1_000_000)
	procStat := procStatTick0
	var pathsRead []string
	cpuinfoReads := 0

	fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
		pathsRead = append(pathsRead, path)
		switch path {
		case cpuStatPath:
			return []byte(cpuStatContent(usageUsec)), nil
		case cpuMaxPath:
			return []byte(cpuMaxContent), nil
		case cpuPressurePath:
			return []byte(cpuPressureContent), nil
		case procStatPath:
			return []byte(procStat), nil
		case procCpuinfoPath:
			cpuinfoReads++
			return []byte(cpuInfoContent), nil
		default:
			return nil, errors.New("unexpected path read: " + path)
		}
	})

	s := cpuhealth.NewCgroupSampler(fs, basePath)

	// (1) First Sample: baselines usage + /proc/stat, no deltas yet. Still
	// must populate Quota/Pressure/Virtualized/LogicalCpus (no-delta fields are
	// 0, but the readability flags + cached reads must already be set).
	sample1, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("first Sample: unexpected error: %v", err)
	}

	// Quota: 200000/100000 = 2.0 cores.
	if sample1.Quota == nil {
		t.Fatalf("first Sample Quota: got nil, want non-nil (cpu.max=200000 100000 → 2.0 cores)")
	}
	if !approx(*sample1.Quota, 2.0, 0.01) {
		t.Fatalf("first Sample Quota: got %v, want 2.0 (quota/period)", *sample1.Quota)
	}

	// PSI: avg60=25.00 (kernel 0..100) ÷ 100 = 0.25 fraction. PsiAvailable=true.
	if !sample1.PsiAvailable {
		t.Fatalf("first Sample PsiAvailable: got false, want true (cpu.pressure present + parsed)")
	}
	if !approx(sample1.PressureAvg60, 0.25, 0.01) {
		t.Fatalf("first Sample PressureAvg60: got %v, want 0.25 (avg60=25.00 ÷ 100; "+
			"raw kernel percentage must be divided by 100 before assignment)", sample1.PressureAvg60)
	}

	// Virtualized: /proc/cpuinfo flags line contains "hypervisor".
	if !sample1.Virtualized {
		t.Fatalf("first Sample Virtualized: got false, want true (/proc/cpuinfo flags line has hypervisor)")
	}
	if cpuinfoReads != 1 {
		t.Fatalf("first Sample: /proc/cpuinfo read %d time(s), want exactly 1 (cached after first read)", cpuinfoReads)
	}

	// LogicalCpus: runtime.NumCPU() (not an I/O read).
	if sample1.LogicalCpus != float64(runtime.NumCPU()) {
		t.Fatalf("first Sample LogicalCpus: got %v, want %v (runtime.NumCPU())",
			sample1.LogicalCpus, float64(runtime.NumCPU()))
	}

	// First-read deltas are 0 (no baseline): StealFraction=0, HostBusyCores=0.
	if sample1.StealFraction != 0 {
		t.Fatalf("first Sample StealFraction: got %v, want 0 (first read: no delta)", sample1.StealFraction)
	}
	if sample1.HostBusyCores != 0 {
		t.Fatalf("first Sample HostBusyCores: got %v, want 0 (first read: no delta)", sample1.HostBusyCores)
	}

	// (2) Second Sample: deltas exist. Quota/Virtualized/LogicalCpus unchanged
	// (Virtualized CACHED: /proc/cpuinfo must NOT be re-read).
	time.Sleep(80 * time.Millisecond)
	usageUsec = 1_000_000 + 100_000 // +0.1 core-seconds of cgroup CPU
	procStat = procStatTick1

	sample2, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("second Sample: unexpected error: %v", err)
	}

	// Virtualized cached: /proc/cpuinfo NOT re-read.
	if cpuinfoReads != 1 {
		t.Fatalf("second Sample: /proc/cpuinfo read %d time(s) total, want still 1 (virtualization is cached after first read)", cpuinfoReads)
	}
	if !sample2.Virtualized {
		t.Fatalf("second Sample Virtualized: got false, want true (cached from first read)")
	}

	// Steal fraction = steal_delta / total_delta = 100/11000 ≈ 0.0090909.
	wantSteal := 100.0 / 11000.0
	if !approx(sample2.StealFraction, wantSteal, 0.02) {
		t.Fatalf("second Sample StealFraction: got %v, want %v (steal_jiffies_delta/total_jiffies_delta = 100/11000)",
			sample2.StealFraction, wantSteal)
	}

	// HostBusyCores: non-idle (excl steal, guest, guest_nice) delta jiffies ÷
	// USER_HZ(100) ÷ elapsed. Non-idle-excl fields delta:
	//   user (2000-1000)=1000 + nice (1500-1000)=500 + system (2000-1000)=1000
	//   + iowait 0 + irq 0 + softirq 0 = 2500 jiffies.
	elapsed := sample2.Timestamp.Sub(sample1.Timestamp).Seconds()
	if elapsed <= 0 {
		t.Fatalf("elapsed between samples non-positive: %v", elapsed)
	}
	wantHostBusy := 2500.0 / 100.0 / elapsed // USER_HZ=100
	if !approx(sample2.HostBusyCores, wantHostBusy, 0.05) {
		t.Fatalf("second Sample HostBusyCores: got %v, want %v (non-idle-excl-steal-guest jiffies_delta(2500)/USER_HZ(100)/elapsed); "+
			"steal MUST be excluded (folding it in double-counts steal, its own cause)",
			sample2.HostBusyCores, wantHostBusy)
	}

	// Quota + PSI + LogicalCpus still set on the second sample.
	if sample2.Quota == nil || !approx(*sample2.Quota, 2.0, 0.01) {
		t.Fatalf("second Sample Quota: got %v, want 2.0 (cpu.max unchanged)", sample2.Quota)
	}
	if !sample2.PsiAvailable || !approx(sample2.PressureAvg60, 0.25, 0.01) {
		t.Fatalf("second Sample PSI: PsiAvailable=%v PressureAvg60=%v, want true/0.25", sample2.PsiAvailable, sample2.PressureAvg60)
	}
	if sample2.LogicalCpus != float64(runtime.NumCPU()) {
		t.Fatalf("second Sample LogicalCpus: got %v, want %v", sample2.LogicalCpus, float64(runtime.NumCPU()))
	}
}

// TestCgroupSampler_UsageCores pins the cgroup Sampler behavior.
//
// The UsageCores divisor is 1e6 (microseconds->seconds), NOT USER_HZ (100) --
// confusing them inflates UsageCores ~10000x. The sampler never reads host
// /proc/stat: reported UsageCores tracks the mock cpuStatContent value, not
// host load, so it must not call gopsutil/host cpu.PercentWithContext.
//
// Behaviors pinned:
//  1. First read: no delta exists yet -> UsageCores == 0 (counter + read time
//     stored as a baseline). Sample carries Timestamp = read time and
//     CgroupCores == 0 (the sampler reads only cpu.stat; the caller fills
//     CgroupCores from cpu.max in a later step).
//  2. Subsequent read: usage_usec rising by deltaUsec over wall-clock elapsed
//     -> UsageCores == deltaUsec/1e6/elapsed. Asserted against the sampler's
//     OWN returned Timestamps (same clock the divisor uses), so the assertion
//     pins the 1e6 divisor + wall-clock denominator precisely; a USER_HZ (100)
//     divisor would be ~10000x off.
//  3. The sampler does NOT fall back to a gopsutil host-numerator for
//     UsageCores: reported UsageCores tracks the mock cpuStatContent value, not
//     host load, so it must not call gopsutil/host cpu.PercentWithContext.
//     Asserted at (2): a host-derived value would not track the mock value.
//  4. Counter reset (usage_usec decreases between reads, e.g. cgroup recreated
//     / pod rescheduled): the sampler re-baselines -> UsageCores == 0, stores
//     the new counter + time.
//  5. After re-baseline, a further positive delta produces a delta-relative
//     UsageCores again (proves reset cleared the old baseline rather than
//     leaving a stale one that would yield a negative or huge value).
//
// This is an integration test against the real cgroupSampler via its public
// constructor + the real filesystem mock (no verdict mocking, no clock mocking
// beyond the real wall-clock the sampler itself uses).
func TestCgroupSampler_UsageCores(t *testing.T) {
	const basePath = "/sys/fs/cgroup"
	cpuStatPath := basePath + "/cpu.stat"
	ctx := context.Background()

	// Scripted usage_usec the mock cpu.stat returns. Mutated between reads to
	// drive the delta / reset scenarios.
	current := int64(1_000_000)

	// Every path the sampler reads via filesystem.Service is recorded. The
	// sampler reads only cpu.stat and must touch no other path; a host
	// /proc/stat read (or any non-cgroup path) would appear here.
	var pathsRead []string

	fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
		pathsRead = append(pathsRead, path)
		if path != cpuStatPath {
			return nil, errors.New("unexpected path read: " + path)
		}
		return []byte(cpuStatContent(current)), nil
	})

	s := cpuhealth.NewCgroupSampler(fs, basePath)

	// (1) First read: no delta yet -> UsageCores == 0.
	current = 1_000_000
	sample1, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("first Sample: unexpected error: %v", err)
	}
	if sample1.UsageCores != 0 {
		t.Fatalf("first Sample UsageCores: got %v, want 0 (no baseline yet)", sample1.UsageCores)
	}
	if sample1.CgroupCores != 0 {
		t.Fatalf("first Sample CgroupCores: got %v, want 0 (sampler reads only cpu.stat; caller fills from cpu.max later)", sample1.CgroupCores)
	}
	if sample1.Timestamp.IsZero() {
		t.Fatalf("first Sample Timestamp: got zero, want the read time")
	}

	// (2) Subsequent read: usage_usec rises by deltaUsec over wall-clock.
	// Sleep so elapsed is comfortably non-zero (avoids div-by-zero flake);
	// the expected value is derived from the sampler's own returned
	// Timestamps, so it is independent of the exact sleep duration.
	deltaUsec := int64(100_000) // 0.1 core-second of cgroup CPU
	current = 1_000_000 + deltaUsec
	sleepFor := 80 * time.Millisecond
	time.Sleep(sleepFor)

	sample2, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("second Sample: unexpected error: %v", err)
	}
	if sample2.CgroupCores != 0 {
		t.Fatalf("second Sample CgroupCores: got %v, want 0 (sampler reads only cpu.stat)", sample2.CgroupCores)
	}
	if sample2.Timestamp.IsZero() {
		t.Fatalf("second Sample Timestamp: got zero, want the read time")
	}

	elapsed := sample2.Timestamp.Sub(sample1.Timestamp).Seconds()
	if elapsed <= 0 {
		t.Fatalf("elapsed between samples non-positive: %v", elapsed)
	}
	wantUsage := float64(deltaUsec) / 1e6 / elapsed
	if !approx(sample2.UsageCores, wantUsage, 0.02) {
		t.Fatalf("second Sample UsageCores: got %v, want %v (deltaUsec/1e6/elapsed); "+
			"a USER_HZ (100) divisor would inflate ~10000x to %v",
			sample2.UsageCores, wantUsage, float64(deltaUsec)/100/elapsed)
	}
	// Upper-bound guard rejects the USER_HZ (~10000x) divisor bug: even with
	// timing jitter the correct value is ~1.0, while a USER_HZ divisor would
	// land ~10000x higher.
	if sample2.UsageCores > 10 {
		t.Fatalf("second Sample UsageCores impossibly high: got %v (divisor likely USER_HZ not 1e6)", sample2.UsageCores)
	}

	// (3) The sampler reads <basePath>/cpu.stat via filesystem.Service on every
	// Sample (it also reads cpu.max/cpu.pressure//proc/stat//proc/cpuinfo, so
	// cpu.stat is no longer the only path read). The protective intent here is
	// the absence of a gopsutil host-numerator fallback: reported UsageCores
	// must track the mock cpuStatContent delta above, not host load (asserted
	// at (2)); a host-derived value would not track the mock value.
	// Assert cpu.stat was read exactly once per Sample (the usage_usec source).
	cpuStatReads := 0
	for _, p := range pathsRead {
		if p == cpuStatPath {
			cpuStatReads++
		}
	}
	if cpuStatReads != 2 {
		t.Fatalf("cpu.stat reads after two samples: got %d, want 2 (one cpu.stat read per Sample)", cpuStatReads)
	}

	// (4) Counter reset: usage_usec decreases (cgroup recreated / pod
	// rescheduled). The sampler must re-baseline: UsageCores == 0, new
	// counter + time stored.
	current = 500_000 // less than the previous 1_100_000
	time.Sleep(sleepFor)

	sample3, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("reset Sample: unexpected error: %v", err)
	}
	if sample3.UsageCores != 0 {
		t.Fatalf("reset Sample UsageCores: got %v, want 0 (counter reset must re-baseline, not emit a negative delta)", sample3.UsageCores)
	}
	if sample3.CgroupCores != 0 {
		t.Fatalf("reset Sample CgroupCores: got %v, want 0", sample3.CgroupCores)
	}
	if sample3.Timestamp.IsZero() {
		t.Fatalf("reset Sample Timestamp: got zero, want the read time")
	}

	// (5) After re-baseline, a further positive delta yields a delta-relative
	// UsageCores again -- proving the reset cleared the stale baseline (else
	// the delta would be measured against the pre-reset 1_100_000 counter and
	// go negative / clamp, not match the new baseline).
	deltaUsec2 := int64(100_000)
	current = 500_000 + deltaUsec2
	time.Sleep(sleepFor)

	sample4, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("post-reset delta Sample: unexpected error: %v", err)
	}
	elapsed2 := sample4.Timestamp.Sub(sample3.Timestamp).Seconds()
	if elapsed2 <= 0 {
		t.Fatalf("elapsed after reset non-positive: %v", elapsed2)
	}
	wantUsage2 := float64(deltaUsec2) / 1e6 / elapsed2
	if !approx(sample4.UsageCores, wantUsage2, 0.02) {
		t.Fatalf("post-reset Sample UsageCores: got %v, want %v (re-baselined delta); "+
			"a stale pre-reset baseline would not produce this", sample4.UsageCores, wantUsage2)
	}
	if sample4.CgroupCores != 0 {
		t.Fatalf("post-reset Sample CgroupCores: got %v, want 0", sample4.CgroupCores)
	}
}

// TestCgroupSampler_ErrorPaths covers the Sampler's error branches through the
// public Sample API by driving the mock filesystem.Service with crafted
// cpu.stat bodies. The missing-usage_usec case is the guard that prevents a
// broken cgroup from looking healthy: if parseUsageUsec returned (0, nil) for
// an absent key, Sample would return a valid Sample with UsageCores==0, which
// Decide reads as fraction=0 -> StateHealthy. That case must yield a non-nil
// error, never (Sample{}, nil).
func TestCgroupSampler_ErrorPaths(t *testing.T) {
	const basePath = "/sys/fs/cgroup"
	ctx := context.Background()

	t.Run("readfile_error_propagates", func(t *testing.T) {
		readErr := errors.New("boom: cgroup unreadable")
		fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, _ string) ([]byte, error) {
			return nil, readErr
		})
		s := cpuhealth.NewCgroupSampler(fs, basePath)

		sample, err := s.Sample(ctx)
		if err == nil {
			t.Fatalf("Sample: expected error from ReadFile, got nil")
		}
		if !errors.Is(err, readErr) {
			t.Fatalf("Sample error: got %v, want it to wrap %v", err, readErr)
		}
		if sample.UsageCores != 0 || !sample.Timestamp.IsZero() {
			t.Fatalf("Sample on error: got %+v, want zero-valued Sample", sample)
		}
	})

	t.Run("missing_usage_usec_returns_error_not_zero_sample", func(t *testing.T) {
		// cpu.stat body with no usage_usec key. Must NOT yield (Sample{}, nil);
		// doing so would let Decide compute fraction=0 -> StateHealthy on a
		// broken/weird cgroup.
		body := "nr_periods 100\nnr_throttled 0\nthrottled_usec 0\n"
		fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, _ string) ([]byte, error) {
			return []byte(body), nil
		})
		s := cpuhealth.NewCgroupSampler(fs, basePath)

		sample, err := s.Sample(ctx)
		if err == nil {
			t.Fatalf("Sample: expected error for missing usage_usec, got nil (Sample=%+v) -- "+
				"a missing usage_usec must NEVER yield (Sample{}, nil)", sample)
		}
		if err.Error() != "cpu.stat: usage_usec not found" {
			t.Fatalf("Sample error: got %q, want %q", err.Error(), "cpu.stat: usage_usec not found")
		}
		if sample.UsageCores != 0 || !sample.Timestamp.IsZero() {
			t.Fatalf("Sample on error: got %+v, want zero-valued Sample", sample)
		}
	})

	t.Run("non_numeric_usage_usec_propagates_strconv_error", func(t *testing.T) {
		body := "nr_periods 100\nusage_usec NaN\n"
		fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, _ string) ([]byte, error) {
			return []byte(body), nil
		})
		s := cpuhealth.NewCgroupSampler(fs, basePath)

		sample, err := s.Sample(ctx)
		if err == nil {
			t.Fatalf("Sample: expected strconv error for non-numeric usage_usec, got nil (Sample=%+v)", sample)
		}
		if !errors.Is(err, strconv.ErrSyntax) {
			t.Fatalf("Sample error: got %v, want it to wrap strconv.ErrSyntax", err)
		}
		if sample.UsageCores != 0 || !sample.Timestamp.IsZero() {
			t.Fatalf("Sample on error: got %+v, want zero-valued Sample", sample)
		}
	})

	t.Run("oversized_line_propagates_scanner_error", func(t *testing.T) {
		// A single line exceeding bufio.Scanner's default 64KiB token limit
		// makes sc.Scan() return false with sc.Err() == bufio.ErrTooLong.
		body := strings.Repeat("x", 65537)
		fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, _ string) ([]byte, error) {
			return []byte(body), nil
		})
		s := cpuhealth.NewCgroupSampler(fs, basePath)

		sample, err := s.Sample(ctx)
		if err == nil {
			t.Fatalf("Sample: expected scanner error for oversized line, got nil (Sample=%+v)", sample)
		}
		if !errors.Is(err, bufio.ErrTooLong) {
			t.Fatalf("Sample error: got %v, want it to wrap bufio.ErrTooLong", err)
		}
		if sample.UsageCores != 0 || !sample.Timestamp.IsZero() {
			t.Fatalf("Sample on error: got %+v, want zero-valued Sample", sample)
		}
	})
}

// cpuStatContent builds a cgroup v2 cpu.stat body whose usage_usec field is the
// given value. usage_usec is intentionally NOT the first key so the parser must
// scan by key name rather than assume a positional first token.
func cpuStatContent(usageUsec int64) string {
	return strings.Join([]string{
		"nr_periods 100",
		"nr_throttled 0",
		"throttled_usec 0",
		"usage_usec " + strconv.FormatInt(usageUsec, 10),
		"user_usec " + strconv.FormatInt(usageUsec/2, 10),
		"system_usec " + strconv.FormatInt(usageUsec-usageUsec/2, 10),
	}, "\n") + "\n"
}

// approx reports whether got is within relative tolerance tol of want.
func approx(got, want, tol float64) bool {
	if want == 0 {
		return got == 0
	}
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff/math.Abs(want) <= tol
}

// TestCgroupSampler_CPUMaxKeyword pins the readCPUMax "max" contract: cpu.max
// "max <period>" (unlimited quota) MUST yield a non-nil zero Quota, not nil.
// The Decide contract (decide_test.go TestDecide_QuotaPointerUncapped) treats
// Quota=&0 as uncapped (no CgroupCores fallback); a nil Quota falls back to
// CgroupCores, which can false-degrade an uncapped container. The "max" keyword
// is detected explicitly before the ParseInt attempt.
func TestCgroupSampler_CPUMaxKeyword(t *testing.T) {
	const basePath = "/sys/fs/cgroup"
	cpuStatPath := basePath + "/cpu.stat"
	cpuMaxPath := basePath + "/cpu.max"
	ctx := context.Background()

	fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
		switch path {
		case cpuStatPath:
			return []byte(cpuStatContent(1_000_000)), nil
		case cpuMaxPath:
			return []byte("max 100000\n"), nil // unlimited quota
		default:
			return nil, errors.New("unexpected path read: " + path)
		}
	})

	s := cpuhealth.NewCgroupSampler(fs, basePath)

	sample, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("Sample: unexpected error: %v", err)
	}

	if sample.Quota == nil {
		t.Fatalf("Quota: got nil for cpu.max=\"max 100000\", want non-nil &0.0 " +
			"(nil would make Decide fall back to CgroupCores and false-degrade an uncapped container)")
	}

	if *sample.Quota != 0.0 {
		t.Fatalf("Quota: got %v, want 0.0 (cpu.max=\"max\" is uncapped → non-nil zero, "+
			"matching the Decide Quota=&0 contract)", *sample.Quota)
	}
}

// TestCgroupSampler_VirtualizedDMIFallback pins the ARM64 virtualization
// detection fallback: ARM64 /proc/cpuinfo has a "Features" line (no "hypervisor"
// bit), so the x86-only flags check misses hypervisors there. The sampler must
// fall back to /sys/class/dmi/id/product_name and set Virtualized=true when a
// known vendor token (KVM, VMware, Xen, Hyper-V, VirtualBox, QEMU) is present.
// Both the cpuinfo and DMI reads are cached (read-once).
func TestCgroupSampler_VirtualizedDMIFallback(t *testing.T) {
	const basePath = "/sys/fs/cgroup"
	cpuStatPath := basePath + "/cpu.stat"
	procStatPath := "/proc/stat"
	procCpuinfoPath := "/proc/cpuinfo"
	dmiPath := "/sys/class/dmi/id/product_name"
	ctx := context.Background()

	// ARM64-style /proc/cpuinfo: "Features" line, no "hypervisor" flag.
	arm64Cpuinfo := "processor\t: 0\nFeatures\t: fp asimd evtstrm aes pmull sha1 sha2 crc32 atomics\n"

	// Baseline + delta /proc/stat so Sample does not fail on a missing cpu stat.
	procStatTick0 := "cpu  1000 1000 1000 8000 0 0 0 50 0 0\n"
	procStatTick1 := "cpu  1100 1100 1100 8100 0 0 0 60 0 0\n"

	t.Run("arm64_cpuinfo_plus_dmi_kvm_sets_virtualized", func(t *testing.T) {
		cpuinfoReads := 0
		dmiReads := 0
		procStat := procStatTick0

		fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case cpuStatPath:
				return []byte(cpuStatContent(1_000_000)), nil
			case procStatPath:
				return []byte(procStat), nil
			case procCpuinfoPath:
				cpuinfoReads++
				return []byte(arm64Cpuinfo), nil
			case dmiPath:
				dmiReads++
				return []byte("KVM Virtual Machine\n"), nil
			default:
				return nil, errors.New("unexpected path read: " + path)
			}
		})

		s := cpuhealth.NewCgroupSampler(fs, basePath)

		sample1, err := s.Sample(ctx)
		if err != nil {
			t.Fatalf("first Sample: unexpected error: %v", err)
		}

		if !sample1.Virtualized {
			t.Fatalf("first Sample Virtualized: got false, want true " +
				"(ARM64 cpuinfo has no hypervisor flag; DMI product_name=\"KVM Virtual Machine\" must set Virtualized via the fallback)")
		}

		if cpuinfoReads != 1 {
			t.Fatalf("first Sample: /proc/cpuinfo read %d time(s), want 1", cpuinfoReads)
		}

		if dmiReads != 1 {
			t.Fatalf("first Sample: DMI product_name read %d time(s), want 1 (fallback triggered when cpuinfo check failed)", dmiReads)
		}

		// Second Sample: both reads cached; neither path re-read.
		procStat = procStatTick1
		_, err = s.Sample(ctx)
		if err != nil {
			t.Fatalf("second Sample: unexpected error: %v", err)
		}

		if cpuinfoReads != 1 {
			t.Fatalf("second Sample: /proc/cpuinfo read %d time(s) total, want still 1 (cached)", cpuinfoReads)
		}

		if dmiReads != 1 {
			t.Fatalf("second Sample: DMI product_name read %d time(s) total, want still 1 (cached after first successful read)", dmiReads)
		}
	})

	t.Run("no_flags_no_dmi_virtualized_false", func(t *testing.T) {
		cpuinfoReads := 0
		dmiReads := 0
		procStat := procStatTick0

		fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case cpuStatPath:
				return []byte(cpuStatContent(1_000_000)), nil
			case procStatPath:
				return []byte(procStat), nil
			case procCpuinfoPath:
				cpuinfoReads++
				return []byte(arm64Cpuinfo), nil
			case dmiPath:
				dmiReads++
				// Bare-metal DMI product_name: no vendor token.
				return []byte("Raspberry Pi 4 Model B Rev 1.4\n"), nil
			default:
				return nil, errors.New("unexpected path read: " + path)
			}
		})

		s := cpuhealth.NewCgroupSampler(fs, basePath)

		sample, err := s.Sample(ctx)
		if err != nil {
			t.Fatalf("Sample: unexpected error: %v", err)
		}

		if sample.Virtualized {
			t.Fatalf("Virtualized: got true, want false (no hypervisor flag in cpuinfo, no vendor token in DMI product_name)")
		}

		if cpuinfoReads != 1 {
			t.Fatalf("/proc/cpuinfo read %d time(s), want 1", cpuinfoReads)
		}

		if dmiReads != 1 {
			t.Fatalf("DMI product_name read %d time(s), want 1 (fallback attempted when cpuinfo check did not prove virtualization)", dmiReads)
		}
	})
}

// TestCgroupSampler_ProcStatCounterReset pins the HostBusyCores counter-reset
// guard: when /proc/stat's total jiffies DECREASE between reads (host reboot,
// /proc/stat wrap), both StealFraction and HostBusyCores MUST be 0 (not
// negative). The reset sample becomes the new baseline, so the following tick
// produces a delta-relative value again.
func TestCgroupSampler_ProcStatCounterReset(t *testing.T) {
	const basePath = "/sys/fs/cgroup"
	cpuStatPath := basePath + "/cpu.stat"
	procStatPath := "/proc/stat"
	ctx := context.Background()

	// Tick 0: baseline. total = 1000+1000+1000+8000+0+0+0+50+0+0 = 11050.
	procStatTick0 := "cpu  1000 1000 1000 8000 0 0 0 50 0 0\n"
	// Tick 1: total grows to 22050 (delta 11000), a normal positive delta.
	procStatTick1 := "cpu  2000 1500 2000 16400 0 0 0 150 0 0\n"
	// Tick 2: total DROPS to 5000 (counter reset / host reboot). Without the
	// guard, busy - lastBusy goes negative → negative HostBusyCores published.
	procStatReset := "cpu  300 300 300 3900 0 0 0 10 0 0\n"

	usageUsec := int64(1_000_000)
	procStat := procStatTick0

	fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
		switch path {
		case cpuStatPath:
			return []byte(cpuStatContent(usageUsec)), nil
		case procStatPath:
			return []byte(procStat), nil
		default:
			return nil, errors.New("unexpected path read: " + path)
		}
	})

	s := cpuhealth.NewCgroupSampler(fs, basePath)

	// (1) Baseline.
	_, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("baseline Sample: unexpected error: %v", err)
	}

	// (2) Normal positive-delta tick: sanity (non-zero StealFraction + HostBusyCores).
	time.Sleep(40 * time.Millisecond)
	usageUsec = 1_100_000
	procStat = procStatTick1

	sample2, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("delta Sample: unexpected error: %v", err)
	}

	if sample2.StealFraction == 0 {
		t.Fatalf("delta Sample StealFraction: got 0, want non-zero (sanity: positive total delta)")
	}

	if sample2.HostBusyCores == 0 {
		t.Fatalf("delta Sample HostBusyCores: got 0, want non-zero (sanity: positive busy delta)")
	}

	// (3) Counter reset: total decreases. Both StealFraction and HostBusyCores
	// MUST be 0: a negative HostBusyCores would be published without the guard.
	time.Sleep(40 * time.Millisecond)
	usageUsec = 1_200_000
	procStat = procStatReset

	sample3, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("reset Sample: unexpected error: %v", err)
	}

	if sample3.StealFraction != 0 {
		t.Fatalf("reset Sample StealFraction: got %v, want 0 (total decreased → counter reset guard must zero BOTH steal and host-busy)",
			sample3.StealFraction)
	}

	if sample3.HostBusyCores != 0 {
		t.Fatalf("reset Sample HostBusyCores: got %v, want 0 (total decreased → busy delta negative → guard must zero, "+
			"not publish a negative core count)", sample3.HostBusyCores)
	}

	if sample3.HostBusyCores < 0 {
		t.Fatalf("reset Sample HostBusyCores: got negative %v (the exact bug the guard prevents)", sample3.HostBusyCores)
	}
}

// TestCgroupSampler_ProcStat_HostBusyCoresAvailableFalseOnBaselineAndReset
// pins the ring-poisoning fix (F3): readProcStat must NOT set
// HostBusyCoresAvailable=true on the first-baseline tick (no delta yet) nor on
// a counter-reset tick (re-baselining, no usable delta). The decide.go
// ring-append gate at :757 checks HostBusyCoresAvailable; if it is true on
// either path with HostBusyCores=0, a synthetic 0 is appended to the
// hostBusyRing, understating the 60s mean by ~50% for the warmup window.
// Available must be true ONLY on the steady-state delta path.
func TestCgroupSampler_ProcStat_HostBusyCoresAvailableFalseOnBaselineAndReset(t *testing.T) {
	const basePath = "/sys/fs/cgroup"
	cpuStatPath := basePath + "/cpu.stat"
	procStatPath := "/proc/stat"
	ctx := context.Background()

	// Tick 0: baseline. total = 11050.
	procStatTick0 := "cpu  1000 1000 1000 8000 0 0 0 50 0 0\n"
	// Tick 1: total grows to 22050 (delta 11000), a normal positive delta.
	procStatTick1 := "cpu  2000 1500 2000 16400 0 0 0 150 0 0\n"
	// Tick 2: total DROPS to 5000 (counter reset / host reboot).
	procStatReset := "cpu  300 300 300 3900 0 0 0 10 0 0\n"

	usageUsec := int64(1_000_000)
	procStat := procStatTick0

	fs := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
		switch path {
		case cpuStatPath:
			return []byte(cpuStatContent(usageUsec)), nil
		case procStatPath:
			return []byte(procStat), nil
		default:
			return nil, errors.New("unexpected path read: " + path)
		}
	})

	s := cpuhealth.NewCgroupSampler(fs, basePath)

	// (1) First call: baseline. No delta can be computed yet, so there is no
	// usable host-busy reading. HostBusyCoresAvailable MUST be false;
	// otherwise the decide.go ring-append gate appends HostBusyCores=0 and
	// poisons the 60s hostBusyMean.
	sample1, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("baseline Sample: unexpected error: %v", err)
	}
	if sample1.HostBusyCoresAvailable {
		t.Fatalf("baseline Sample HostBusyCoresAvailable: got true, want false (no delta on the first read; appending HostBusyCores=0 poisons the ring)")
	}

	// (2) Second call with a positive delta: the steady-state path. A real
	// reading was computed, so HostBusyCoresAvailable MUST be true and
	// HostBusyCores MUST be non-zero.
	time.Sleep(40 * time.Millisecond)
	usageUsec = 1_100_000
	procStat = procStatTick1

	sample2, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("delta Sample: unexpected error: %v", err)
	}
	if !sample2.HostBusyCoresAvailable {
		t.Fatalf("delta Sample HostBusyCoresAvailable: got false, want true (a real delta was computed on the steady-state path)")
	}
	if sample2.HostBusyCores == 0 {
		t.Fatalf("delta Sample HostBusyCores: got 0, want non-zero (steady-state path must still populate the reading)")
	}

	// (3) Third call with a counter reset (total decreases below baseline):
	// re-baselining, no usable delta. HostBusyCoresAvailable MUST be false;
	// otherwise the gate appends a synthetic 0 on every host reboot.
	time.Sleep(40 * time.Millisecond)
	usageUsec = 1_200_000
	procStat = procStatReset

	sample3, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("reset Sample: unexpected error: %v", err)
	}
	if sample3.HostBusyCoresAvailable {
		t.Fatalf("reset Sample HostBusyCoresAvailable: got true, want false (counter reset re-baselines; appending HostBusyCores=0 poisons the ring)")
	}
}
