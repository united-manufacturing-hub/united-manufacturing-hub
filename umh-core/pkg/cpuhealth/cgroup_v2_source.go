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

// The cgroup v2 reader. hostSource reads the machine-wide files.

package cpuhealth

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// The files the v2 reader opens, all directly under the cgroup base.
// https://docs.kernel.org/admin-guide/cgroup-v2.html#cpu-interface-files
const (
	// "$QUOTA $PERIOD" in microseconds, with the quota "max" when uncapped.
	v2CPUMaxFile = "cpu.max"
	// usage_usec, nr_periods and nr_throttled.
	v2CPUStatFile = "cpu.stat"
	// The cgroup's own pressure stall information. v1 has no equivalent.
	v2CPUPressureFile = "cpu.pressure"
	// The CPUs the cgroup can run on.
	v2CpusetFile = "cpuset.cpus.effective"
)

// cgroupV2Source reads one cgroup's CPU accounting files, and keeps the usage
// baseline between ticks.
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

// advanceUsageRate returns the average cores in use between the previous tick and this one.
// timestamp must be the tick's single Timestamp, never time.Now(); Read in
// read.go says why.
func (c *cgroupV2Source) advanceUsageRate(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading {
	return c.usageBase.averageCoresOverLastInterval(timestamp, usage)
}

// readQuota reads cpu.max, the cgroup's CPU limit. The kernel writes the file
// as "$QUOTA $PERIOD" and puts the literal string "max" in the quota field when
// the cgroup is unlimited:
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#cpu-interface-files
//
// A positive quota returns the limit in cores. "max" and a non-positive quota
// return a present 0.0, meaning no limit, with ReadOK. An unreadable, empty or
// unparsable cpu.max returns an absent limit and the outcome naming the failure.
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
		// Uncapped reads as a present 0.0 (no limit), never a capacity.
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
	// A non-positive quota cannot be a capacity or a divisor, so it reads as no limit.
	return quotaRead{Limit: diagnosis.Known(0.0), Raw: raw}, ReadOK, nil
}

func (c *cgroupV2Source) readStat(ctx context.Context) statRead {
	stat, err := c.readStatFile(ctx)
	stat.Reads = []readAttempt{{Operation: OperationCPUStat, Outcome: statOutcome(stat, err), Err: err}}

	return stat
}

// statOutcome reports a successful read with no usage figure as ReadEmpty,
// since ReadOK would claim a value never produced. A zero-byte file and a
// usage_usec line with no value both return ReadEmpty; Raw tells them apart.
func statOutcome(stat statRead, err error) ReadOutcome {
	if err != nil {
		return classifyRead(err)
	}

	if _, ok := stat.Usage.Get(); !ok {
		return ReadEmpty
	}

	return ReadOK
}

// readStatFile reads cpu.stat once. A non-nil error means the read or a
// counter's parse failed. parseCounter says what an absent or unparsable key
// does to a single counter.
func (c *cgroupV2Source) readStatFile(ctx context.Context) (statRead, error) {
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

// readPSI reads cpu.pressure's "some" avg60 as a 0..1 fraction. On a non-nil
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
					// An unparsable avg60 leaves Pressure absent this tick, never a present 0.0.
					return 0, errUnparsableRead
				}
				return v / 100.0, nil
			}
		}
	}
	// The file was read but has no "some" line carrying avg60.
	return 0, errUnparsableRead
}

// readCpuset counts the CPUs in the cgroup's effective cpuset.
func (c *cgroupV2Source) readCpuset(ctx context.Context) (count int, err error) {
	data, err := c.fs.ReadFile(ctx, pathOf(c.base, OperationCpusetCPUs))
	if err != nil {
		return 0, err
	}

	return countCPUList(string(data))
}
