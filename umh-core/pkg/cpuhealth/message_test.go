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

func TestComposeMessage_HostContentionTwoLayerC2(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionHost,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindHostContention, Value: 3.0},
		},
	}
	signals := cpuhealth.Signals{HostContentionFired: true}

	msg := cpuhealth.ComposeMessage(verdict, signals)
	details := assertTwoLayer(t, msg, "Host CPU taken by other software",
		"cores are used by software outside UMH",
		"Give UMH dedicated CPU",
		"does not protect it from neighbours",
	)

	// 3 contention cores => 300% busy (Linux CPU% convention) and 3 cores.
	if !strings.Contains(details, "300%") {
		t.Fatalf("Technical Details missing the host-busy percentage 300%%: details=%q", details)
	}
	if !strings.Contains(details, "3 cores") {
		t.Fatalf("Technical Details missing the contention core count 3 cores: details=%q", details)
	}
}

func TestComposeMessage_SaturationTwoLayerC2(t *testing.T) {
	verdict := cpuhealth.Verdict{
		State:       cpuhealth.StateDegraded,
		Attribution: cpuhealth.AttributionUnknown,
		Causes: []cpuhealth.Cause{
			{Kind: cpuhealth.CauseKindSaturation, Value: 0.82},
		},
	}
	signals := cpuhealth.Signals{SaturationFired: true, AvgUsageFraction: 0.82}

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

func TestComposeMessage_HealthyReturnsHeadlineNotEmpty(t *testing.T) {
	verdict := cpuhealth.Verdict{State: cpuhealth.StateHealthy}
	signals := cpuhealth.Signals{}

	msg := cpuhealth.ComposeMessage(verdict, signals)

	if msg == "" {
		t.Fatalf("ComposeMessage returned empty string for a healthy verdict: the healthy case must render the %q headline, not a blank Console headline", "CPU healthy")
	}
	if msg != "CPU healthy" {
		t.Fatalf("healthy message: got %q, want %q (no limited-visibility note when LimitedVisibility is false)", msg, "CPU healthy")
	}
}

func TestComposeMessage_HealthyLimitedVisibilityAppendsNote(t *testing.T) {
	verdict := cpuhealth.Verdict{State: cpuhealth.StateHealthy}
	signals := cpuhealth.Signals{LimitedVisibility: true}

	msg := cpuhealth.ComposeMessage(verdict, signals)

	if msg == "" {
		t.Fatalf("ComposeMessage returned empty string for a healthy dead-zone verdict: must render the healthy headline + limited-visibility note")
	}

	firstLine, rest, found := strings.Cut(msg, "\n")
	if !found {
		t.Fatalf("healthy limited-visibility message is a single line with no note separator: %q", msg)
	}
	if firstLine != "CPU healthy" {
		t.Fatalf("headline: got %q, want %q", firstLine, "CPU healthy")
	}
	if !strings.Contains(rest, "Limited visibility:") {
		t.Fatalf("limited-visibility note missing from message: %q", msg)
	}
	if !strings.Contains(rest, "psi=1") {
		t.Fatalf("limited-visibility note missing the psi=1 remediation: %q", msg)
	}
}

func TestBlockReason_PerCause(t *testing.T) {
	cases := []struct {
		kind cpuhealth.CauseKind
		want string
	}{
		{cpuhealth.CauseKindThrottling, "Can't add another bridge: this instance is already hitting its CPU limit. Raise the limit or reduce load first."},
		{cpuhealth.CauseKindPressure, "Can't add another bridge: tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first."},
		{cpuhealth.CauseKindSteal, "Can't add another bridge: the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first."},
		{cpuhealth.CauseKindHostContention, "Can't add another bridge: other software on this host is using most of the CPU. Give UMH dedicated CPU, or reduce what else runs here, first."},
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
