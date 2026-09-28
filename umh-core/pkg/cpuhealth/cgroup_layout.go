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
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

type cgroupReader interface {
	readQuota(ctx context.Context) (quotaRead, ReadOutcome, error)
	readStat(ctx context.Context) (statRead, error)
	readPSI(ctx context.Context) (fraction float64, err error)
	readCpuset(ctx context.Context) (count int, err error)
	advanceUsageRate(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading
	// pathOf returns "" for a read this reader has no file for.
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

type cgroupLayout int

const (
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

	// A kernel built without CONFIG_CFS_BANDWIDTH writes no v1 cpu.stat but
	// still writes cpuacct.usage: https://docs.kernel.org/scheduler/sched-bwc.html
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

// firstDirHolding falls back to dirs[0], so a later read of the file fails as missing.
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

func fileExists(ctx context.Context, fs filesystem.Service, path string) bool {
	exists, err := fs.FileExists(ctx, path)

	return err == nil && exists
}
