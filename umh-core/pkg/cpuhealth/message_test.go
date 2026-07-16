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

//go:build test

// Run with -tags=test: this file is excluded from plain go test, which
// reports green with 0 specs. CI passes -tags=test for this package.
package cpuhealth_test

import (
	"strings"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
)

// TestComposeMessage_ThrottleTwoLayerC2 pins the two-layer contract:
// ComposeMessage is a pure function turning a Verdict + Signals into the
// two-layer format the Console alert adapter renders (first line = headline,
// everything after a literal "Technical Details:" separator collapses into
// the expandable panel). A degraded message MUST carry the separator: without
// it the whole message lands in the collapsed panel and the headline goes
// blank. The throttle cause composes headline "CPU limited" plus the curated
// Technical-Details copy (why + live number + what-to-do), NOT the generic
// "CPU degraded".
func TestComposeMessage_ThrottleTwoLayerC2(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindThrottling, Value: 0.12},
		},
	}
	signals := cpuhealth.Signals{
		ThrottleRatio: 0.12,
		ThrottleFired: true,
	}

	msg := cpuhealth.ComposeMessage(verdict, signals)

	if msg == "" {
		t.Fatalf("ComposeMessage returned empty string for a degraded throttle verdict: every degraded message must render a non-blank headline")
	}

	if msg == "CPU degraded" {
		t.Fatalf("ComposeMessage returned the generic %q for a throttle verdict: must return the curated per-cause copy from the supertable", msg)
	}

	// (1) The first line is the one-line headline naming the dominant cause.
	firstLine, rest, found := strings.Cut(msg, "\n")
	if !found {
		t.Fatalf("ComposeMessage returned a single line with no separator: %q (must be two-layer: headline + Technical Details)", msg)
	}
	if firstLine != "CPU limited" {
		t.Fatalf("headline (first line): got %q, want %q (the throttle-cause headline from the supertable)", firstLine, "CPU limited")
	}

	// (2) The literal "Technical Details:" separator MUST be present, the
	// Console adapter splits on it; a degraded message without it renders with a
	// blank headline.
	sepLine, details, sepFound := strings.Cut(rest, "Technical Details:")
	if !sepFound {
		t.Fatalf("message is missing the literal %q separator (C2 hard constraint): full message=%q", "Technical Details:", msg)
	}
	_ = sepLine

	// (3) The Technical Details carry the throttle-specific curated copy (why +
	// the live throttle number + what-to-do), not generic wording.
	details = strings.TrimSpace(details)
	if details == "" {
		t.Fatalf("Technical Details section is empty: full message=%q", msg)
	}
	for _, want := range []string{
		"paused until the next cycle",
		"CPU scheduling periods",
		"Raise this instance",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("Technical Details missing throttle-specific copy %q: details=%q", want, details)
		}
	}

	// The live number (12% throttle ratio) must appear in the Technical Details.
	if !strings.Contains(details, "12%") {
		t.Fatalf("Technical Details missing the live throttle number 12%%: details=%q", details)
	}
}

// assertTwoLayer enforces the hard constraints shared by every degraded
// cause: a non-blank headline on the first line, the literal "Technical
// Details:" separator, and a non-empty details section containing the given
// key phrases. It returns the details body for cause-specific follow-up checks.
func assertTwoLayer(t *testing.T, msg, wantHeadline string, wantPhrases ...string) string {
	t.Helper()

	if msg == "" {
		t.Fatalf("ComposeMessage returned empty string: every degraded message must render a non-blank headline")
	}
	if msg == "CPU degraded" {
		t.Fatalf("ComposeMessage returned the generic %q: must return the curated per-cause copy from the supertable", msg)
	}

	firstLine, rest, found := strings.Cut(msg, "\n")
	if !found {
		t.Fatalf("ComposeMessage returned a single line with no separator: %q (must be two-layer: headline + Technical Details)", msg)
	}
	if firstLine != wantHeadline {
		t.Fatalf("headline (first line): got %q, want %q", firstLine, wantHeadline)
	}

	sepLine, details, sepFound := strings.Cut(rest, "Technical Details:")
	if !sepFound {
		t.Fatalf("message is missing the literal %q separator (C2 hard constraint): full message=%q", "Technical Details:", msg)
	}
	_ = sepLine

	details = strings.TrimSpace(details)
	if details == "" {
		t.Fatalf("Technical Details section is empty: full message=%q", msg)
	}

	for _, want := range wantPhrases {
		if !strings.Contains(details, want) {
			t.Fatalf("Technical Details missing phrase %q: details=%q", want, details)
		}
	}

	return details
}

func TestComposeMessage_PressureTwoLayerC2(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindPressure, Value: 0.23},
		},
	}
	signals := cpuhealth.Signals{PressureFired: true}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	details := assertTwoLayer(t, msg, "CPU contention",
		"waiting for a free CPU core",
		"Reduce the load on this instance",
	)

	if !strings.Contains(details, "23%") {
		t.Fatalf("Technical Details missing the live pressure number 23%%: details=%q", details)
	}
}

func TestComposeMessage_StealTwoLayerC2(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionHost,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSteal, Value: 0.15},
		},
	}
	signals := cpuhealth.Signals{StealFired: true}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	details := assertTwoLayer(t, msg, "CPU taken by the server",
		"Other virtual machines on the same physical server",
		"give this VM more guaranteed CPU",
	)

	if !strings.Contains(details, "15%") {
		t.Fatalf("Technical Details missing the live steal number 15%%: details=%q", details)
	}
}

