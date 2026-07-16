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
package cpuhealth

import (
	"testing"
)

// These tests pin the kind-priority cause ordering: throttle, pressure, and
// steal (the serious signals) rank above saturation (capacity). A
// severity-only sort lets a low-severity starvation cause rank below a
// high-severity saturation, headlining "CPU running near full" instead of
// e.g. "CPU taken by the server" (steal), a less actionable headline on a
// box where the hypervisor is the culprit. The tests pin the kind-tier sort
// directly on constructed firedCause slices (no Decide/ring construction).

func fc(kind CauseKind, severity float64, external bool) firedCause {
	return firedCause{cause: Cause{Kind: kind, Value: 0}, severity: severity, external: external}
}

func kindsAfterSort(fired []firedCause) []CauseKind {
	sortFiredCauses(fired)

	out := make([]CauseKind, len(fired))
	for i, f := range fired {
		out[i] = f.cause.Kind
	}

	return out
}

func TestSortFiredCauses_KindTier(t *testing.T) {
	// Kind-priority overrides severity: starvation (tier 0) ranks above
	// saturation (tier 1) EVEN when saturation's severity is higher (the
	// narrow dead-zone + steal + high-container-usage case where a
	// severity-only sort deviates).
	t.Run("steal ranks above saturation despite lower severity (the R4b fix case)", func(t *testing.T) {
		got := kindsAfterSort([]firedCause{
			fc(CauseKindSteal, 0.011, true),
			fc(CauseKindSaturation, 0.333, false),
		})
		if got[0] != CauseKindSteal {
			t.Fatalf("got %v, want [steal, saturation] (steal tier 0 above saturation tier 1 despite severity 0.011 < 0.333)", got)
		}
	})

	t.Run("pressure ranks above saturation despite lower severity", func(t *testing.T) {
		got := kindsAfterSort([]firedCause{
			fc(CauseKindPressure, 0.1, false),
			fc(CauseKindSaturation, 0.9, false),
		})
		if got[0] != CauseKindPressure {
			t.Fatalf("got %v, want [pressure, saturation] (pressure tier 0 above saturation tier 1)", got)
		}
	})

	t.Run("throttle ranks above saturation despite lower severity", func(t *testing.T) {
		got := kindsAfterSort([]firedCause{
			fc(CauseKindSaturation, 0.95, false),
			fc(CauseKindThrottling, 0.05, false),
		})
		if got[0] != CauseKindThrottling {
			t.Fatalf("got %v, want [throttling, saturation]", got)
		}
	})
}

func TestSortFiredCauses_SeverityWithinTier(t *testing.T) {
	// Within the same tier, higher severity ranks first.
	t.Run("within starvation tier, higher severity first", func(t *testing.T) {
		got := kindsAfterSort([]firedCause{
			fc(CauseKindPressure, 0.3, false),
			fc(CauseKindThrottling, 0.5, false),
		})
		if got[0] != CauseKindThrottling {
			t.Fatalf("got %v, want [throttling, pressure] (same tier, throttle severity 0.5 > pressure 0.3)", got)
		}
	})

	t.Run("within saturation tier, higher severity first", func(t *testing.T) {
		fired := []firedCause{
			fc(CauseKindSaturation, 0.5, false),
			fc(CauseKindSaturation, 0.8, false),
		}
		sortFiredCauses(fired)

		if fired[0].severity != 0.8 {
			t.Fatalf("got first severity %v, want 0.8 (higher severity first within saturation tier)", fired[0].severity)
		}
	})
}

func TestSortFiredCauses_TiesGoExternal(t *testing.T) {
	// Same tier + same severity: the external (host) side ranks first.
	t.Run("steal (external) before pressure (internal) on a tie", func(t *testing.T) {
		got := kindsAfterSort([]firedCause{
			fc(CauseKindPressure, 0.1, false),
			fc(CauseKindSteal, 0.1, true),
		})
		if got[0] != CauseKindSteal {
			t.Fatalf("got %v, want [steal, pressure] (tie -> external/host first)", got)
		}
	})
}
