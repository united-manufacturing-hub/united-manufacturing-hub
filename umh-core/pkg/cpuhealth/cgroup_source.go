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

// The cgroup source: everything this package reads from under one cgroup's
// base — cpu.max, cpu.stat, cpu.pressure and cpuset.cpus.effective. Distinct
// from hostSource, which reads the machine-wide files that apply regardless
// of which cgroup is asking.

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

// cgroupSource reads one cgroup's CPU accounting files. It owns the two facts
// that persist across ticks for this cgroup — the usage-rate baseline and the
// sticky PSI-availability flag — so it is constructible and testable
// independently of hostSource.
type cgroupSource struct {
	fs   filesystem.Service
	base string

	usageBase usageBaseline

	// psiAvailable is sticky: set true on the first successful cpu.pressure
	// read and never cleared, even when a later read fails.
	psiAvailable bool
}

// newCgroupSource returns a cgroupSource reading via fs from base.
func newCgroupSource(fs filesystem.Service, base string) *cgroupSource {
	return &cgroupSource{fs: fs, base: base}
}

// usageBaseline is the previous tick's usage_usec edge, from which advanceUsageRate
// derives the instantaneous usage rate. have is false before the first
// successful read; a falling edge (a counter reset) re-baselines instead of
// publishing a nonsense rate.
type usageBaseline struct {
	time  time.Time
	usage float64
	have  bool
}

// advanceUsageRate advances the baseline this source owns to this tick, which is
// what the next tick measures against. It returns this tick's instantaneous usage
// rate: the delta of usage against the baseline it replaced, divided by the
// elapsed time since that baseline. ts is the composer's single per-tick
// Timestamp and never time.Now(); Read in read.go says why both sources have to
// divide by the same elapsed time.
func (c *cgroupSource) advanceUsageRate(ts time.Time, usage diagnosis.Reading) diagnosis.Reading {
	rate := diagnosis.Unknown()
	if c.usageBase.have {
		// A rising cumulative counter over a positive elapsed time derives an
		// instantaneous rate; a falling one has been reset, so no rate.
		if u, ok := usage.Get(); ok && u >= c.usageBase.usage {
			if elapsed := ts.Sub(c.usageBase.time).Seconds(); elapsed > 0 {
				rate = diagnosis.Known((u - c.usageBase.usage) / 1e6 / elapsed)
			}
		}
	}
	if u, ok := usage.Get(); ok {
		c.usageBase = usageBaseline{usage: u, time: ts, have: true}
	}
	return rate
}

// readQuota reads cpu.max, the container's CPU limit: a positive limit reads as
// a capacity, the literal "max" or a non-positive limit reads as a present
// no-limit (a present 0.0), and an unreadable or unparsable cpu.max reads as
// absent no-signal.
//
// The Reading carries presence, as it always did. The ReadOutcome carries the
// reason: an absent Reading says only that there is no quota to judge against,
// and the outcome says whether cpu.max was missing, unreadable or unparsable.
// A readable "max" is ReadOK — a present no-limit is not a failed read.
//
// The Reading travels inside a quotaRead, together with the text it was parsed
// out of, for the reason statRead gives: the two come from one open of one file
// and are never independently available.
func (c *cgroupSource) readQuota(ctx context.Context) (quotaRead, ReadOutcome) {
	data, err := c.fs.ReadFile(ctx, c.base+"/cpu.max")
	if err != nil {
		// An unreadable cpu.max is no-signal: Quota stays absent, and there are
		// no bytes to report.
		return quotaRead{Limit: diagnosis.Unknown()}, classifyRead(err)
	}
	raw := string(data)
	if strings.TrimSpace(raw) == "" {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(errEmptyRead)
	}

	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(errUnparsableRead)
	}

	if fields[0] == "max" {
		// Uncapped is a definite no-limit: present, but never a positive capacity.
		return quotaRead{Limit: diagnosis.Known(0.0), Raw: raw}, ReadOK
	}

	quota, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(errUnparsableRead)
	}
	period, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || period <= 0 {
		return quotaRead{Limit: diagnosis.Unknown(), Raw: raw}, classifyRead(errUnparsableRead)
	}

	if quota > 0 {
		return quotaRead{Limit: diagnosis.Known(float64(quota) / float64(period)), Raw: raw}, ReadOK
	}
	// A non-positive limit is never a positive capacity/denominator.
	return quotaRead{Limit: diagnosis.Known(0.0), Raw: raw}, ReadOK
}

// quotaRead is one cpu.max read: the CPU limit it yields and the text that
// limit was parsed out of.
type quotaRead struct {
	// Limit is the container's CPU limit in cores — see readQuota for when it
	// is present and what a present 0.0 means.
	Limit diagnosis.Reading

	// Raw is the file's text, verbatim. It is set whenever the read itself
	// succeeded, including when the parse then failed, since the text is what
	// says which content would not parse. It is the empty string when cpu.max
	// could not be read at all.
	Raw string
}