// NOTE: there is deliberately no host-contention message test:
// CauseKindHostContention is never emitted (Decide folds a neighbor filling
// the box into saturation + the host/container attribution split), so
// causeHeadline/causeDetails carry no copy for it, and a test asserting such
// strings would pin dead code.

func TestComposeMessage_SaturationTwoLayerC2(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0.82},
		},
	}
	// NoHostStatsSaturationFired true selects the no-host-stats saturation (no-limit, no-host-stats) copy this
	// test's phrases assert. The host-headroom variant is pinned separately in
	// TestComposeMessage_SaturationDetailsPsiConditional.
	signals := cpuhealth.Signals{SaturationFired: true, NoHostStatsSaturationFired: true, LimitedVisibility: true}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	details := assertTwoLayer(t, msg, "CPU running near full",
		"of the machine",
		"Host contention is not visible",
		"host CPU usage is not readable",
		"psi=1",
	)

	if !strings.Contains(details, "82%") {
		t.Fatalf("Technical Details missing the live saturation number 82%%: details=%q", details)
	}
}

// TestComposeMessage_SaturationDetailsPsiConditional pins the saturation
// Technical-Details copy split on signals.LimitedVisibility. When blind (no
// limit, no PSI) the copy names the missing signals and remediation; when NOT
// blind (a PSI-only box, LimitedVisibility false) it must NOT claim PSI is
// missing, it gives the actionable capacity/limit advice instead.
func TestComposeMessage_SaturationDetailsPsiConditional(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0.82},
		},
	}

	t.Run("Blind", func(t *testing.T) {
		// no-host-stats saturation: no limit, no host stats, no PSI. The detail carries the
		// "Host contention is not visible" caveat and psi=1 guidance.
		signals := cpuhealth.Signals{SaturationFired: true, NoHostStatsSaturationFired: true, LimitedVisibility: true}
		msg := cpuhealth.ComposeMessage(verdict, signals)
		want := "CPU running near full\nTechnical Details: CPU averaged 82% of the machine over the last minute and this instance has little headroom left. Host contention is not visible here (host CPU usage is not readable). Enable Linux pressure stats (boot with psi=1) for richer detail. Consider adding CPU capacity."
		if msg != want {
			t.Fatalf("blind saturation message:\n got: %q\nwant: %q", msg, want)
		}
	})

	// No-limit, host-stats-readable, PSI present: the no-limit host-headroom
	// copy. It must NOT claim PSI is missing or carry the no-host-stats saturation caveat.
	t.Run("NonBlindPsiPresent", func(t *testing.T) {
		signals := cpuhealth.Signals{
			SaturationFired:      true,
			HostBusyCores60sMean: 6.56,
			CapacityCores:        8.0,
			LimitedVisibility:    false,
		}
		msg := cpuhealth.ComposeMessage(verdict, signals)
		want := "CPU running near full\nTechnical Details: CPU averaged 82% of the machine over the last minute and this instance has little headroom left. Add CPU capacity, or reduce the load on it."
		if msg != want {
			t.Fatalf("non-blind saturation message:\n got: %q\nwant: %q", msg, want)
		}
		if strings.Contains(msg, "pressure stats") || strings.Contains(msg, "psi=1") {
			t.Fatalf("non-blind saturation copy must not claim PSI is missing: %q", msg)
		}
	})
}

func TestComposeMessage_MultipleCausesListsAllDetailsDominantFirst(t *testing.T) {
	// Dominant = throttle, secondary = pressure. The headline is the dominant
	// cause's, and BOTH detail paragraphs appear in Technical Details
	// (dominant first).
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindThrottling, Value: 0.12},
			{Kind: cpuhealth.CauseKindPressure, Value: 0.23},
		},
	}
	signals := cpuhealth.Signals{ThrottleRatio: 0.12, ThrottleFired: true, PressureFired: true}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	details := assertTwoLayer(t, msg, "CPU limited",
		"paused until the next cycle",
		"waiting for a free CPU core",
	)

	// The dominant (throttle) detail must appear before the secondary (pressure)
	// detail in the Technical Details section.
	throttleIdx := strings.Index(details, "paused until the next cycle")
	pressureIdx := strings.Index(details, "waiting for a free CPU core")
	if throttleIdx < 0 || pressureIdx < 0 || throttleIdx > pressureIdx {
		t.Fatalf("dominant cause detail must precede secondary: throttle@%d pressure@%d details=%q", throttleIdx, pressureIdx, details)
	}
}

// limitedVisibilityNoteText is the exact rewritten limited-visibility note the
// healthy budget dashboard appends when the box is blind (no CPU limit, no
// PSI). It is duplicated here (the package const is unexported) so the exact
// wording is pinned from the caller's side.
const limitedVisibilityNoteText = "Limited visibility: this instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so UMH cannot fully tell when work is waiting for a free core. Set a CPU limit or enable Linux pressure stats (boot with psi=1) to turn on full monitoring."

