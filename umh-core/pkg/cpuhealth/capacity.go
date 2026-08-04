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
	"time"

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

	// UsageUsec is the raw cumulative usage_usec counter from cpu.stat. It is
	// present when the key is in cpu.stat and parses, and unavailable when it
	// is absent or unparsable. The raw total is kept beside the rate so a later
	// throttle-ratio reduction still has the totals.
	UsageUsec diagnosis.Reading

	// UsageCores is the instantaneous usage rate in cores: the delta of
	// cumulative usage_usec across two consecutive reads divided by 1e6 (the
	// microsecond divisor) and by the elapsed seconds between the reads'
	// Timestamps. It is Unknown on the first read (no previous edge to subtract
	// from) and when usage_usec falls (a cumulative counter that falls has been
	// reset) — never a confident zero from no measurement.
	UsageCores diagnosis.Reading

	// HostBusy is the host's busy cores as an instantaneous rate: the delta of
	// the first aggregate "cpu " line's busy jiffies (user+nice+sys+irq+softirq)
	// between this read and the previous one, divided by USER_HZ into seconds
	// and by the interval's elapsed seconds. It is absent on the first read
	// (that read only fixes a baseline), when /proc/stat is unreadable or
	// unparsable, and when the busy counter falls (a host restart has reset it).
	HostBusy diagnosis.Reading

	// Steal is the host's CPU-steal fraction over the last poll interval: the
	// interval's steal-jiffy delta over the interval's total-jiffy (fields 0..7)
	// delta, read off the same first aggregate "cpu " line as HostBusy. It is
	// absent on the baseline read, when that line is unavailable like HostBusy,
	// when the steal counter falls (a reset), and when the denominator did not
	// advance (nothing measured, not a NaN/Inf reading).
	Steal diagnosis.Reading

	// Timestamp is the time of this read. Every field off the same read carries
	// the same Timestamp.
	Timestamp time.Time
}

// NewCgroupSampler returns a Sampler reading via fs from base.
func NewCgroupSampler(fs filesystem.Service, base string) Sampler {
	return &cgroupSampler{fs: fs, base: base}
}

type cgroupSampler struct {
	fs           filesystem.Service
	base         string
	psiAvailable bool

	// prevUsage and prevTime are the usage_usec edge and its read timestamp from
	// the previous Read, used to derive the instantaneous usage rate. haveUsage
	// reports whether the previous edge exists at all (false before the first
	// successful usage read).
	prevUsage float64
	prevTime  time.Time
	haveUsage bool

	// prevHostBusy, prevHostSteal and prevHostDenom are the raw jiffy totals of
	// the previous /proc/stat read, and prevHostTime its timestamp. These give
	// the interval edges from which the instantaneous host-busy rate and the
	// per-interval steal fraction are derived. haveHost reports whether a
	// previous Read has fixed the /proc/stat baseline; only a read after that
	// publishes host signals, and a falling counter (a host restart) re-baselines
	// instead of publishing a nonsense value.
	prevHostBusy  float64
	prevHostSteal float64
	prevHostDenom float64
	prevHostTime  time.Time
	haveHost      bool
}

// userHz matches the kernel's USER_HZ: the tick rate dividing /proc/stat jiffies
// into seconds, hence cores.
const userHz = 100.0

// readHost reads the first aggregate "cpu " line of /proc/stat and yields the
// raw busy, steal and denominator jiffy totals. busy counts user, nice, system,
// irq and softirq jiffies (idle, iowait, steal, guest and guest_nice excluded).
// The steal denominator is the sum of fields 0..7 only, since the kernel folds
// guest and guest_nice into user and nice. Both totals are kept raw so the
// caller can derive interval deltas. The trailing space in "cpu " keeps the
// aggregate line from matching cpu0/cpu1.
func (s *cgroupSampler) readHost(ctx context.Context) (busy, steal, denom float64, ok bool) {
	data, err := s.fs.ReadFile(ctx, "/proc/stat")
	if err != nil {
		return 0, 0, 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line) // fields[0] == "cpu"
		if len(fields) < 9 {
			return 0, 0, 0, false
		}
		vals := make([]float64, len(fields))
		for i := 1; i < len(fields); i++ {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				return 0, 0, 0, false
			}
			vals[i] = v
		}
		busy := vals[1] + vals[2] + vals[3] + vals[6] + vals[7]
		denom := vals[1] + vals[2] + vals[3] + vals[4] + vals[5] + vals[6] + vals[7] + vals[8]
		return busy, vals[8], denom, true
	}
	return 0, 0, 0, false
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

