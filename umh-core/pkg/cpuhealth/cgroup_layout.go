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

// The reader interface the sampler holds, and the readers behind it.
package cpuhealth

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cgroupReader is one cgroup's CPU accounting. An implementation owns the
// usage-rate baseline, the one fact that has to persist across ticks.
type cgroupReader interface {
	// readQuota reads the CPU limit in cores. See cgroupSource.readQuota for
	// what present, present-zero and absent each mean.
	readQuota(ctx context.Context) (quotaRead, ReadOutcome, error)
	// readStat yields the usage total and both throttle counters. A non-nil
	// error says why there are none.
	readStat(ctx context.Context) (statRead, error)
	// readPSI reads this tick's pressure fraction as a 0..1 figure.
	readPSI(ctx context.Context) (fraction float64, err error)
	// readCpuset counts the CPUs this cgroup may run on.
	readCpuset(ctx context.Context) (count int, err error)
	// advanceUsageRate advances the baseline to timestamp and returns this
	// tick's rate in cores.
	advanceUsageRate(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading
	// pathOf returns the file this reader opens for operation, or "" when it
	// opens none.
	pathOf(operation ReadOperation) string
}

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

// cgroupLayout is the hierarchy a mount turned out to be.
type cgroupLayout int

const (
	// layoutNone means neither shape matched. A box with no CPU accounting
	// reads this way, and so does one probed before its mount appeared, so it
	// is never final.
	layoutNone cgroupLayout = iota
	layoutV2
	layoutV1
)

func (l cgroupLayout) String() string {
	switch l {
	case layoutV2:
		return "v2"
	case layoutV1:
		return "v1"
	case layoutNone:
		return "unresolved"
	}

	return "unresolved"
}

func resolveLayout(ctx context.Context, fs filesystem.Service, base string) (cgroupLayout, v1Locations) {
	if fileExists(ctx, fs, base+"/cpu.stat") {
		return layoutV2, v1Locations{}
	}

	// A kernel built without CONFIG_CFS_BANDWIDTH writes no v1 cpu.stat, and
	// still writes cpuacct.usage.
	cpuDir, hasCPUStat := firstDirHolding(ctx, fs, base, v1CPUDirs, "cpu.stat")
	cpuacctDir, hasCPUAcctUsage := firstDirHolding(ctx, fs, base, v1CPUAcctDirs, "cpuacct.usage")
	if !hasCPUStat && !hasCPUAcctUsage {
		return layoutNone, v1Locations{}
	}

	return layoutV1, v1Locations{
		cpuDir:     cpuDir,
		cpuacctDir: cpuacctDir,
		cpusetFile: v1CpusetFile(ctx, fs, base),
	}
}

// firstDirHolding returns dirs[0] when no directory holds the file, so a later
// read of it fails as missing.
func firstDirHolding(ctx context.Context, fs filesystem.Service, base string, dirs []string, name string) (dir string, found bool) {
	for _, candidate := range dirs {
		if fileExists(ctx, fs, base+"/"+candidate+"/"+name) {
			return candidate, true
		}
	}

	return dirs[0], false
}

// v1CpusetFile prefers the set the kernel narrowed, which is the one tasks run on.
// https://docs.kernel.org/admin-guide/cgroup-v1/cpusets.html
func v1CpusetFile(ctx context.Context, fs filesystem.Service, base string) string {
	if fileExists(ctx, fs, base+"/cpuset/cpuset.effective_cpus") {
		return "cpuset.effective_cpus"
	}

	return "cpuset.cpus"
}

// fileExists counts a filesystem error as absent: a path that cannot be checked
// identifies no hierarchy.
func fileExists(ctx context.Context, fs filesystem.Service, path string) bool {
	exists, err := fs.FileExists(ctx, path)

	return err == nil && exists
}
