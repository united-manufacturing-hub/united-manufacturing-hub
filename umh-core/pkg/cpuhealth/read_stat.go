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

// The cgroup's own CPU accounting: usage and throttling, read from cpu.stat —
// distinct from the machine-wide totals read_host.go reads from /proc/stat.

package cpuhealth

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// readStat reads cpu.stat once and yields the raw usage total and both throttle
// counters. A non-nil error reports a read OR parse failure of cpu.stat, either
// of which fails the whole sample; each value's Reading is independently present
// or unavailable on success.
func (s *linuxSampler) readStat(ctx context.Context) (usage, periods, throttled diagnosis.Reading, err error) {
	var data []byte
	data, err = s.fs.ReadFile(ctx, s.base+"/cpu.stat")
	if err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if usage, err = parseCounter(data, "usage_usec"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if periods, err = parseCounter(data, "nr_periods"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if throttled, err = parseCounter(data, "nr_throttled"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	return usage, periods, throttled, nil
}

// parseCounter reads one key's numeric value out of cpu.stat bytes. An absent
// key yields an unavailable Reading — never a trusted 0 — while an unparsable
// value for a present key returns a non-nil error, which fails the whole sample.
func parseCounter(data []byte, key string) (diagnosis.Reading, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			v, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return diagnosis.Unknown(), fmt.Errorf("unparsable %s value %q: %w", key, fields[1], err)
			}
			return diagnosis.Known(v), nil
		}
	}
	return diagnosis.Unknown(), nil
}
