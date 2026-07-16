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

package generator

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// composeContainerHealthMessage propagates the CPU's curated health.message
// (cpuhealth.ComposeMessage output, stored on cpu.Health.Message by
// getCPUMetrics) to the container health.message on the wire (the field MC
// renders in the badge tooltip), instead of the generic per-category string.
// It prefers the CPU message when CPU is the health driver (healthy OR
// CPU-degraded); otherwise it falls back to the generic line. These tests pin
// the propagation contract directly on the pure helper (no fsm-snapshot
// construction needed).

func withCPUMessage(msg string) *models.CPU {
	return &models.CPU{Health: &models.Health{Message: msg}}
}

func TestComposeContainerHealthMessage(t *testing.T) {
	const (
		blindNote    = "CPU healthy\nLimited visibility: no CPU limit or pressure stats set, so starvation cannot be fully measured. Set a CPU limit or enable Linux pressure stats (psi=1) to turn on full monitoring."
		richDegraded = "CPU running near full\nTechnical Details: CPU averaged 82% over the last minute. This instance has no CPU limit and its OS isn't reporting CPU-pressure stats, so we can't confirm whether work is being starved — but there's little headroom left."
	)

	t.Run("healthy blind box surfaces the CPU limitedVisibility note (the delivery case)", func(t *testing.T) {
		got := composeContainerHealthMessage(models.Active, models.Active, withCPUMessage(blindNote))
		if got != blindNote {
			t.Fatalf("healthy+blind: got %q, want the CPU message (with the limitedVisibility note), not the generic %q", got, getContainerHealthMessage(models.Active))
		}
	})

	t.Run("healthy non-blind surfaces the CPU healthy message", func(t *testing.T) {
		got := composeContainerHealthMessage(models.Active, models.Active, withCPUMessage("CPU healthy"))
		if got != "CPU healthy" {
			t.Fatalf("healthy: got %q, want %q", got, "CPU healthy")
		}
	})

	t.Run("healthy with no CPU message falls back to the generic healthy string", func(t *testing.T) {
		got := composeContainerHealthMessage(models.Active, models.Active, nil)
		if got != getContainerHealthMessage(models.Active) {
			t.Fatalf("healthy no-CPU: got %q, want %q", got, getContainerHealthMessage(models.Active))
		}
	})

	t.Run("CPU-degraded surfaces the rich two-layer CPU message", func(t *testing.T) {
		got := composeContainerHealthMessage(models.Degraded, models.Degraded, withCPUMessage(richDegraded))
		if got != richDegraded {
			t.Fatalf("CPU-degraded: got %q, want the rich CPU message, not the generic %q", got, getContainerHealthMessage(models.Degraded))
		}
	})

	t.Run("CPU-degraded with empty CPU message falls back to the generic degraded string", func(t *testing.T) {
		got := composeContainerHealthMessage(models.Degraded, models.Degraded, withCPUMessage(""))
		if got != getContainerHealthMessage(models.Degraded) {
			t.Fatalf("CPU-degraded empty: got %q, want %q", got, getContainerHealthMessage(models.Degraded))
		}
	})

	t.Run("memory-degraded does NOT leak the CPU message (CPU is not the driver)", func(t *testing.T) {
		// overall Degraded but CPU is Active (memory is the degraded driver): the
		// CPU's healthy message must NOT propagate to the container message —
		// memory/disk get the generic line until they have rich composers (TODO).
		got := composeContainerHealthMessage(models.Degraded, models.Active, withCPUMessage("CPU healthy"))
		if got != getContainerHealthMessage(models.Degraded) {
			t.Fatalf("memory-degraded: got %q, want the generic %q (CPU message must not leak when CPU is not the driver)", got, getContainerHealthMessage(models.Degraded))
		}
	})

	t.Run("degraded with nil CPU falls back to the generic degraded string (no nil panic)", func(t *testing.T) {
		got := composeContainerHealthMessage(models.Degraded, models.Active, nil)
		if got != getContainerHealthMessage(models.Degraded) {
			t.Fatalf("degraded nil-CPU: got %q, want %q", got, getContainerHealthMessage(models.Degraded))
		}
	})
}