// TestComposeMessage_HealthyBudgetDashboard pins the two-layer healthy message:
// a headline stating current use against the degraded budget, and a Technical
// Details dashboard listing ONLY the applicable per-rule budgets. The displayed
// headroom is computed from the already-rounded total/used/reserve so the
// printed arithmetic is exact by construction (no independent rounding of
// HeadroomCores). One subtest per applicability configuration; each asserts the
// exact string.
func TestComposeMessage_HealthyBudgetDashboard(t *testing.T) {
	healthy := cpuhealth.Verdict{State: cpuhealth.StateHealthy}

	// Fully-instrumented: limit + PSI + virtualized, not blind. All four
	// dashboard rules appear (headroom always, then throttle/pressure/steal).
	// In limit mode the headline carries the "(Z% of its limit)" suffix and
	// the budget uses container-scope values (AvgUsageCores, ReserveCores).
	t.Run("FullyInstrumented", func(t *testing.T) {
		signals := cpuhealth.Signals{
			LimitApplies:     true,
			AvgUsageCores:    2.8,
			CapacityCores:    8,
			ReserveCores:     0.8,
			PsiApplies:       true,
			StealApplies:     true,
			ThrottleRatio:    0.0,
			PressureAvg60Out: 0.03,
			StealP95:         0.0,
		}
		want := "CPU healthy. This instance is using 2.8 of 8 cores (35% of its limit) and can use 4.4 more before it is marked degraded.\nTechnical Details: Headroom 4.4 cores = 8 total - 2.8 used - 0.8 reserved (degraded below 0). Throttling 0% (degraded above 5%). Pressure 3% (degraded above 20%). Steal 0% (degraded above 10%)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("fully-instrumented healthy message:\n got: %q\nwant: %q", got, want)
		}
	})

	// Limit-only: only the throttle budget joins the headroom line.
	t.Run("LimitOnly", func(t *testing.T) {
		signals := cpuhealth.Signals{
			LimitApplies:  true,
			AvgUsageCores: 2.8,
			CapacityCores: 8,
			ReserveCores:  0.8,
			ThrottleRatio: 0.0,
		}
		want := "CPU healthy. This instance is using 2.8 of 8 cores (35% of its limit) and can use 4.4 more before it is marked degraded.\nTechnical Details: Headroom 4.4 cores = 8 total - 2.8 used - 0.8 reserved (degraded below 0). Throttling 0% (degraded above 5%)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("limit-only healthy message:\n got: %q\nwant: %q", got, want)
		}
	})

	// PSI-only: only the pressure budget joins the headroom line.
	t.Run("PsiOnly", func(t *testing.T) {
		signals := cpuhealth.Signals{
			HostBusyCores60sMean: 2.8,
			CapacityCores:        8,
			PsiApplies:           true,
			PressureAvg60Out:     0.03,
		}
		want := "CPU healthy. The machine is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0). Pressure 3% (degraded above 20%)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("PSI-only healthy message:\n got: %q\nwant: %q", got, want)
		}
	})

	// Virtualized-only: only the steal budget joins the headroom line.
	t.Run("VirtualizedOnly", func(t *testing.T) {
		signals := cpuhealth.Signals{
			HostBusyCores60sMean: 2.8,
			CapacityCores:        8,
			StealApplies:         true,
			StealP95:             0.0,
		}
		want := "CPU healthy. The machine is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0). Steal 0% (degraded above 10%)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("virtualized-only healthy message:\n got: %q\nwant: %q", got, want)
		}
	})

	// Bare dead-zone: no rule applies, blind → only the headroom line, and the
	// rewritten limited-visibility note between the headline and Technical
	// Details.
	t.Run("BareDeadZone", func(t *testing.T) {
		signals := cpuhealth.Signals{
			HostBusyCores60sMean: 2.8,
			CapacityCores:        8,
			LimitedVisibility:    true,
		}
		want := "CPU healthy. The machine is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\n" + limitedVisibilityNoteText + "\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("bare dead-zone healthy message:\n got: %q\nwant: %q", got, want)
		}
	})

	// Headroom rounds to exactly 0.0 → the headline switches to the "close to
	// being marked degraded" phrasing (no "can use X more").
	t.Run("HeadroomZeroBoundary", func(t *testing.T) {
		signals := cpuhealth.Signals{
			HostBusyCores60sMean: 3.0,
			CapacityCores:        4,
		}
		want := "CPU healthy. The machine is using 3.0 of 4 cores and is close to being marked degraded.\nTechnical Details: Headroom 0.0 cores = 4 total - 3.0 used - 1.0 reserved (degraded below 0)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("headroom-zero-boundary healthy message:\n got: %q\nwant: %q", got, want)
		}
	})

	// Rounding consistency: raw used 2.75 rounds to 2.8, and headroom is
	// total-used-reserve on the ALREADY-ROUNDED values (8 - 2.8 - 1.0 = 4.2).
	// Independently rounding raw headroom (8 - 2.75 - 1.0 = 4.25 → 4.3) would
	// disagree; the exact string pins the by-construction 4.2.
	t.Run("RoundingConsistency", func(t *testing.T) {
		signals := cpuhealth.Signals{
			HostBusyCores60sMean: 2.75,
			CapacityCores:        8,
		}
		want := "CPU healthy. The machine is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("rounding-consistency healthy message:\n got: %q\nwant: %q", got, want)
		}
	})
}

