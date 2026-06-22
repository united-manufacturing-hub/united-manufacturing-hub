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
	"bufio"
	"bytes"
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Sampler reads cgroup CPU usage observations.
//
// Not safe for concurrent use; Sample must be called from a single goroutine
// (the reconcile loop).
type Sampler interface {
	Sample(ctx context.Context) (Sample, error)
}

// cgroupSampler reads cgroup v2 cpu.stat usage_usec through the injected
// filesystem.Service and reports container-relative UsageCores as the counter
// delta over wall-clock.
type cgroupSampler struct {
	lastTime time.Time
	fs       filesystem.Service
	basePath string

	lastUsage   int64
	hasBaseline bool
}

// NewCgroupSampler returns a Sampler that reads cpu.stat from basePath.
func NewCgroupSampler(fs filesystem.Service, basePath string) Sampler {
	return &cgroupSampler{fs: fs, basePath: basePath}
}

// Sample reads cpu.stat, parses usage_usec, and returns a Sample. On the first
// read (or after a counter reset) it stores a baseline and reports
// UsageCores == 0. CgroupCores is left zero on every returned Sample; the
// caller populates it from cpu.max, because cpu.stat does not carry the quota.
func (s *cgroupSampler) Sample(ctx context.Context) (Sample, error) {
	data, err := s.fs.ReadFile(ctx, s.basePath+"/cpu.stat")
	if err != nil {
		return Sample{}, err
	}

	usage, err := parseUsageUsec(data)
	if err != nil {
		return Sample{}, err
	}

	now := time.Now()
	sample := Sample{Timestamp: now}

	if !s.hasBaseline || usage < s.lastUsage {
		s.lastUsage = usage
		s.lastTime = now
		s.hasBaseline = true

		return sample, nil
	}

	elapsed := now.Sub(s.lastTime).Seconds()
	if elapsed > 0 {
		sample.UsageCores = float64(usage-s.lastUsage) / 1e6 / elapsed
		s.lastUsage = usage
		s.lastTime = now
	}

	return sample, nil
}

// parseUsageUsec extracts the usage_usec field from a cgroup v2 cpu.stat body.
func parseUsageUsec(data []byte) (int64, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := bytes.Fields(sc.Bytes())
		if len(fields) >= 2 && bytes.Equal(fields[0], []byte("usage_usec")) {
			v, err := strconv.ParseInt(string(fields[1]), 10, 64)
			if err != nil {
				return 0, err
			}

			return v, nil
		}
	}

	if err := sc.Err(); err != nil {
		return 0, err
	}

	return 0, errors.New("cpu.stat: usage_usec not found")
}
