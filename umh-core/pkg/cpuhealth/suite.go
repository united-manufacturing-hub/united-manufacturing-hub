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

// Bind the generated suite to the real CPU table. pkg/diagnosis supplies Feed,
// Outcome and Run; this is the CPU half — a Readable snapshot in which every
// Reading is Known and well inside every mark, and an Unreadable one in which
// every Reading is Unknown(), so the six-scenario suite is generated from
// cpuTable itself and proves the readability path on the declaration that will
// actually run.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// cpuFeed is the CPU half of the generated suite: the two snapshots, built
// from outside pkg/diagnosis through Known and Unknown.
//
// Virtualized and CpuScope are startup facts, not per-tick reads, so they do
// not move between Readable and Unreadable — the capability gate is
// CaseUnsupported's subject, and driving it from the feed would test the feed
// instead of the table.
type cpuFeed struct{ cores, quota float64 }

// Readable returns a snapshot in which every source the table reads answers,
// with values well inside every mark: the suite proves the readability path,
// never the marks. The throttle counters are cumulative, so they are derived
// from at — NrPeriods climbs by 100 per elapsed second and NrThrottled holds a
// steady 0.02 ratio well under the 0.05 fire mark; a constant snapshot would
// give throttle-ratio a zero denominator delta and strand it below its floor
// forever, failing CaseLive on a build with nothing wrong with it. Quota comes
// from f.quota so the feed matches the table cpuTable(f.cores, f.quota) built
// for it, rather than a limit no scenario asked for. A known Pressure reading
// requires PsiAvailable true, per Sample's own sticky contract.
func (f cpuFeed) Readable(at time.Time) Sample {
	elapsed := at.Sub(time.Unix(0, 0)).Seconds()
	return Sample{
		Timestamp:    at,
		UsageUsec:    diagnosis.Known(0),
		UsageCores:   diagnosis.Known(0.1),
		NrPeriods:    diagnosis.Known(100 * elapsed),
		NrThrottled:  diagnosis.Known(0.02 * 100 * elapsed),
		Pressure:     diagnosis.Known(0),
		PsiAvailable: true,
		Steal:        diagnosis.Known(0),
		HostBusy:     diagnosis.Known(0.1),
		Quota:        diagnosis.Known(f.quota),
		LogicalCpus:  diagnosis.Known(f.cores),
		HostCpus:     diagnosis.Known(f.cores),
		CpuScope:     ScopeHost,
		Virtualized:  true,
	}
}

// Unreadable returns a snapshot in which every Reading is absent. Virtualized
// and CpuScope are startup facts and do not move. PsiAvailable stays true: this
// feed always models a box whose kernel reports PSI, so an outage tick must
// not un-latch it — Sample's own contract makes PsiAvailable sticky once set.
func (f cpuFeed) Unreadable(at time.Time) Sample {
	return Sample{Timestamp: at, CpuScope: ScopeHost, Virtualized: true, PsiAvailable: true}
}

// RunSuite drives the six-scenario suite generated from the CPU table itself,
// through diagnosis.Run. A sixth row that reports a Known value on a failed
// read instead of Unknown() reaches Ready on CaseLongOutage where AllAbsent is
// required, and there is no way to make that scenario green.
func RunSuite(cores, quota float64) []diagnosis.Outcome {
	t := cpuTable(cores, quota)
	return diagnosis.Run(t, suiteEnvironment(), cpuFeed{cores: cores, quota: quota})
}

// suiteEnvironment is the environment the suite runs every scenario in: every
// capability the CPU table's instruments require, so the suite exercises the
// whole table rather than the part this box happens to support.
//
// It is a named function rather than a literal inside RunSuite so the spec that
// checks it against the table's Requires reads the SAME value the suite runs
// on. A capability missing here does not fail loudly: the signal that requires
// it resolves NoInstrument in all six scenarios — skipped, not tested — and the
// asserted scenario count does not move, because outcomes are emitted per
// signal x case whatever they conclude.
func suiteEnvironment() diagnosis.Environment {
	return diagnosis.NewEnvironment(HasLimit, HasVirtualization, HasPressureStats)
}