func TestBlockReason_PerCause(t *testing.T) {
	// Throttling/Pressure/Steal/HostContention/bogus are signal-independent:
	// their BlockReason case does not consult the signals, so the zero-value
	// Signals is used. The saturation case is exercised separately in
	// TestBlockReason_SaturationSubLatch because it dispatches on the sub-latch
	// flags; here a zero-signals saturation hits the generic default.
	cases := []struct {
		kind cpuhealth.CauseKind
		want string
	}{
		{cpuhealth.CauseKindThrottling, "Can't add another bridge: this instance is already hitting its CPU limit. Raise the limit or reduce load first."},
		{cpuhealth.CauseKindPressure, "Can't add another bridge: tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first."},
		{cpuhealth.CauseKindSteal, "Can't add another bridge: the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first."},
		// Host-contention is never emitted (folded into saturation +
		// attribution): BlockReason has no case for it, so it falls through
		// to the generic degraded default.
		{cpuhealth.CauseKindHostContention, "Can't add another bridge: CPU is degraded."},
		// Saturation with no sub-latch set (shouldn't happen for a real
		// saturation cause, but BlockReason must not panic) hits the generic
		// default remediation, dropping the pre-existing first-person "we".
		{cpuhealth.CauseKindSaturation, "Can't add another bridge: CPU is running near full. Add CPU capacity, or set a CPU limit, first."},
		{cpuhealth.CauseKind("bogus"), "Can't add another bridge: CPU is degraded."},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			got := cpuhealth.BlockReason(tc.kind, cpuhealth.Signals{})
			if got != tc.want {
				t.Fatalf("BlockReason(%q): got %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

// TestBlockReason_SaturationSubLatch pins that BlockReason dispatches the
// saturation cause on the sub-latch flags carried by Signals, instead of
// returning one generic string for every saturation cause. The generic "set a
// CPU limit" advice is useless for a full host (HostFullFired /
// NoLimitHostFired: a limit caps the container, it does not protect it from a
// full host) and backwards for LimitSaturationFired (the limit is what's
// saturated, so it needs raising, not setting). NoHostStatsSaturationFired is
// the one case where "set a CPU limit" is correct advice, since a limit gives
// a scoping ceiling when host stats are unavailable.
func TestBlockReason_SaturationSubLatch(t *testing.T) {
	cases := []struct {
		name    string
		signals cpuhealth.Signals
		// contains Asserts the dispatched message names the right remediation.
		contains []string
		// notContains Asserts the wrong-for-this-sub-latch remediation is absent.
		notContains []string
	}{
		{
			name:    "HostFullFired",
			signals: cpuhealth.Signals{HostFullFired: true},
			contains: []string{
				"the machine is full",
				"Add CPU to the machine",
			},
			notContains: []string{
				// "set a CPU limit" is useless for a full host.
				"set a CPU limit",
				"Raise the limit",
				"raise the limit",
			},
		},
		{
			name:    "LimitSaturationFired",
			signals: cpuhealth.Signals{LimitSaturationFired: true},
			contains: []string{
				"at its CPU limit",
				"Raise the limit",
			},
			notContains: []string{
				// "set a CPU limit" is backwards here: the limit exists and is
				// what's saturated, so it needs raising, not setting.
				"set a CPU limit",
				"the machine is full",
			},
		},
		{
			name:    "NoHostStatsSaturationFired",
			signals: cpuhealth.Signals{NoHostStatsSaturationFired: true},
			contains: []string{
				"host stats are unavailable",
				// A limit WOULD help here: it gives a scoping ceiling when host
				// stats are unavailable.
				"set a CPU limit",
			},
			notContains: []string{
				"Raise the limit",
				"the machine is full",
			},
		},
		{
			name:    "NoLimitHostFired",
			signals: cpuhealth.Signals{NoLimitHostFired: true},
			contains: []string{
				"the machine is full",
				"Add CPU to the machine",
			},
			notContains: []string{
				// Same as HostFullFired: the host is full, a limit does not help.
				"set a CPU limit",
				"Raise the limit",
				"raise the limit",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cpuhealth.BlockReason(cpuhealth.CauseKindSaturation, tc.signals)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("BlockReason(saturation, %s): message must contain %q, got %q", tc.name, want, got)
				}
			}
			for _, bad := range tc.notContains {
				if strings.Contains(got, bad) {
					t.Errorf("BlockReason(saturation, %s): message must NOT contain %q, got %q", tc.name, bad, got)
				}
			}
		})
	}
}

// TestComposeMessage_HealthyBudget_LimitMode pins the limit-mode healthy
// budget dashboard: the used/total/reserve come from the container-scope
// signals (AvgUsageCores, CapacityCores, ReserveCores), and the headline
// carries the "(Z% of its limit)" suffix. At headroom 0.0 the boundary
// "close to being marked degraded" variant fires.
func TestComposeMessage_HealthyBudget_LimitMode(t *testing.T) {
	healthy := cpuhealth.Verdict{State: cpuhealth.StateHealthy}
	signals := cpuhealth.Signals{
		LimitApplies:  true,
		AvgUsageCores: 1.8,
		CapacityCores: 2.0,
		ReserveCores:  0.2,
	}

	msg := cpuhealth.ComposeMessage(healthy, signals)

	if !strings.Contains(msg, "1.8 of 2 cores") {
		t.Fatalf("limit-mode healthy headline must contain the used/total: %q", msg)
	}
	if !strings.Contains(msg, "(90% of its limit)") {
		t.Fatalf("limit-mode healthy headline must carry the percentage-of-limit suffix: %q", msg)
	}
	if !strings.Contains(msg, "Headroom 0.0 cores = 2 total - 1.8 used - 0.2 reserved") {
		t.Fatalf("limit-mode headroom line must use container-scope values: %q", msg)
	}
	if !strings.Contains(msg, "close to being marked degraded") {
		t.Fatalf("headroom=0.0 must trigger the boundary variant: %q", msg)
	}
}

// TestComposeMessage_HealthyBudget_NoLimit_HeadlineSubject pins the
// attribution fix: in no-limit mode composeHealthy sets usedDisp to
// HostBusyCores60sMean (the host-WIDE busy: all software on the machine, not
// just this instance), so the headline must say "The machine is using", not
// "This instance is using" (which would misattribute host-wide usage to the
// instance). Both the close-to-degraded and the has-headroom variants are
// covered. The limit-mode headline keeps "This instance is using" (there
// usedDisp = AvgUsageCores, the instance's own usage).
func TestComposeMessage_HealthyBudget_NoLimit_HeadlineSubject(t *testing.T) {
	healthy := cpuhealth.Verdict{State: cpuhealth.StateHealthy}

	// No-limit, has-headroom: host-busy 4.0 of 8 cores, 3.0 headroom.
	t.Run("NoLimit_HasHeadroom", func(t *testing.T) {
		signals := cpuhealth.Signals{
			LimitApplies:         false,
			HostBusyCores60sMean: 4.0,
			CapacityCores:        8.0,
		}
		msg := cpuhealth.ComposeMessage(healthy, signals)
		firstLine, _, _ := strings.Cut(msg, "\n")
		if !strings.Contains(firstLine, "The machine is using") {
			t.Fatalf("no-limit has-headroom headline must say \"The machine is using\" (usedDisp is host-wide): %q", firstLine)
		}
		if strings.Contains(firstLine, "This instance is using") {
			t.Fatalf("no-limit has-headroom headline must NOT say \"This instance is using\" (misattributes host-wide usage to the instance): %q", firstLine)
		}
	})

	// No-limit, close-to-degraded: headroom rounds to 0.0.
	t.Run("NoLimit_CloseToDegraded", func(t *testing.T) {
		signals := cpuhealth.Signals{
			LimitApplies:         false,
			HostBusyCores60sMean: 3.0,
			CapacityCores:        4.0,
		}
		msg := cpuhealth.ComposeMessage(healthy, signals)
		firstLine, _, _ := strings.Cut(msg, "\n")
		if !strings.Contains(firstLine, "The machine is using") {
			t.Fatalf("no-limit close-to-degraded headline must say \"The machine is using\" (usedDisp is host-wide): %q", firstLine)
		}
		if strings.Contains(firstLine, "This instance is using") {
			t.Fatalf("no-limit close-to-degraded headline must NOT say \"This instance is using\" (misattributes host-wide usage to the instance): %q", firstLine)
		}
	})

	// Limit-mode regression guard: keeps "This instance is using" (instance-scoped).
	t.Run("LimitMode_HasHeadroom", func(t *testing.T) {
		signals := cpuhealth.Signals{
			LimitApplies:  true,
			AvgUsageCores: 2.8,
			CapacityCores: 8,
			ReserveCores:  0.8,
		}
		msg := cpuhealth.ComposeMessage(healthy, signals)
		firstLine, _, _ := strings.Cut(msg, "\n")
		if !strings.Contains(firstLine, "This instance is using") {
			t.Fatalf("limit-mode has-headroom headline must keep \"This instance is using\" (usedDisp is instance-scoped): %q", firstLine)
		}
		if strings.Contains(firstLine, "The machine is using") {
			t.Fatalf("limit-mode has-headroom headline must NOT say \"The machine is using\" (usedDisp is instance-scoped): %q", firstLine)
		}
	})
}

// TestComposeMessage_HealthyBudget_NoLimit_Unchanged is a regression guard:
// the no-limit healthy budget dashboard keeps the R7 shape (host-busy as
// used, cpuReserveCores as reserve, no limit-percentage suffix).
func TestComposeMessage_HealthyBudget_NoLimit_Unchanged(t *testing.T) {
	healthy := cpuhealth.Verdict{State: cpuhealth.StateHealthy}
	signals := cpuhealth.Signals{
		LimitApplies:         false,
		HostBusyCores60sMean: 4.0,
		CapacityCores:        8.0,
	}

	msg := cpuhealth.ComposeMessage(healthy, signals)

	if !strings.Contains(msg, "using 4.0 of 8 cores") {
		t.Fatalf("no-limit healthy headline must use host-busy as used: %q", msg)
	}
	if !strings.Contains(msg, "can use 3.0 more") {
		t.Fatalf("no-limit healthy headroom must be 8 - 4.0 - 1.0 = 3.0: %q", msg)
	}
	if strings.Contains(msg, "of its limit") {
		t.Fatalf("no-limit healthy headline must NOT carry the limit suffix: %q", msg)
	}
}

// TestComposeMessage_HealthyBudget_ZeroSignalsGuard pins the cgroup-read-
// failure path: when CapacityCores is 0 (Decide never ran, signals zero-
// valued), composeHealthy must NOT emit the garbled "0.0 of 0 cores, -1.0
// headroom" budget dashboard. It returns a safe string instead. The State on
// the wire stays healthy (binary contract), so the guard must still mention
// healthy, but the message must NOT lead with "healthy" / "CPU status
// healthy": it conveys monitoring-unavailability first (the operator sees
// "unavailable" in the tooltip even though the badge stays green). It must
// also name the cgroup read as the cause so the failure is diagnosable.
func TestComposeMessage_HealthyBudget_ZeroSignalsGuard(t *testing.T) {
	healthy := cpuhealth.Verdict{State: cpuhealth.StateHealthy}
	signals := cpuhealth.Signals{}

	msg := cpuhealth.ComposeMessage(healthy, signals)

	if strings.Contains(msg, "0.0 of 0 cores") {
		t.Fatalf("zero-signals guard must prevent the garbled budget dashboard: %q", msg)
	}
	if strings.Contains(msg, "-1.0") {
		t.Fatalf("zero-signals guard must prevent the negative headroom: %q", msg)
	}
	if strings.Contains(msg, "CPU status healthy") {
		t.Fatalf("zero-signals guard must not lead with the healthy status: %q", msg)
	}
	if !strings.Contains(msg, "unavailable") {
		t.Fatalf("zero-signals guard must convey monitoring-unavailability: %q", msg)
	}
	if !strings.Contains(msg, "cgroup") {
		t.Fatalf("zero-signals guard must name the cgroup read failure: %q", msg)
	}
	if !strings.Contains(msg, "healthy") {
		t.Fatalf("zero-signals guard must still return a healthy string: %q", msg)
	}
}

// TestComposeMessage_Saturation_LimitMode pins the limit-saturation detail:
// the percentage is container-usage vs the limit (not AvgUsageFraction), and
// the guidance says "raise its CPU limit" (degraded = no room to grow, not
// broken).
func TestComposeMessage_Saturation_LimitMode(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: -0.1},
		},
	}
	signals := cpuhealth.Signals{
		LimitApplies:               true,
		LimitSaturationFired:       true,
		HostFullFired:              false,
		NoHostStatsSaturationFired: false,
		AvgUsageCores:              1.9,
		CapacityCores:              2.0,
	}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	_, details, _ := strings.Cut(msg, "Technical Details: ")
	details = strings.TrimSpace(details)

	if !strings.Contains(details, "of its limit") {
		t.Fatalf("limit-saturation detail must say 'of its limit': %q", details)
	}
	if !strings.Contains(details, "Raise its CPU limit") {
		t.Fatalf("limit-saturation detail must advise raising the limit: %q", details)
	}
	if strings.Contains(details, "0%") {
		t.Fatalf("limit-saturation detail must NOT show 0%% (AvgUsageFraction=0 bug): %q", details)
	}
	if !strings.Contains(details, "95%") {
		t.Fatalf("limit-saturation detail must show 95%% (1.9/2.0): %q", details)
	}
}