// readStat reads cpu.stat once and yields the raw usage total and both throttle
// counters. A non-nil error reports a READ failure, which fails the whole
// sample; each value's Reading is independently present or unavailable on
// success.
func (s *cgroupSampler) readStat(ctx context.Context) (usage, periods, throttled diagnosis.Reading, err error) {
	var data []byte
	data, err = s.fs.ReadFile(ctx, s.base+"/cpu.stat")
	if err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	return parseCounter(data, "usage_usec"),
		parseCounter(data, "nr_periods"),
		parseCounter(data, "nr_throttled"), nil
}

// Read samples the cgroup at base from cpu.max: a positive limit reads as a
// capacity, "max" and non-positive limits as a present no-limit, and an
// unreadable or unparsable cpu.max as absent no-signal.
func (s *cgroupSampler) Read(ctx context.Context) (Sample, error) {
	var smp Sample
	smp.Timestamp = time.Now()

	// cpu.pressure: PSI presence is sticky once seen; this tick's read success
	// is Pressure's own Reading, absent when the read fails this tick.
	if frac, ok := s.readPSI(ctx); ok {
		s.psiAvailable = true
		smp.Pressure = diagnosis.Known(frac)
	} else {
		smp.Pressure = diagnosis.Unknown()
	}
	smp.PsiAvailable = s.psiAvailable

	usage, periods, throttled, statErr := s.readStat(ctx)
	if statErr != nil {
		// cpu.stat is primary: a read failure there fails the WHOLE sample,
		// never a silent drop of the throttle counters as absent no-signal.
		return smp, fmt.Errorf("read %s/cpu.stat: %w", s.base, statErr)
	}
	smp.NrPeriods = periods
	smp.NrThrottled = throttled
	smp.UsageUsec = usage
	if s.haveUsage {
		// A rising cumulative counter over a positive elapsed time derives an
		// instantaneous rate; a falling one has been reset, so no rate.
		if u, ok := usage.Get(); ok && u >= s.prevUsage {
			if elapsed := smp.Timestamp.Sub(s.prevTime).Seconds(); elapsed > 0 {
				smp.UsageCores = diagnosis.Known((u - s.prevUsage) / 1e6 / elapsed)
			}
		}
	}
	if u, ok := usage.Get(); ok {
		s.prevUsage = u
		s.prevTime = smp.Timestamp
		s.haveUsage = true
	}

	// Host signals: the first /proc/stat read fixes a baseline and publishes
	// neither; a read after that publishes this tick's instantaneous host-busy
	// rate and per-interval steal fraction, each derived from the delta of two
	// consecutive reads. A falling cumulative counter (a host restart) is a
	// reset: the baseline is re-established and nothing is published this tick.
	if busy, steal, denom, ok := s.readHost(ctx); ok {
		if s.haveHost {
			// Busy cores over the interval is the busy-jiffy delta divided by
			// USER_HZ into seconds; the interval's elapsed time turns that into
			// a per-second rate. A falling busy counter (a reset) and a zero
			// elapsed time each publish nothing.
			if busy >= s.prevHostBusy {
				if elapsed := smp.Timestamp.Sub(s.prevHostTime).Seconds(); elapsed > 0 {
					smp.HostBusy = diagnosis.Known((busy - s.prevHostBusy) / userHz / elapsed)
				}
			}
			// The steal fraction is the interval's steal-jiffy delta over the
			// interval's total-jiffy delta. A falling steal counter (a reset)
			// and a non-positive denominator delta (proc/stat did not advance,
			// or is all zeros) publish nothing, never a NaN/Inf reading.
			dDenom := denom - s.prevHostDenom
			if steal >= s.prevHostSteal && dDenom > 0 {
				frac := (steal - s.prevHostSteal) / dDenom
				smp.Steal = diagnosis.Known(frac)
			}
		}
		s.prevHostBusy = busy
		s.prevHostSteal = steal
		s.prevHostDenom = denom
		s.prevHostTime = smp.Timestamp
		s.haveHost = true
	}

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
