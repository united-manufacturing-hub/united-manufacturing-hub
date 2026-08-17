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

// How many CPUs the cgroup is allowed to run on, counted from the kernel's
// cpuset list.

package cpuhealth

import (
	"context"
	"strconv"
	"strings"
)

// readCpuset counts the CPUs in the cgroup's effective cpuset, which the kernel
// writes as a comma-separated list of inclusive ranges and single ids: "0-3",
// "0,2,4", "0-1,4-5", documented at
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#cpuset-interface-files.
// An unreadable file, or any entry that does not parse, yields zero and false
// rather than a partial count.
func (s *linuxSampler) readCpuset(ctx context.Context) (count int, ok bool) {
	data, err := s.fs.ReadFile(ctx, s.base+"/cpuset.cpus.effective")
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, false
	}
	// Non-contiguous ranges are the shapes the scheduler emits when pinning a
	// pod to specific CPUs — the pinned-container case the scope check exists
	// for; count every id so any shape collapses to the allowed set's size.
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