// TestComposeMessage_Saturation_HostFull pins the host-full detail: it leads
// with "The machine is full" (the un-fixable-from-inside condition dominates),
// and when both host-full and limit-saturation fire the limit fact appears as
// detail.
func TestComposeMessage_Saturation_HostFull(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionHost,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: -0.5},
		},
	}
	signals := cpuhealth.Signals{
		LimitApplies:               true,
		LimitSaturationFired:       true,
		HostFullFired:              true,
		NoHostStatsSaturationFired: false,
		CapacityCores:              2.0,
	}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	_, details, _ := strings.Cut(msg, "Technical Details: ")
	details = strings.TrimSpace(details)

	if !strings.HasPrefix(details, "The machine is full") {
		t.Fatalf("host-full detail must lead with the machine: %q", details)
	}
	if !strings.Contains(details, "at its 2-core limit") {
		t.Fatalf("both-fire detail must carry the limit fact: %q", details)
	}
	if !strings.Contains(details, "Add CPU to the machine") {
		t.Fatalf("host-full detail must advise adding CPU to the machine: %q", details)
	}
}

// TestComposeMessage_Saturation_NoHostStatsSaturation pins the no-host-stats saturation detail (no-limit, no host
// stats): it carries the "host contention is not visible" caveat and the
// psi=1 guidance.
func TestComposeMessage_Saturation_NoHostStatsSaturation(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0.82},
		},
	}
	signals := cpuhealth.Signals{
		LimitSaturationFired:       false,
		HostFullFired:              false,
		NoHostStatsSaturationFired: true,
		LimitedVisibility:          true,
	}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	_, details, _ := strings.Cut(msg, "Technical Details: ")
	details = strings.TrimSpace(details)

	if !strings.Contains(details, "Host contention is not visible") {
		t.Fatalf("no-host-stats saturation detail must carry the host-contention caveat: %q", details)
	}
	if !strings.Contains(details, "psi=1") {
		t.Fatalf("no-host-stats saturation detail must carry the psi=1 guidance: %q", details)
	}
	if !strings.Contains(details, "82%") {
		t.Fatalf("no-host-stats saturation detail must show 82%%: %q", details)
	}
	if !strings.Contains(details, "of the machine") {
		t.Fatalf("no-host-stats saturation detail must say 'of the machine': %q", details)
	}
}

