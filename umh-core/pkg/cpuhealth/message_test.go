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

package cpuhealth_test

import (
	"strings"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
)

// TestComposeMessage_ThrottleTwoLayerC2 pins Rung 13's behavioral contract:
// ComposeMessage is a pure function turning a Verdict + Signals into the C2
// two-layer format the Console alert adapter renders (first line = headline,
// everything after a literal "Technical Details:" separator collapses into the
// expandable panel). A degraded message MUST carry the separator — without it
// the whole message lands in the collapsed panel and the headline goes blank.
//
// This traces to the spec's "User-facing messages" supertable
// (2026-06-18-eng5128-cpu-metric-pr1-design.md) and PLAN Rung 13: the throttle
// cause composes headline "CPU limited" + the curated Technical-Details copy
// (why + live number + what-to-do), NOT the generic "CPU degraded".
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

	// (2) The literal "Technical Details:" separator MUST be present — the
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

// assertTwoLayer enforces the C2 hard constraints shared by every degraded
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

// NOTE: TestComposeMessage_HostContentionTwoLayerC2 was removed with the v4
// host-contention fold: CauseKindHostContention is never emitted (Decide folds
// a neighbour filling the box into saturation + the host/container attribution
// split), so its headline/details copy is dead code and its strings were
// deleted from causeHeadline/causeDetails. A test asserting those strings would
// pin dead code.

func TestComposeMessage_SaturationTwoLayerC2(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0.82},
		},
	}
	// LimitedVisibility true selects the blind saturation copy this test's
	// phrases assert (no limit + no PSI). The non-blind variant is pinned
	// separately in TestComposeMessage_SaturationDetailsPsiConditional.
	signals := cpuhealth.Signals{SaturationFired: true, AvgUsageFraction: 0.82, LimitedVisibility: true}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	details := assertTwoLayer(t, msg, "CPU running near full",
		"no CPU limit",
		"CPU-pressure stats",
		"Set a CPU limit",
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
// missing — it gives the actionable capacity/limit advice instead.
func TestComposeMessage_SaturationDetailsPsiConditional(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0.82},
		},
	}

	t.Run("Blind", func(t *testing.T) {
		signals := cpuhealth.Signals{SaturationFired: true, AvgUsageFraction: 0.82, LimitedVisibility: true}
		msg := cpuhealth.ComposeMessage(verdict, signals)
		want := "CPU running near full\nTechnical Details: CPU averaged 82% over the last minute, with little headroom left. This instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so we cannot confirm whether work is waiting for a free core. Set a CPU limit, which also lets us measure that wait directly, or enable Linux pressure stats (boot with psi=1). Consider adding CPU capacity."
		if msg != want {
			t.Fatalf("blind saturation message:\n got: %q\nwant: %q", msg, want)
		}
	})

	// PSI-only box: LimitedVisibility false → the non-blind variant, which does
	// NOT claim PSI is missing.
	t.Run("NonBlindPsiPresent", func(t *testing.T) {
		signals := cpuhealth.Signals{SaturationFired: true, AvgUsageFraction: 0.82, LimitedVisibility: false}
		msg := cpuhealth.ComposeMessage(verdict, signals)
		want := "CPU running near full\nTechnical Details: CPU averaged 82% over the last minute and this instance has little headroom left. Add CPU capacity, or raise its CPU limit if one is set and this load is expected."
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
const limitedVisibilityNoteText = "Limited visibility: this instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so we cannot fully tell when work is waiting for a free core. Set a CPU limit or enable Linux pressure stats (boot with psi=1) to turn on full monitoring."

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
	t.Run("FullyInstrumented", func(t *testing.T) {
		signals := cpuhealth.Signals{
			HostBusyCores60sMean: 2.8,
			CapacityCores:        8,
			LimitApplies:         true,
			PsiApplies:           true,
			StealApplies:         true,
			ThrottleRatio:        0.0,
			PressureAvg60Out:     0.03,
			StealP95:             0.0,
		}
		want := "CPU healthy. This instance is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0). Throttling 0% (degraded above 5%). Pressure 3% (degraded above 20%). Steal 0% (degraded above 10%)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("fully-instrumented healthy message:\n got: %q\nwant: %q", got, want)
		}
	})

	// Limit-only: only the throttle budget joins the headroom line.
	t.Run("LimitOnly", func(t *testing.T) {
		signals := cpuhealth.Signals{
			HostBusyCores60sMean: 2.8,
			CapacityCores:        8,
			LimitApplies:         true,
			ThrottleRatio:        0.0,
		}
		want := "CPU healthy. This instance is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0). Throttling 0% (degraded above 5%)."
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
		want := "CPU healthy. This instance is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0). Pressure 3% (degraded above 20%)."
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
		want := "CPU healthy. This instance is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0). Steal 0% (degraded above 10%)."
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
		want := "CPU healthy. This instance is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\n" + limitedVisibilityNoteText + "\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0)."
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
		want := "CPU healthy. This instance is using 3.0 of 4 cores and is close to being marked degraded.\nTechnical Details: Headroom 0.0 cores = 4 total - 3.0 used - 1.0 reserved (degraded below 0)."
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
		want := "CPU healthy. This instance is using 2.8 of 8 cores and can use 4.2 more before it is marked degraded.\nTechnical Details: Headroom 4.2 cores = 8 total - 2.8 used - 1.0 reserved (degraded below 0)."
		if got := cpuhealth.ComposeMessage(healthy, signals); got != want {
			t.Fatalf("rounding-consistency healthy message:\n got: %q\nwant: %q", got, want)
		}
	})
}

func TestBlockReason_PerCause(t *testing.T) {
	cases := []struct {
		kind cpuhealth.CauseKind
		want string
	}{
		{cpuhealth.CauseKindThrottling, "Can't add another bridge: this instance is already hitting its CPU limit. Raise the limit or reduce load first."},
		{cpuhealth.CauseKindPressure, "Can't add another bridge: tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first."},
		{cpuhealth.CauseKindSteal, "Can't add another bridge: the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first."},
		// Host-contention is folded in v4 (never emitted): its BlockReason case
		// was deleted, so it falls through to the generic degraded default.
		{cpuhealth.CauseKindHostContention, "Can't add another bridge: CPU is degraded."},
		{cpuhealth.CauseKindSaturation, "Can't add another bridge: CPU has been running near full and we can't determine the cause. Add CPU capacity, or set a CPU limit, first."},
		{cpuhealth.CauseKind("bogus"), "Can't add another bridge: CPU is degraded."},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			got := cpuhealth.BlockReason(tc.kind)
			if got != tc.want {
				t.Fatalf("BlockReason(%q): got %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}
