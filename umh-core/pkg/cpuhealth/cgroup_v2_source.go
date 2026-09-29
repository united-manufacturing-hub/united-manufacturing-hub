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

// The cgroup v2 reader: cpu.max, cpu.stat, cpu.pressure and
// cpuset.cpus.effective under one cgroup's base. hostSource reads the
// machine-wide files.

package cpuhealth

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cgroupV2Source reads one cgroup's CPU accounting files, and owns the facts
// that persist across ticks for this cgroup.
type cgroupV2Source struct {
	fs   filesystem.Service
	base string

	usageBase usageBaseline
}

func (c *cgroupV2Source) pathOf(operation ReadOperation) string {
	return pathOf(c.base, operation)
}

// newCgroupV2Source returns a cgroupV2Source reading via fs from base.
func newCgroupV2Source(fs filesystem.Service, base string) *cgroupV2Source {
	return &cgroupV2Source{fs: fs, base: base}
}

// advanceUsageRate advances the baseline this source owns to this tick, which is
// what the next tick measures against. It returns this tick's instantaneous usage
// rate: the delta of usage against the baseline it replaced, divided by the
// elapsed time since that baseline. timestamp is the composer's single per-tick
// Timestamp and never time.Now(); Read in read.go says why both sources have to
// divide by the same elapsed time.
func (c *cgroupV2Source) advanceUsageRate(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading {
	return c.usageBase.advance(timestamp, usage)
}

// as "$QUOTA $PERIOD" and puts the literal string "max" in the quota field when
// the cgroup is unlimited:
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#cpu-interface-files
//
// A positive quota reads as a capacity in cores. "max" and a non-positive quota
// read as a present no-limit, a present 0.0, and both return ReadOK. A cpu.max
// that is unreadable, empty or unparsable reads as absent no-signal, under the
// outcome that says which.
func (c *cgroupV2Source) readQuota(ctx context.Context) (quotaRead, ReadOutcome, error) {
	data, err := c.fs.ReadFile(ctx, pathOf(c.base, OperationCPUMax))
	if err != nil {
		return quotaRead{Limit: diagnosis.Unknown()}, classifyRead(err), err
	}
	raw := string(data)
	if strings.TrimSpace(raw) == "" {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(errEmptyRead), errEmptyRead
	}

	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(errUnparsableRead), errUnparsableRead
	}

	if fields[0] == "max" {
		// Uncapped is a definite no-limit: present, but never a positive capacity.
		return quotaRead{Limit: diagnosis.Known(0.0), Raw: raw}, ReadOK, nil
	}

	quota, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(errUnparsableRead), errUnparsableRead
	}
	period, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || period <= 0 {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(errUnparsableRead), errUnparsableRead
	}

	if quota > 0 {
		return quotaRead{Limit: diagnosis.Known(float64(quota) / float64(period)), Raw: raw}, ReadOK, nil
	}
	// A non-positive limit is never a positive capacity/denominator.
	return quotaRead{Limit: diagnosis.Known(0.0), Raw: raw}, ReadOK, nil
}

// readStat reads cpu.stat once. A non-nil error means either the read or a
// counter's parse failed. linuxSampler.Read turns that into the whole tick's
// error, and parseCounter says what an absent or unparsable key does to a
// single counter.
func (c *cgroupV2Source) readStat(ctx context.Context) (statRead, error) {
	failed := statRead{Usage: diagnosis.Unknown(), Periods: diagnosis.Unknown(), Throttled: diagnosis.Unknown()}

	data, err := c.fs.ReadFile(ctx, pathOf(c.base, OperationCPUStat))
	if err != nil {
		return failed, err
	}
	failed.Raw = string(data)

	usage, err := parseCounter(data, "usage_usec")
	if err != nil {
		return failed, err
	}
	periods, err := parseCounter(data, "nr_periods")
	if err != nil {
		return failed, err
	}
	throttled, err := parseCounter(data, "nr_throttled")
	if err != nil {
		return failed, err
	}

	return statRead{Usage: usage, Periods: periods, Throttled: throttled, Raw: string(data)}, nil
}

// error no fraction was read and fraction is 0, which is not a measured zero.
func (c *cgroupV2Source) readPSI(ctx context.Context) (fraction float64, err error) {
	data, err := c.fs.ReadFile(ctx, pathOf(c.base, OperationCPUPressure))
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return 0, errEmptyRead
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "some") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "avg60=") {
				v, parseErr := strconv.ParseFloat(strings.TrimPrefix(field, "avg60="), 64)
				if parseErr != nil {
					// An unparsable avg60 is no pressure this tick, matching the
					// unparsable cpu.max no-signal handling: never a present 0.0.
					return 0, errUnparsableRead
				}
				return v / 100.0, nil
			}
		}
	}
	// The file was there; its documented "some"/avg60 shape was not.
	return 0, errUnparsableRead
}

// readCpuset counts the CPUs in the cgroup's effective cpuset, which the kernel
// writes as a comma-separated list of inclusive ranges and single ids: "0-3",
// "0,2,4", "0-1,4-5", documented at
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#cpuset-interface-files.
// An unreadable file, or any entry that does not parse, yields zero and the
// reason rather than a partial count.
func (c *cgroupV2Source) readCpuset(ctx context.Context) (count int, err error) {
	data, err := c.fs.ReadFile(ctx, pathOf(c.base, OperationCpusetCPUs))
	if err != nil {
		return 0, err
	}

	return countCPUList(string(data))
}