// TestComposeMessage_Saturation_NoHostStatsSaturation_PsiAvailable pins the no-host-stats saturation detail when
// PSI is available: /proc/stat may be transiently unreadable while
// /proc/pressure/cpu is readable, so the no-host-stats saturation can fire with PsiApplies=true.
// In that case the message must NOT claim "no pressure stats" (false: PSI is
// on) and must NOT advise "enable psi=1" (PSI is already enabled). It must
// name the real reason host contention is invisible (host CPU usage is not
// readable) and must NOT misattribute it to a missing CPU limit.
func TestComposeMessage_Saturation_NoHostStatsSaturation_PsiAvailable(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0.75},
		},
	}
	signals := cpuhealth.Signals{
		LimitSaturationFired:       false,
		HostFullFired:              false,
		NoHostStatsSaturationFired: true,
		PsiApplies:                 true,
		LimitedVisibility:          false,
	}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	_, details, _ := strings.Cut(msg, "Technical Details: ")
	details = strings.TrimSpace(details)

	if strings.Contains(details, "no pressure stats") {
		t.Fatalf("no-host-stats saturation detail with PSI available must not claim \"no pressure stats\": %q", details)
	}
	if strings.Contains(details, "psi=1") {
		t.Fatalf("no-host-stats saturation detail with PSI available must not advise enabling psi=1: %q", details)
	}
	if strings.Contains(details, "no CPU limit set") {
		t.Fatalf("no-host-stats saturation detail must not misattribute the cause to a missing CPU limit: %q", details)
	}
	if !strings.Contains(details, "host CPU usage is not readable") {
		t.Fatalf("no-host-stats saturation detail must name the real cause (host CPU usage is not readable): %q", details)
	}
	if !strings.Contains(details, "75%") {
		t.Fatalf("no-host-stats saturation detail must show 75%%: %q", details)
	}
}

