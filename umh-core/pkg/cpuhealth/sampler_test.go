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

package cpuhealth_test

import (
	"bufio"
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

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
//  3. The sampler reads ONLY <basePath>/cpu.stat via filesystem.Service --
//     every path handed to the mock is the cgroup cpu.stat path; a host
//     /proc/stat read would surface here as an unexpected path.
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

	// (3) The sampler read only <basePath>/cpu.stat via filesystem.Service --
	// no host /proc/stat, no other cgroup file. (Reported UsageCores matching
	// the mock cpuStatContent delta above is what proves the sampler did not
	// fall back to a gopsutil host read; a host-derived value would not track
	// the mock value.)
	for i, p := range pathsRead {
		if p != cpuStatPath {
			t.Fatalf("pathsRead[%d]: got %q, want %q (sampler must read only cpu.stat)", i, p, cpuStatPath)
		}
	}
	if len(pathsRead) != 2 {
		t.Fatalf("pathsRead length after two samples: got %d, want 2", len(pathsRead))
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
