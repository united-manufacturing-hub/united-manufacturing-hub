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

// userHz matches the kernel's USER_HZ: the tick rate dividing /proc/stat jiffies
// into seconds, hence cores.
const userHz = 100.0

// readHost reads the first aggregate "cpu " line of /proc/stat and yields the
// raw busy, steal and denominator jiffy totals. busy counts user, nice, system,
// irq and softirq jiffies (idle, iowait, steal, guest and guest_nice excluded).
// The steal denominator is the sum of fields 0..7 only, since the kernel folds
// guest and guest_nice into user and nice. Both totals are kept raw so the
// caller can derive interval deltas. machine is the number of per-CPU (cpu0,
// cpu1, …) lines in the same file, the machine's CPU count. The trailing space
// in "cpu " keeps the aggregate line from matching cpu0/cpu1.
func (s *linuxSampler) readHost(ctx context.Context) (busy, steal, denom, machine float64, ok bool) {
	data, err := s.fs.ReadFile(ctx, "/proc/stat")
	if err != nil {
		return 0, 0, 0, 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		// A per-CPU line is "cpu" followed by a digit; the aggregate "cpu " line
		// (space, not digit) is not one of them.
		if len(line) > 3 && strings.HasPrefix(line, "cpu") && line[3] >= '0' && line[3] <= '9' {
			machine++
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line) // fields[0] == "cpu"
		if len(fields) < 9 {
			return 0, 0, 0, machine, false
		}
		vals := make([]float64, len(fields))
		for i := 1; i < len(fields); i++ {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				return 0, 0, 0, machine, false
			}
			vals[i] = v
		}
		busy := vals[1] + vals[2] + vals[3] + vals[6] + vals[7]
		denom := vals[1] + vals[2] + vals[3] + vals[4] + vals[5] + vals[6] + vals[7] + vals[8]
		return busy, vals[8], denom, machine, true
	}
	return 0, 0, 0, machine, false
}