// TestComposeMessage_Saturation_NoLimitHostHeadroom pins the no-limit
// host-stats-readable saturation detail: the percentage is host-busy vs host
// cores (not AvgUsageFraction=0), and the guidance says "Add CPU capacity".
func TestComposeMessage_Saturation_NoLimitHostHeadroom(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionHost,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: -0.5},
		},
	}
	signals := cpuhealth.Signals{
		LimitApplies:               false,
		LimitSaturationFired:       false,
		HostFullFired:              false,
		NoHostStatsSaturationFired: false,
		HostBusyCores60sMean:       6.56,
		CapacityCores:              8.0,
	}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	_, details, _ := strings.Cut(msg, "Technical Details: ")
	details = strings.TrimSpace(details)

	if strings.Contains(details, "0%") {
		t.Fatalf("no-limit-host-headroom detail must NOT show 0%% (AvgUsageFraction bug): %q", details)
	}
	if !strings.Contains(details, "82%") {
		t.Fatalf("no-limit-host-headroom detail must show 82%% (6.56/8.0): %q", details)
	}
	if !strings.Contains(details, "of the machine") {
		t.Fatalf("no-limit-host-headroom detail must say 'of the machine': %q", details)
	}
	if !strings.Contains(details, "Add CPU capacity") {
		t.Fatalf("no-limit-host-headroom detail must advise adding CPU capacity: %q", details)
	}
}

// TestComposeMessage_Saturation_NoLimitHostOutage pins the sustained
// /proc/stat outage sub-case of the no-limit host-headroom saturation: when
// NoLimitHostFired is latched but HostBusyCoresAvailable is false (outage),
// the detail must NOT render "0% of the machine" (the default arm computes
// pct from the aged-out HostBusyCores60sMean=0, yielding a self-contradictory
// "0% but degraded" message) and must instead name the host-stats outage.
func TestComposeMessage_Saturation_NoLimitHostOutage(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionHost,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0},
		},
	}
	signals := cpuhealth.Signals{
		LimitApplies:               false,
		LimitSaturationFired:       false,
		HostFullFired:              false,
		NoHostStatsSaturationFired: false,
		NoLimitHostFired:           true,
		HostBusyCoresAvailable:     false,
		HostBusyCores60sMean:       0,
		CapacityCores:              8.0,
	}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	_, details, _ := strings.Cut(msg, "Technical Details: ")
	details = strings.TrimSpace(details)

	if strings.Contains(details, "0% of the machine") {
		t.Fatalf("no-limit-host-outage detail must NOT show 0%% of the machine (aged-out HostBusyCores60sMean=0 yields a self-contradictory 0%% but degraded): %q", details)
	}
	if !strings.Contains(details, "host stats") {
		t.Fatalf("no-limit-host-outage detail must name the host-stats outage: %q", details)
	}
}

// TestComposeMessage_Saturation_NoLimitHostReadableLatched pins the guard
// against the new NoLimitHostFired && !HostBusyCoresAvailable arm shadowing
// the default arm: when NoLimitHostFired is latched but HostBusyCoresAvailable
// is true (host stats readable), the default arm must run and render the
// host-busy percentage (HostBusyCores60sMean/CapacityCores), NOT the new
// "host stats unavailable" arm.
func TestComposeMessage_Saturation_NoLimitHostReadableLatched(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionHost,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0.44},
		},
	}
	signals := cpuhealth.Signals{
		LimitApplies:               false,
		LimitSaturationFired:       false,
		HostFullFired:              false,
		NoHostStatsSaturationFired: false,
		NoLimitHostFired:           true,
		HostBusyCoresAvailable:     true,
		HostBusyCores60sMean:       6.56,
		CapacityCores:              8.0,
	}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	_, details, _ := strings.Cut(msg, "Technical Details: ")
	details = strings.TrimSpace(details)

	if !strings.Contains(details, "82%") {
		t.Fatalf("readable-latched detail must render the host-busy percentage 82%% (pctOf(6.56/8.0)): %q", details)
	}
	if strings.Contains(details, "host stats") || strings.Contains(details, "unavailable") {
		t.Fatalf("readable-latched detail must NOT render the host-stats-unavailable arm (host stats are readable): %q", details)
	}
}

// TestComposeMessage_Steal_NoEmDash pins the steal detail: no em-dash (use a
// comma), and the %d reflects a peak phrasing ("up to N% at peak").
func TestComposeMessage_Steal_NoEmDash(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionHost,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSteal, Value: 0.15},
		},
	}
	signals := cpuhealth.Signals{StealFired: true, StealApplies: true}

	msg := cpuhealth.ComposeMessage(verdict, signals)

	if strings.Contains(msg, "—") {
		t.Fatalf("steal detail must NOT contain an em-dash: %q", msg)
	}
	if !strings.Contains(msg, "up to 15% at peak") {
		t.Fatalf("steal detail must use peak phrasing: %q", msg)
	}
}

