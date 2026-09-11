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

// The same CPU accounting cgroupSource reads, as cgroup v1 writes it. Three
// things differ. The files sit under a directory per controller. The CPU limit
// is two files, and spells no-limit as -1 rather than "max". The usage total is
// nanoseconds in cpuacct.usage rather than microseconds in cpu.stat.
//
// v1 publishes no per-cgroup pressure file. The machine-wide
// /proc/pressure/cpu is not read in its place: it measures the whole machine,
// and Sample.Pressure is cgroup-scoped.
//
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v1/cpu.html and
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v1/cpuacct.html.
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

// cgroupV1Source reads one cgroup's CPU accounting from a v1 hierarchy. cpuDir
// is the controller directory cpu.stat was found in.
type cgroupV1Source struct {
	fs     filesystem.Service
	base   string
	cpuDir string

	usageBase usageBaseline
}

func newCgroupV1Source(fs filesystem.Service, base, cpuDir string) *cgroupV1Source {
	return &cgroupV1Source{fs: fs, base: base, cpuDir: cpuDir}
}

// readQuota reads cpu.cfs_quota_us, the microseconds of CPU time allowed per
// period, over cpu.cfs_period_us. A positive quota reads as a capacity in
// cores, -1 as a present no-limit, and either file unreadable as absent.
func (c *cgroupV1Source) readQuota(ctx context.Context) (quotaRead, ReadOutcome) {
	quota, raw, err := c.readInt(ctx, c.path(c.cpuDir, "cpu.cfs_quota_us"))
	if err != nil {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(err)
	}

	if quota <= 0 {
		// -1 means uncapped: a definite no-limit, never a positive capacity.
		return quotaRead{Limit: diagnosis.Known(0.0), Raw: raw}, ReadOK
	}

	period, _, err := c.readInt(ctx, c.path(c.cpuDir, "cpu.cfs_period_us"))
	if err != nil {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(err)
	}
	if period <= 0 {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, ReadUnparsable
	}

	return quotaRead{Limit: diagnosis.Known(float64(quota) / float64(period)), Raw: raw}, ReadOK
}

// readStat reads cpu.stat, which carries the same nr_periods and nr_throttled
// keys v2 does. The usage total comes from cpuacct.usage instead, so a cpu.stat
// that will not open still leaves usage readable.
func (c *cgroupV1Source) readStat(ctx context.Context) (statRead, error) {
	usage := c.readUsage(ctx)
	failed := statRead{Usage: usage, Periods: diagnosis.Unknown(), Throttled: diagnosis.Unknown()}
	path := c.path(c.cpuDir, "cpu.stat")

	data, err := c.fs.ReadFile(ctx, path)
	if err != nil {
		return failed, err
	}
	failed.Raw = string(data)

	periods, err := parseCounter(data, "nr_periods")
	if err != nil {
		return failed, fmt.Errorf("%s: %w", path, err)
	}
	throttled, err := parseCounter(data, "nr_throttled")
	if err != nil {
		return failed, fmt.Errorf("%s: %w", path, err)
	}

	return statRead{Usage: usage, Periods: periods, Throttled: throttled, Raw: string(data)}, nil
}

// readUsage reads cpuacct.usage, the cumulative CPU time used. v1 writes it in
// nanoseconds and Sample.UsageUsec is microseconds. Both mount points are
// tried, since a runtime may mount cpuacct on its own.
func (c *cgroupV1Source) readUsage(ctx context.Context) diagnosis.Reading {
	for _, dir := range []string{c.cpuDir, "cpuacct"} {
		nanoseconds, _, err := c.readInt(ctx, c.path(dir, "cpuacct.usage"))
		if err != nil {
			continue
		}

		return diagnosis.Known(float64(nanoseconds) / 1e3)
	}

	return diagnosis.Unknown()
}

// readPSI reports no pressure: v1 publishes no per-cgroup pressure file.
func (c *cgroupV1Source) readPSI(context.Context) (frac float64, err error) {
	return 0, errNoPressureFile
}

// readCpuset counts the CPUs in the cgroup's cpuset. Tasks run on the set the
// kernel narrowed, cpuset.effective_cpus, so it is read before the written
// cpuset.cpus.
func (c *cgroupV1Source) readCpuset(ctx context.Context) (count int, err error) {
	for _, name := range []string{"cpuset.effective_cpus", "cpuset.cpus"} {
		data, readErr := c.fs.ReadFile(ctx, c.path("cpuset", name))
		if readErr != nil {
			err = readErr

			continue
		}

		if count, err = countCPUList(string(data)); err == nil {
			return count, nil
		}
	}

	return 0, err
}

// advanceUsageRate derives this tick's usage rate in cores from the baseline it
// replaces, exactly as cgroupSource does.
func (c *cgroupV1Source) advanceUsageRate(ts time.Time, usage diagnosis.Reading) diagnosis.Reading {
	return c.usageBase.advance(ts, usage)
}

func (c *cgroupV1Source) path(dir, name string) string {
	return c.base + "/" + dir + "/" + name
}

// readInt reads one file holding a single integer, and returns its text so a
// value that will not parse is still reportable.
func (c *cgroupV1Source) readInt(ctx context.Context, path string) (value int64, raw string, err error) {
	data, err := c.fs.ReadFile(ctx, path)
	if err != nil {
		return 0, "", err
	}
	raw = string(data)

	value, err = strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, raw, errUnparsableRead
	}

	return value, raw, nil
}
