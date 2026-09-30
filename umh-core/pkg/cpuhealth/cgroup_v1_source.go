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

// The cgroup v1 reader. v1 has no per-cgroup pressure file, and the
// machine-wide /proc/pressure/cpu is not read in its place, because
// Sample.Pressure is cgroup-scoped.
// https://docs.kernel.org/scheduler/sched-bwc.html
// https://docs.kernel.org/admin-guide/cgroup-v1/cpuacct.html

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

// The files the v1 reader opens. Each sits in the controller directory its
// comment names, under the cgroup base.
// https://docs.kernel.org/admin-guide/cgroup-v1/cpusets.html
const (
	// cpu: the CPU time the cgroup may use per period, in microseconds, or -1 when uncapped.
	v1CPUQuotaFile = "cpu.cfs_quota_us"
	// cpu: the length of one period, in microseconds.
	v1CPUPeriodFile = "cpu.cfs_period_us"
	// cpu: nr_periods and nr_throttled. Unlike v2's cpu.stat it carries no usage.
	v1CPUStatFile = "cpu.stat"
	// cpuacct: the CPU time the cgroup has used, in nanoseconds.
	v1CPUAcctUsageFile = "cpuacct.usage"
	// cpuset: the CPUs in use. It equals cpuset.cpus unless the hierarchy is mounted with cpuset_v2_mode.
	v1EffectiveCpusetFile = "cpuset.effective_cpus"
	// cpuset: the CPUs configured for the cgroup, read when cpuset.effective_cpus is absent.
	v1ConfiguredCpusetFile = "cpuset.cpus"

	v1CpusetDir = "cpuset"
)

// systemd mounts cpu and cpuacct together; a container runtime may not.
var (
	v1CPUDirs     = []string{"cpu,cpuacct", "cpu"}
	v1CPUAcctDirs = []string{"cpu,cpuacct", "cpuacct"}
)

type v1Locations struct {
	cpuDir     string
	cpuacctDir string
	cpusetFile string
}

func locateV1Files(ctx context.Context, fs filesystem.Service, base string) v1Locations {
	// A file the probe did not find is still read at its usual directory, so it reports as missing.
	cpuDir, found := findDirContaining(ctx, fs, base, v1CPUDirs, v1CPUStatFile)
	if !found {
		cpuDir = v1CPUDirs[0]
	}
	cpuacctDir, found := findDirContaining(ctx, fs, base, v1CPUAcctDirs, v1CPUAcctUsageFile)
	if !found {
		cpuacctDir = v1CPUAcctDirs[0]
	}

	return v1Locations{
		cpuDir:     cpuDir,
		cpuacctDir: cpuacctDir,
		cpusetFile: v1CpusetFileName(ctx, fs, base),
	}
}

func v1CpusetFileName(ctx context.Context, fs filesystem.Service, base string) string {
	if fileExists(ctx, fs, base+"/"+v1CpusetDir+"/"+v1EffectiveCpusetFile) {
		return v1EffectiveCpusetFile
	}

	return v1ConfiguredCpusetFile
}

type cgroupV1Source struct {
	fs        filesystem.Service
	base      string
	locations v1Locations

	usageBase usageBaseline
}

func newCgroupV1Source(fs filesystem.Service, base string, locations v1Locations) *cgroupV1Source {
	return &cgroupV1Source{fs: fs, base: base, locations: locations}
}

// readQuota divides the quota by the period, which v1 writes to two files.
func (c *cgroupV1Source) readQuota(ctx context.Context) (quotaRead, ReadOutcome, error) {
	quota, quotaRaw, err := c.readInt(ctx, c.pathOf(OperationCPUMax))
	if err != nil {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: quotaRaw}, classifyRead(err), err
	}

	periodPath := c.path(c.locations.cpuDir, v1CPUPeriodFile)
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

func (c *cgroupV1Source) readStat(ctx context.Context) statRead {
	usage, usageErr := c.readUsage(ctx)
	stat, statErr := c.readStatFile(ctx)
	stat.Usage = usage
	stat.Reads = []readAttempt{
		{Operation: OperationCPUAcctUsage, Outcome: classifyRead(usageErr), Err: usageErr},
		{Operation: OperationCPUStat, Outcome: classifyRead(statErr), Err: statErr},
	}

	return stat
}

func (c *cgroupV1Source) readStatFile(ctx context.Context) (statRead, error) {
	failed := statRead{Usage: diagnosis.Unknown(), Periods: diagnosis.Unknown(), Throttled: diagnosis.Unknown()}

	data, err := c.fs.ReadFile(ctx, c.pathOf(OperationCPUStat))
	if err != nil {
		return failed, err
	}
	failed.Raw = string(data)

	periods, err := parseCounter(data, "nr_periods")
	if err != nil {
		return failed, err
	}
	throttled, err := parseCounter(data, "nr_throttled")
	if err != nil {
		return failed, err
	}

	return statRead{Usage: diagnosis.Unknown(), Periods: periods, Throttled: throttled, Raw: string(data)}, nil
}

// readUsage reads cpuacct.usage, which v1 writes in nanoseconds.
func (c *cgroupV1Source) readUsage(ctx context.Context) (diagnosis.Reading, error) {
	nanoseconds, _, err := c.readInt(ctx, c.pathOf(OperationCPUAcctUsage))
	if err != nil {
		return diagnosis.Unknown(), err
	}

	return diagnosis.Known(float64(nanoseconds) / 1e3), nil
}

func (c *cgroupV1Source) readPSI(context.Context) (fraction float64, err error) {
	return 0, errNoPressureFile
}

func (c *cgroupV1Source) readCpuset(ctx context.Context) (count int, err error) {
	data, err := c.fs.ReadFile(ctx, c.pathOf(OperationCpusetCPUs))
	if err != nil {
		return 0, err
	}

	return countCPUList(string(data))
}

func (c *cgroupV1Source) advanceUsageRate(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading {
	return c.usageBase.advance(timestamp, usage)
}

func (c *cgroupV1Source) pathOf(operation ReadOperation) string {
	switch operation {
	case OperationCPUMax:
		return c.path(c.locations.cpuDir, v1CPUQuotaFile)
	case OperationCPUStat:
		return c.path(c.locations.cpuDir, v1CPUStatFile)
	case OperationCPUAcctUsage:
		return c.path(c.locations.cpuacctDir, v1CPUAcctUsageFile)
	case OperationCpusetCPUs:
		return c.path(v1CpusetDir, c.locations.cpusetFile)
	case OperationCPUPressure:
		return ""
	}

	return pathOf(c.base, operation)
}

func (c *cgroupV1Source) path(dir, name string) string {
	return c.base + "/" + dir + "/" + name
}

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

// contentError names the period file, which shares OperationCPUMax with the quota file.
func contentError(path string, readErr error) error {
	return &fs.PathError{Op: "read", Path: path, Err: readErr}
}