// TestComposeMessage_LimitedVisibilityNote_ProductVoice pins the
// limited-visibility note: it uses "UMH cannot" (product voice), not
// "we cannot" (first-person).
func TestComposeMessage_LimitedVisibilityNote_ProductVoice(t *testing.T) {
	healthy := cpuhealth.Verdict{State: cpuhealth.StateHealthy}
	signals := cpuhealth.Signals{
		HostBusyCores60sMean: 2.8,
		CapacityCores:        8.0,
		LimitedVisibility:    true,
	}

	msg := cpuhealth.ComposeMessage(healthy, signals)

	if !strings.Contains(msg, "UMH cannot") {
		t.Fatalf("limited-visibility note must use product voice 'UMH cannot': %q", msg)
	}
	if strings.Contains(msg, "we cannot") {
		t.Fatalf("limited-visibility note must NOT use first-person 'we cannot': %q", msg)
	}
}

// TestComposeMessage_HealthyBudget_FractionalLimitRoundsToZero pins the
// sub-0.05-core quota path (e.g. a 40m Kubernetes limit = 0.04 cores).
// CapacityCores=0.04 passes the == 0 guard, but round1(0.04) collapses to 0.0,
// so usedDisp/totalDisp = +Inf and pctOf(+Inf) renders a garbled negative int
// in the "(Z% of its limit)" suffix. The healthy headline must NOT contain a
// negative percentage, +Inf, or Infinity; it must render a sane no-percentage
// variant when totalDisp rounds to 0.
func TestComposeMessage_HealthyBudget_FractionalLimitRoundsToZero(t *testing.T) {
	healthy := cpuhealth.Verdict{State: cpuhealth.StateHealthy}
	signals := cpuhealth.Signals{
		LimitApplies:  true,
		AvgUsageCores: 0.05, // rounds to 0.1 (non-zero) so usedDisp/totalDisp = +Inf
		CapacityCores: 0.04, // a 40m quota: passes the == 0 guard, rounds to 0.0
		ReserveCores:  0.0,
	}

	msg := cpuhealth.ComposeMessage(healthy, signals)

	if strings.Contains(msg, "-9223372036854775808") || strings.Contains(msg, "9223372036854775807") {
		t.Fatalf("healthy headline must NOT contain the garbled int64 from pctOf(+Inf): %q", msg)
	}
	if strings.Contains(msg, "+Inf") || strings.Contains(msg, "Infinity") {
		t.Fatalf("healthy headline must NOT contain +Inf/Infinity: %q", msg)
	}
	// The percentage suffix must not carry a negative number.
	if strings.Contains(msg, "-") && strings.Contains(msg, "of its limit") {
		t.Fatalf("healthy headline must NOT contain a negative percentage of its limit: %q", msg)
	}
	if !strings.Contains(msg, "healthy") {
		t.Fatalf("healthy headline must still render a healthy string: %q", msg)
	}
	// sub-0.05 quota still has LimitApplies=true, so usedDisp comes from
	// AvgUsageCores (instance-scoped): the no-percentage headline subject must
	// be "This instance is using", NOT "The machine is using" (which would
	// misattribute the instance's own usage to the host).
	firstLine, _, _ := strings.Cut(msg, "\n")
	if !strings.Contains(firstLine, "This instance is using") {
		t.Fatalf("sub-0.05-quota headline must say \"This instance is using\" (LimitApplies=true, usedDisp=AvgUsageCores is instance-scoped): %q", firstLine)
	}
	if strings.Contains(firstLine, "The machine is using") {
		t.Fatalf("sub-0.05-quota headline must NOT say \"The machine is using\" (usedDisp is instance-scoped, not host-wide): %q", firstLine)
	}
}

// TestComposeMessage_Saturation_CScenarioHonestyNote pins the C-scenario
// "host stats unavailable" note on the real HostBusyCoresAvailable flag (this
// replaces a HostBusyCores60sMean==0 proxy, unreliable on a readable idle host).
func TestComposeMessage_Saturation_CScenarioHonestyNote(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State: cpuhealth.StateDegraded, Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{{Kind: cpuhealth.CauseKindSaturation, Value: -0.1}},
	}
	t.Run("HostStatsUnavailable", func(t *testing.T) {
		// Limit-saturation fired; /proc/stat unreadable → the note must appear.
		signals := cpuhealth.Signals{
			SaturationFired: true, LimitSaturationFired: true, LimitApplies: true,
			AvgUsageCores: 1.9, CapacityCores: 2.0, ReserveCores: 0.2,
			HostBusyCoresAvailable: false,
		}
		msg := cpuhealth.ComposeMessage(verdict, signals)
		if !strings.Contains(msg, "Host stats are unavailable") {
			t.Fatalf("C-scenario note must appear when HostBusyCoresAvailable=false: %q", msg)
		}
	})
	t.Run("HostStatsReadable_NoNote", func(t *testing.T) {
		// Limit-saturation fired; /proc/stat readable (even if host is idle,
		// HostBusyCores60sMean ~0) → the note must NOT appear (the proxy would
		// have falsely appended it on a readable idle host).
		signals := cpuhealth.Signals{
			SaturationFired: true, LimitSaturationFired: true, LimitApplies: true,
			AvgUsageCores: 1.9, CapacityCores: 2.0, ReserveCores: 0.2,
			HostBusyCores60sMean:   0, // idle readable host, the old proxy fired here
			HostBusyCoresAvailable: true,
		}
		msg := cpuhealth.ComposeMessage(verdict, signals)
		if strings.Contains(msg, "Host stats are unavailable") {
			t.Fatalf("C-scenario note must NOT appear when HostBusyCoresAvailable=true (readable idle host): %q", msg)
		}
	})
}
