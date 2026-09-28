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
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

type cgroupV1Source struct {
	fs        filesystem.Service
	base      string
	locations v1Locations

	usageBase usageBaseline
}

func newCgroupV1Source(fs filesystem.Service, base string, locations v1Locations) *cgroupV1Source {
	return &cgroupV1Source{fs: fs, base: base, locations: locations}
}

// readQuota reads cpu.cfs_quota_us, the microseconds of CPU time allowed per
// period, over cpu.cfs_period_us. A positive quota reads as a capacity in
// cores, -1 as a present no-limit, and either file unreadable as absent.
func (c *cgroupV1Source) readQuota(ctx context.Context) (quotaRead, ReadOutcome, error) {
	quota, quotaRaw, err := c.readInt(ctx, c.path(c.locations.cpuDir, "cpu.cfs_quota_us"))
	if err != nil {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: quotaRaw}, classifyRead(err), err
	}

	periodPath := c.path(c.locations.cpuDir, "cpu.cfs_period_us")
	period, periodRaw, periodErr := c.readInt(ctx, periodPath)
	raw := quotaAndPeriodRaw(quotaRaw, periodRaw)

	if quota <= 0 {
		// -1 means uncapped: a definite no-limit, never a positive capacity.
		return quotaRead{Limit: diagnosis.Known(0.0), Raw: raw}, ReadOK, nil
	}

	if periodErr != nil {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(periodErr), periodErr
	}
	if period <= 0 {
		periodErr = contentError(periodPath, errUnparsableRead)

		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, ReadUnparsable, periodErr
	}

	return quotaRead{Limit: diagnosis.Known(float64(quota) / float64(period)), Raw: raw}, ReadOK, nil
}

func quotaAndPeriodRaw(quotaRaw, periodRaw string) string {
	if periodRaw == "" {
		return quotaRaw
	}

	return strings.TrimSpace(quotaRaw) + " " + strings.TrimSpace(periodRaw)
}

// readStat reads cpu.stat, which carries the same nr_periods and nr_throttled
// keys v2 does. The usage total comes from cpuacct.usage instead, so a cpu.stat
// that will not open still leaves usage readable.
func (c *cgroupV1Source) readStat(ctx context.Context) (statRead, error) {
	usage, usageErr := c.readUsage(ctx)
	failed := statRead{
		Usage:            usage,
		Periods:          diagnosis.Unknown(),
		Throttled:        diagnosis.Unknown(),
		UsageFromCPUAcct: true,
		UsageErr:         usageErr,
	}
	path := c.path(c.locations.cpuDir, "cpu.stat")

	data, err := c.fs.ReadFile(ctx, path)
	if err != nil {
		return failed, err
	}
	failed.Raw = string(data)

	periods, err := parseCounter(data, "nr_periods")
	if err != nil {
		return failed, contentError(path, err)
	}
	throttled, err := parseCounter(data, "nr_throttled")
	if err != nil {
		return failed, contentError(path, err)
	}

	read := failed
	read.Periods = periods
	read.Throttled = throttled

	return read, nil
}

// readUsage reads cpuacct.usage, which v1 writes in nanoseconds.
func (c *cgroupV1Source) readUsage(ctx context.Context) (diagnosis.Reading, error) {
	nanoseconds, _, err := c.readInt(ctx, c.path(c.locations.cpuacctDir, "cpuacct.usage"))
	if err != nil {
		return diagnosis.Unknown(), err
	}

	return diagnosis.Known(float64(nanoseconds) / 1e3), nil
}

func (c *cgroupV1Source) readPSI(context.Context) (fraction float64, err error) {
	return 0, errNoPressureFile
}

func (c *cgroupV1Source) readCpuset(ctx context.Context) (count int, err error) {
	path := c.path("cpuset", c.locations.cpusetFile)

	data, err := c.fs.ReadFile(ctx, path)
	if err != nil {
		return 0, err
	}

	count, err = countCPUList(string(data))
	if err != nil {
		return 0, contentError(path, err)
	}

	return count, nil
}

// advanceUsageRate derives this tick's usage rate in cores from the baseline it
// replaces, exactly as cgroupSource does.
func (c *cgroupV1Source) advanceUsageRate(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading {
	return c.usageBase.advance(timestamp, usage)
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

	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, raw, contentError(path, errEmptyRead)
	}

	value, err = strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, raw, contentError(path, errUnparsableRead)
	}

	return value, raw, nil
}

// contentError names the v1 file a content failure came from. Without it the
// report would name the v2 file PathOf maps the operation to.
func contentError(path string, readErr error) error {
	return &fs.PathError{Op: "read", Path: path, Err: readErr}
}