// statRead is one cpu.stat read: the three counters it yields and the text they
// were parsed out of. The four travel together because they come from a single
// open of a single file, and returning them separately made a caller thread
// four values that are never independently available.
type statRead struct {
	Usage     diagnosis.Reading
	Periods   diagnosis.Reading
	Throttled diagnosis.Reading

	// Raw is the file's text, verbatim. It is set whenever the read itself
	// succeeded, including when the parse then failed: the text is what says
	// which value was malformed, which the error's message alone does not.
	Raw string
}

// readStat reads cpu.stat once and yields the raw usage total, both throttle
// counters, and the text all three came from. A non-nil error reports a read OR
// parse failure of cpu.stat, either of which fails the whole sample; each
// value's Reading is independently present or unavailable on success.
func (c *cgroupSource) readStat(ctx context.Context) (statRead, error) {
	// What a failure yields: no counter is trusted, and the zero Reading is an
	// absent one.
	failed := statRead{Usage: diagnosis.Unknown(), Periods: diagnosis.Unknown(), Throttled: diagnosis.Unknown()}

	data, err := c.fs.ReadFile(ctx, c.base+"/cpu.stat")
	if err != nil {
		return failed, err
	}
	failed.Raw = string(data)

	usage, err := parseCounter(data, "usage_usec")
	if err != nil {
		return failed, err
	}
	periods, err := parseCounter(data, "nr_periods")
	if err != nil {
		return failed, err
	}
	throttled, err := parseCounter(data, "nr_throttled")
	if err != nil {
		return failed, err
	}

	return statRead{Usage: usage, Periods: periods, Throttled: throttled, Raw: string(data)}, nil
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

// readPSI reads cpu.pressure's "some" avg60 as a 0..1 fraction. A non-nil error
// is why there is no fraction this tick: the filesystem's own error where the
// file could not be read, and one of this package's two sentinels where the
// file read but held nothing usable.
func (c *cgroupSource) readPSI(ctx context.Context) (frac float64, err error) {
	data, err := c.fs.ReadFile(ctx, c.base+"/cpu.pressure")
	if err != nil {
		// Returned unwrapped: the caller classifies it, and wrapping would hide
		// the errno behind this package's own text.
		return 0, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return 0, errEmptyRead
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "some") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "avg60=") {
				v, parseErr := strconv.ParseFloat(strings.TrimPrefix(field, "avg60="), 64)
				if parseErr != nil {
					// An unparsable avg60 is no pressure this tick, matching the
					// unparsable cpu.max no-signal handling: never a present 0.0.
					return 0, errUnparsableRead
				}
				return v / 100.0, nil
			}
		}
	}
	// The file was there and neither a "some" line nor an avg60 field was: the
	// content is not the shape cpu.pressure is documented to have.
	return 0, errUnparsableRead
}

// readCpuset counts the CPUs in the cgroup's effective cpuset, which the kernel
// writes as a comma-separated list of inclusive ranges and single ids: "0-3",
// "0,2,4", "0-1,4-5", documented at
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#cpuset-interface-files.
// An unreadable file, or any entry that does not parse, yields zero and the
// reason rather than a partial count.
func (c *cgroupSource) readCpuset(ctx context.Context) (count int, err error) {
	data, err := c.fs.ReadFile(ctx, c.base+"/cpuset.cpus.effective")
	if err != nil {
		// Returned unwrapped: the caller classifies it, and wrapping would hide
		// the errno behind this package's own text.
		return 0, err
	}
	text := strings.TrimSpace(string(data))
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

// The three evidence readers. Each returns exactly what its source gave and
// the outcome of the read, and none of them parses, tokenises, trims to
// meaning, or judges: a raw string still says whether a failure is a shape
// nobody predicted, and a value derived from it would already have decided
// that question. Sample's raw fields say how an empty string reads.

// readControllers reads cgroup.controllers, the list of controllers the parent
// delegated to this cgroup. The token that matters is "cpuset", but which
// tokens matter is the reader's question, not this function's.
func (c *cgroupSource) readControllers(ctx context.Context) (string, ReadOutcome) {
	data, err := c.fs.ReadFile(ctx, c.base+"/cgroup.controllers")
	if err != nil {
		return "", classifyRead(err)
	}

	return string(data), ReadOK
}

// readProcSelfCgroup reads /proc/self/cgroup, the cgroup path this process is
// in. It is machine-wide by path and cgroup-specific by content, which is why
// it sits here: it is the fact that says whether base is the cgroup we are
// actually running in.
func (c *cgroupSource) readProcSelfCgroup(ctx context.Context) (string, ReadOutcome) {
	data, err := c.fs.ReadFile(ctx, "/proc/self/cgroup")
	if err != nil {
		return "", classifyRead(err)
	}

	return string(data), ReadOK
}

// countBaseEntries lists the cgroup base directory and keeps only how many
// entries it holds — a mounted cgroup v2 tree serves dozens of files, and a
// bind mount that served almost none is the shape worth seeing. The names
// themselves are not reported, since the count is what distinguishes those two.
// An unlistable directory yields -1: zero entries is a real reading, and
// Sample.BaseEntryCount says why the two must not look alike.
func (c *cgroupSource) countBaseEntries(ctx context.Context) (int, ReadOutcome) {
	entries, err := c.fs.ReadDir(ctx, c.base)
	if err != nil {
		return -1, classifyRead(err)
	}

	return len(entries), ReadOK
}
