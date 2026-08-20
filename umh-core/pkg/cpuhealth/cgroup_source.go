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

// The cgroup source: everything this package reads from under one cgroup's
// base — cpu.max, cpu.stat, cpu.pressure and cpuset.cpus.effective. Distinct
// from hostSource, which reads the machine-wide files that apply regardless
// of which cgroup is asking.

package cpuhealth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cgroupSource reads one cgroup's CPU accounting files. It owns the two facts
// that persist across ticks for this cgroup — the usage-rate baseline and the
// sticky PSI-availability flag — so it is constructible and testable
// independently of hostSource.
type cgroupSource struct {
	fs   filesystem.Service
	base string

	usageBase usageBaseline

	// psiAvailable is sticky: set true on the first successful cpu.pressure
	// read and never cleared, even when a later read fails.
	psiAvailable bool
}

// newCgroupSource returns a cgroupSource reading via fs from base.
func newCgroupSource(fs filesystem.Service, base string) *cgroupSource {
	return &cgroupSource{fs: fs, base: base}
}

// usageBaseline is the previous tick's usage_usec edge, from which usageRate
// derives the instantaneous usage rate. have is false before the first
// successful read; a falling edge (a counter reset) re-baselines instead of
// publishing a nonsense rate.
type usageBaseline struct {
	usage float64
	time  time.Time
	have  bool
}

// usageRate derives this tick's instantaneous usage rate — the delta of usage
// against the baseline this source owns, divided by the elapsed time since the
// baseline — then updates the baseline for the next tick. ts is the composer's
// single per-tick Timestamp and never time.Now(); Read in read.go says why both
// sources have to divide by the same elapsed time.
func (c *cgroupSource) usageRate(ts time.Time, usage diagnosis.Reading) diagnosis.Reading {
	rate := diagnosis.Unknown()
	if c.usageBase.have {
		// A rising cumulative counter over a positive elapsed time derives an
		// instantaneous rate; a falling one has been reset, so no rate.
		if u, ok := usage.Get(); ok && u >= c.usageBase.usage {
			if elapsed := ts.Sub(c.usageBase.time).Seconds(); elapsed > 0 {
				rate = diagnosis.Known((u - c.usageBase.usage) / 1e6 / elapsed)
			}
		}
	}
	if u, ok := usage.Get(); ok {
		c.usageBase = usageBaseline{usage: u, time: ts, have: true}
	}
	return rate
}

// readQuota reads cpu.max, the container's CPU limit: a positive limit reads as
// a capacity, the literal "max" or a non-positive limit reads as a present
// no-limit (a present 0.0), and an unreadable or unparsable cpu.max reads as
// absent no-signal.
func (c *cgroupSource) readQuota(ctx context.Context) diagnosis.Reading {
	data, err := c.fs.ReadFile(ctx, c.base+"/cpu.max")
	if err != nil {
		// An unreadable cpu.max is no-signal: Quota stays absent.
		return diagnosis.Unknown()
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return diagnosis.Unknown()
	}

	if fields[0] == "max" {
		// Uncapped is a definite no-limit: present, but never a positive capacity.
		return diagnosis.Known(0.0)
	}

	quota, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return diagnosis.Unknown()
	}
	period, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || period <= 0 {
		return diagnosis.Unknown()
	}

	if quota > 0 {
		return diagnosis.Known(float64(quota) / float64(period))
	}
	// A non-positive limit is never a positive capacity/denominator.
	return diagnosis.Known(0.0)
}

// readStat reads cpu.stat once and yields the raw usage total and both throttle
// counters. A non-nil error reports a read OR parse failure of cpu.stat, either
// of which fails the whole sample; each value's Reading is independently present
// or unavailable on success.
func (c *cgroupSource) readStat(ctx context.Context) (usage, periods, throttled diagnosis.Reading, err error) {
	var data []byte
	data, err = c.fs.ReadFile(ctx, c.base+"/cpu.stat")
	if err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if usage, err = parseCounter(data, "usage_usec"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if periods, err = parseCounter(data, "nr_periods"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if throttled, err = parseCounter(data, "nr_throttled"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	return usage, periods, throttled, nil
}

// parseCounter reads one key's numeric value out of cpu.stat bytes. An absent
// key yields an unavailable Reading — never a trusted 0 — while an unparsable
// value for a present key returns a non-nil error, which fails the whole sample.
func parseCounter(data []byte, key string) (diagnosis.Reading, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			v, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return diagnosis.Unknown(), fmt.Errorf("unparsable %s value %q: %w", key, fields[1], err)
			}
			return diagnosis.Known(v), nil
		}
	}
	return diagnosis.Unknown(), nil
}

// readPSI reads cpu.pressure's "some" avg60 as a 0..1 fraction.
func (c *cgroupSource) readPSI(ctx context.Context) (frac float64, ok bool) {
	data, err := c.fs.ReadFile(ctx, c.base+"/cpu.pressure")
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "some") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "avg60=") {
				v, err := strconv.ParseFloat(strings.TrimPrefix(field, "avg60="), 64)
				if err != nil {
					// An unparsable avg60 is no pressure this tick, matching the
					// unparsable cpu.max no-signal handling: never a present 0.0.
					return 0, false
				}
				return v / 100.0, true
			}
		}
	}
	return 0, false
}

// readCpuset counts the CPUs in the cgroup's effective cpuset, which the kernel
// writes as a comma-separated list of inclusive ranges and single ids: "0-3",
// "0,2,4", "0-1,4-5", documented at
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#cpuset-interface-files.
// An unreadable file, or any entry that does not parse, yields zero and false
// rather than a partial count.
func (c *cgroupSource) readCpuset(ctx context.Context) (count int, ok bool) {
	data, err := c.fs.ReadFile(ctx, c.base+"/cpuset.cpus.effective")
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, false
	}
	// Non-contiguous ranges are the shapes the scheduler emits when pinning a
	// pod to specific CPUs — the pinned-container case the scope check exists
	// for; count every id so any shape collapses to the allowed set's size.
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, false
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || hi < lo {
				return 0, false
			}
			count += hi - lo + 1
		} else {
			if _, err := strconv.Atoi(part); err != nil {
				return 0, false
			}
			count++
		}
	}
	return count, true
}
