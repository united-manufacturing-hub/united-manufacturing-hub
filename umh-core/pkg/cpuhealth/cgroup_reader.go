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

// cgroupReader is the interface linuxSampler reads the cgroup through. It hides
// whether the host runs cgroup v1 or v2. The result types and parsers in this
// file are shared by both readers.

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
	// readQuota reads the cgroup's CPU limit in cores. A present 0 means no limit.
	readQuota(ctx context.Context) (quotaRead, ReadOutcome, error)
	// readStat reads the usage and throttle counters, with one readAttempt per file it opened.
	readStat(ctx context.Context) statRead
	// readPSI reads the cgroup's "some" avg60 CPU pressure as a 0..1 fraction.
	readPSI(ctx context.Context) (fraction float64, err error)
	// readCpuset counts the CPUs the cgroup can run on.
	readCpuset(ctx context.Context) (count int, err error)
	// advanceUsageRate returns the average cores in use between the previous tick and this one, and stores usage as the new baseline.
	advanceUsageRate(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading
	// pathOf returns the file this reader opens for operation, or "" when it has none.
	pathOf(operation ReadOperation) string
}

// readPaths maps each operation to the file cgroup opens for it, so a failure report names the file on this host.
func readPaths(cgroup cgroupReader) map[ReadOperation]string {
	paths := make(map[ReadOperation]string, len(allReadOperations))
	for _, spec := range allReadOperations {
		paths[spec.Operation] = cgroup.pathOf(spec.Operation)
	}

	return paths
}

// usageBaseline is the previous tick's usage total and the time it was read.
// have is false before the first successful read.
type usageBaseline struct {
	time  time.Time
	usage float64
	have  bool
}

// averageCoresOverLastInterval returns the average cores in use between the previous tick and this one, and stores usage as the new baseline.
func (b *usageBaseline) averageCoresOverLastInterval(timestamp time.Time, usage diagnosis.Reading) diagnosis.Reading {
	rate := diagnosis.Unknown()
	if b.have {
		// A usage total below the baseline means the counter was reset, so no
		// rate is returned and the new total becomes the baseline.
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

// quotaRead is one CPU limit read: the limit in cores, and the text it came from.
type quotaRead struct {
	Limit diagnosis.Reading

	// Raw is the text read, published as Sample.Troubleshooting.CPUMaxRaw for a
	// failure report. On v1 it is the quota, then the period when the period
	// file was read. It is set whenever the file was read, including when its
	// content did not parse.
	Raw string
}

// statRead is one read of the usage and throttle counters, and the cpu.stat text they came from.
type statRead struct {
	Usage     diagnosis.Reading
	Periods   diagnosis.Reading
	Throttled diagnosis.Reading

	// Raw is cpu.stat's text, published as Sample.Troubleshooting.CPUStatRaw for
	// a failure report. It is set whenever the file was read, including when its
	// content did not parse.
	Raw string

	// Reads holds one entry per file the reader tried to open for these readings.
	Reads []readAttempt
}

type readAttempt struct {
	Operation ReadOperation
	Outcome   ReadOutcome
	Err       error
}

// parseCounter returns one key's numeric value from cpu.stat bytes. An absent
// key returns an unavailable Reading, never a trusted 0. An unparsable value
// for a present key returns a non-nil error, which fails the whole sample.
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

// countCPUList counts the CPUs in a kernel CPU list, a comma-separated list of
// inclusive ranges and single ids such as "0-3", "0,2,4" or "0-1,4-5":
// https://docs.kernel.org/admin-guide/cgroup-v2.html#cpuset-interface-files
// Any entry that does not parse returns 0 and the error, never a partial count.
func countCPUList(list string) (count int, err error) {
	text := strings.TrimSpace(list)
	if text == "" {
		return 0, errEmptyRead
	}
	// The scheduler writes non-contiguous lists when it pins a pod to specific
	// CPUs, which is the case recordCPUScope reports as ScopeAffinity.
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
