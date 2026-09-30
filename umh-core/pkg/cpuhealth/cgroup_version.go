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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

type cgroupVersion int

const (
	cgroupVersionUnresolved cgroupVersion = iota
	cgroupV2
	cgroupV1
)

func (v cgroupVersion) String() string {
	switch v {
	case cgroupV2:
		return "v2"
	case cgroupV1:
		return "v1"
	case cgroupVersionUnresolved:
		return "unresolved"
	}

	return "unresolved"
}

func detectCgroupVersion(ctx context.Context, fs filesystem.Service, base string) cgroupVersion {
	if fileExists(ctx, fs, base+"/cpu.stat") {
		return cgroupV2
	}

	// A kernel built without CONFIG_CFS_BANDWIDTH writes no v1 cpu.stat but
	// still writes cpuacct.usage: https://docs.kernel.org/scheduler/sched-bwc.html
	_, hasCPUStat := findDirContaining(ctx, fs, base, v1CPUDirs, "cpu.stat")
	_, hasCPUAcctUsage := findDirContaining(ctx, fs, base, v1CPUAcctDirs, "cpuacct.usage")
	if !hasCPUStat && !hasCPUAcctUsage {
		return cgroupVersionUnresolved
	}

	return cgroupV1
}

func findDirContaining(ctx context.Context, fs filesystem.Service, base string, dirs []string, file string) (dir string, found bool) {
	for _, candidate := range dirs {
		if fileExists(ctx, fs, base+"/"+candidate+"/"+file) {
			return candidate, true
		}
	}

	return "", false
}

func fileExists(ctx context.Context, fs filesystem.Service, path string) bool {
	exists, err := fs.FileExists(ctx, path)

	return err == nil && exists
}
