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

package container_monitor

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// TestCauseKindToModel_RoundTripsEveryKnownKind pins that every known
// cpuhealth.CauseKind survives the producer conversion unchanged: the
// consumer (IsResourceLimited) casts the wire string back to
// cpuhealth.CauseKind for BlockReason, so any drift here silently drops the
// cause-specific remediation to the generic default.
func TestCauseKindToModel_RoundTripsEveryKnownKind(t *testing.T) {
	kinds := []cpuhealth.CauseKind{
		cpuhealth.CauseKindSaturation,
		cpuhealth.CauseKindThrottling,
		cpuhealth.CauseKindPressure,
		cpuhealth.CauseKindSteal,
		cpuhealth.CauseKindHostContention,
	}

	for _, k := range kinds {
		got := causeKindToModel(k)
		if got != models.CauseKind(k) {
			t.Fatalf("causeKindToModel(%q): got %q, want %q (the wire string must round-trip so BlockReason keeps its specific remediation)", k, got, models.CauseKind(k))
		}
	}

	// An unknown kind passes through as its string cast (BlockReason then
	// falls back to the generic degraded message).
	if got := causeKindToModel(cpuhealth.CauseKind("future-kind")); got != models.CauseKind("future-kind") {
		t.Fatalf("causeKindToModel(unknown): got %q, want the pass-through string cast", got)
	}
}
