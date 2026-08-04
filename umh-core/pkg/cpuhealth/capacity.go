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
	"fmt"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Sampler reads a cgroup's CPU health signals.
type Sampler interface {
	Read(ctx context.Context) (Sample, error)
}

// Sample holds the CPU health readings for one cgroup.
type Sample struct {
	// Quota is present when cpu.max names a positive limit (the capacity in
	// cores), the literal "max" (a present 0.0, a definite no-limit), or a
	// non-positive numeric limit (a present 0.0, never a positive capacity).
	// It is absent no-signal when cpu.max is unreadable or unparsable.
	Quota diagnosis.Reading

	// Pressure is present when cpu.pressure's "some" line yields a readable
	// avg60 this tick (the kernel's 0..100 figure divided by 100 into the 0..1
	// fraction the marks are denominated in). It is absent when that read fails
	// this tick.
	Pressure diagnosis.Reading

	// PsiAvailable is sticky: it is set true on the first successful
	// cpu.pressure read and never cleared, even when a later read fails.
	PsiAvailable bool

	// NrPeriods and NrThrottled come from the SAME cpu.stat read that carries
	// usage. Each is present when its key is in cpu.stat and parses, and
	// unavailable (never a trusted 0) when the key is absent or unparsable.
	NrPeriods   diagnosis.Reading
	NrThrottled diagnosis.Reading
}

// NewCgroupSampler returns a Sampler reading via fs from base.
func NewCgroupSampler(fs filesystem.Service, base string) Sampler {
	return &cgroupSampler{fs: fs, base: base}
}

type cgroupSampler struct {
	fs           filesystem.Service
	base         string
	psiAvailable bool
}

// readPSI reads cpu.pressure's "some" avg60 as a 0..1 fraction. The second
// value reports whether a present Pressure Reading was produced this tick.
func (s *cgroupSampler) readPSI(ctx context.Context) (float64, bool) {
	data, err := s.fs.ReadFile(ctx, s.base+"/cpu.pressure")
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "some") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "avg60=") {
				v, err := strconv.ParseFloat(strings.TrimPrefix(field, "avg60="), 64)
				if err != nil {
					// An unparsable avg60 is no pressure this tick, matching the
					// unparsable cpu.max no-signal handling: never a present 0.0.
					return 0, false
				}
				return v / 100.0, true
			}
		}
	}
	return 0, false
}

// parseCounter reads one key's numeric value out of cpu.stat bytes. An absent
// key or an unparsable value yields an unavailable Reading — never a trusted 0.
func parseCounter(data []byte, key string) diagnosis.Reading {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			v, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return diagnosis.Unknown()
			}
			return diagnosis.Known(v)
		}
	}
	return diagnosis.Unknown()
}

// readThrottle reads cpu.stat once and yields both throttle counters. A non-nil
// error reports a READ failure, which fails the whole sample; a counter's
// Reading is independently present or unavailable on success.
func (s *cgroupSampler) readThrottle(ctx context.Context) (periods, throttled diagnosis.Reading, err error) {
	var data []byte
	data, err = s.fs.ReadFile(ctx, s.base+"/cpu.stat")
	if err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	return parseCounter(data, "nr_periods"), parseCounter(data, "nr_throttled"), nil
}

// Read samples the cgroup at base from cpu.max: a positive limit reads as a
// capacity, "max" and non-positive limits as a present no-limit, and an
// unreadable or unparsable cpu.max as absent no-signal.
func (s *cgroupSampler) Read(ctx context.Context) (Sample, error) {
	var smp Sample

	// cpu.pressure: PSI presence is sticky once seen; this tick's read success
	// is Pressure's own Reading, absent when the read fails this tick.
	if frac, ok := s.readPSI(ctx); ok {
		s.psiAvailable = true
		smp.Pressure = diagnosis.Known(frac)
	} else {
		smp.Pressure = diagnosis.Unknown()
	}
	smp.PsiAvailable = s.psiAvailable

	periods, throttled, statErr := s.readThrottle(ctx)
	if statErr != nil {
		// cpu.stat is primary: a read failure there fails the WHOLE sample,
		// never a silent drop of the throttle counters as absent no-signal.
		return smp, fmt.Errorf("read %s/cpu.stat: %w", s.base, statErr)
	}
	smp.NrPeriods = periods
	smp.NrThrottled = throttled

	data, err := s.fs.ReadFile(ctx, s.base+"/cpu.max")
	if err != nil {
		// An unreadable cpu.max is no-signal: Quota stays absent.
		return smp, nil
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return smp, nil
	}

	if fields[0] == "max" {
		// Uncapped is a definite no-limit: present, but never a positive capacity.
		smp.Quota = diagnosis.Known(0.0)
		return smp, nil
	}

	quota, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return smp, nil
	}
	period, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || period <= 0 {
		return smp, nil
	}

	if quota > 0 {
		smp.Quota = diagnosis.Known(float64(quota) / float64(period))
	} else {
		// A non-positive limit is never a positive capacity/denominator.
		smp.Quota = diagnosis.Known(0.0)
	}
	return smp, nil
}
