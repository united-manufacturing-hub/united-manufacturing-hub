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

// The container's CPU limit, read from the cgroup's cpu.max.

package cpuhealth

import (
	"context"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// readQuota reads cpu.max, the container's CPU limit: a positive limit reads as
// a capacity, the literal "max" or a non-positive limit reads as a present
// no-limit (a present 0.0), and an unreadable or unparsable cpu.max reads as
// absent no-signal.
func (s *linuxSampler) readQuota(ctx context.Context) diagnosis.Reading {
	data, err := s.fs.ReadFile(ctx, s.base+"/cpu.max")
	if err != nil {
		// An unreadable cpu.max is no-signal: Quota stays absent.
		return diagnosis.Unknown()
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return diagnosis.Unknown()
	}

	if fields[0] == "max" {
		// Uncapped is a definite no-limit: present, but never a positive capacity.
		return diagnosis.Known(0.0)
	}

	quota, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return diagnosis.Unknown()
	}
	period, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || period <= 0 {
		return diagnosis.Unknown()
	}

	if quota > 0 {
		return diagnosis.Known(float64(quota) / float64(period))
	}
	// A non-positive limit is never a positive capacity/denominator.
	return diagnosis.Known(0.0)
}
