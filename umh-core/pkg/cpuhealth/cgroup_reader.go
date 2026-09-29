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

// The interface both cgroup readers implement, and the result types and
// parsers they share.

package cpuhealth

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
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

func readPaths(cgroup cgroupReader) map[ReadOperation]string {
	paths := make(map[ReadOperation]string, len(allReadOperations))
	for _, spec := range allReadOperations {
		paths[spec.Operation] = cgroup.pathOf(spec.Operation)
	}

	return paths
}

// usageBaseline is the previous tick's usage total, from which advanceUsageRate
// derives the instantaneous usage rate. have is false before the first
// successful read; a falling edge (a counter reset) re-baselines instead of
// publishing a nonsense rate.
type usageBaseline struct {
	time  time.Time
	usage float64
	have  bool
}

func (b *usageBaseline) advance(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading {
	rate := diagnosis.Unknown()
	if b.have {
		// A rising cumulative counter over a positive elapsed time derives an
		// instantaneous rate; a falling one has been reset, so no rate.
		if u, ok := usage.Get(); ok && u >= b.usage {
			if elapsed := timestamp.Sub(b.time).Seconds(); elapsed > 0 {
				rate = diagnosis.Known((u - b.usage) / 1e6 / elapsed)
			}
		}
	}
	if u, ok := usage.Get(); ok {
		*b = usageBaseline{usage: u, time: timestamp, have: true}
	}

	return rate
}

// quotaRead is one cpu.max read: the limit in cores, and the text it came from.
type quotaRead struct {
	Limit diagnosis.Reading

	// Raw is the file's text, kept for a failure report and published as
	// Sample.Troubleshooting.CPUMaxRaw. It is set whenever the read succeeded,
	// a failed parse included, so a report can show the text that would not
	// parse.
	Raw string
}

// statRead is one cpu.stat read: the counters, and the text they came from.
type statRead struct {
	Usage     diagnosis.Reading
	Periods   diagnosis.Reading
	Throttled diagnosis.Reading

	// Raw is the file's text, kept for a failure report and published as
	// Sample.Troubleshooting.CPUStatRaw. It is set whenever the read succeeded,
	// a failed parse included.
	Raw string

	// Set only on v1, where usage is a separate cpuacct.usage read.
	UsageFromCPUAcct bool
	UsageErr         error
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
				return diagnosis.Unknown(), fmt.Errorf("%w: %s value %q: %w", errUnparsableRead, key, fields[1], err)
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return diagnosis.Unknown(), fmt.Errorf("%w: %s value %q is non-finite", errUnparsableRead, key, fields[1])
			}
			return diagnosis.Known(v), nil
		}
	}
	return diagnosis.Unknown(), nil
}

func countCPUList(list string) (count int, err error) {
	text := strings.TrimSpace(list)
	if text == "" {
		return 0, errEmptyRead
	}
	// Non-contiguous ranges are the shapes the scheduler emits when pinning a
	// pod to specific CPUs — the pinned-container case the scope check exists
	// for; count every id so any shape collapses to the allowed set's size.
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, errUnparsableRead
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || hi < lo {
				return 0, errUnparsableRead
			}
			count += hi - lo + 1
		} else {
			if _, atoiErr := strconv.Atoi(part); atoiErr != nil {
				return 0, errUnparsableRead
			}
			count++
		}
	}
	return count, nil
}
