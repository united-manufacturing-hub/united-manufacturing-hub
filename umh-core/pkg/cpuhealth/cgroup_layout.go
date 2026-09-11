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
	readQuota(ctx context.Context) (quotaRead, ReadOutcome)
	// readStat yields the usage total and both throttle counters. A non-nil
	// error says why there are none.
	readStat(ctx context.Context) (statRead, error)
	// readPSI reads this tick's pressure fraction as a 0..1 figure.
	readPSI(ctx context.Context) (frac float64, err error)
	// readCpuset counts the CPUs this cgroup may run on.
	readCpuset(ctx context.Context) (count int, err error)
	// advanceUsageRate advances the baseline to ts and returns this tick's rate
	// in cores.
	advanceUsageRate(ts time.Time, usage diagnosis.Reading) diagnosis.Reading
}

// v1CPUDirs are the directories a v1 cpu controller is mounted at, in the order
// probed. systemd mounts cpu and cpuacct together; a runtime may not.
var v1CPUDirs = []string{"cpu,cpuacct", "cpu"}

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

// resolveLayout reports the hierarchy under base, and for v1 the controller
// directory cpu.stat was found in. Where cpu.stat sits is the discriminant.
func resolveLayout(ctx context.Context, fs filesystem.Service, base string) (layout cgroupLayout, cpuDir string) {
	if fileExists(ctx, fs, base+"/cpu.stat") {
		return layoutV2, ""
	}

	for _, dir := range v1CPUDirs {
		if fileExists(ctx, fs, base+"/"+dir+"/cpu.stat") {
			return layoutV1, dir
		}
	}

	return layoutNone, ""
}

// fileExists counts a filesystem error as absent: a path that cannot be checked
// identifies no hierarchy.
func fileExists(ctx context.Context, fs filesystem.Service, path string) bool {
	exists, err := fs.FileExists(ctx, path)

	return err == nil && exists
}
