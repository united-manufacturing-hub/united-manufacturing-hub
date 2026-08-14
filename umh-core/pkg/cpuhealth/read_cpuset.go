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
	"strconv"
	"strings"
)

// readCpuset counts the CPUs in the cgroup's effective cpuset when it is a
// single inclusive range such as "0-3". The second value reports whether the
// set was readable and parsed as such a range.
func (s *cgroupSampler) readCpuset(ctx context.Context) (int, bool) {
	data, err := s.fs.ReadFile(ctx, s.base+"/cpuset.cpus.effective")
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, false
	}
	// The cpuset is a comma-separated list of ranges and single ids — "0-3",
	// "0,2,4", "0-1,4-5" — the shapes the scheduler emits when it pins a pod
	// to non-contiguous CPUs, which is F6's primary target. Count every id it
	// names, so any shape collapses to the size of the allowed set.
	var count int
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
