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

// ENG-5006 reproduction: output metrics are broken for `switch` outputs.
//
// Benthos's `Manager.IntoPath()` injects a `path` label per output instance.
// Coordinators (switch/broker/fallback) do not emit `output_sent` themselves;
// only leaf `AsyncWriter`s do. So a switch with N cases emits N distinct
// `output_sent` series with different `path` labels and NO top-level aggregate.
//
// Both parsers currently treat `output_sent` as a singleton:
//   - ParseMetricsFromBytes:     last-wins (overwrite in the case branch)
//   - ParseMetricsFromBytesSlow: first-wins (reads family.GetMetric()[0])
//
// Either way the UI shows the wrong number. These tests pin the bug.
package benthos_monitor

import "testing"

// switchOutputMetrics models the prometheus output benthos emits for the
// reporter's three-route switch config in ENG-5006 (job_start/job_end/logbook).
// Each case has a sql_raw → stdout fallback; the leaf AsyncWriters are
// sql_raw (path .../fallback.0) and stdout (path .../fallback.1).
//
// Numbers: 10 messages to case 0, 7 to case 1, 3 to case 2. Total 20.
const switchOutputMetrics = `# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 20
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output.switch.cases.0.output.fallback.0"} 10
output_sent{label="",path="root.output.switch.cases.0.output.fallback.1"} 0
output_sent{label="",path="root.output.switch.cases.1.output.fallback.0"} 7
output_sent{label="",path="root.output.switch.cases.1.output.fallback.1"} 0
output_sent{label="",path="root.output.switch.cases.2.output.fallback.0"} 3
output_sent{label="",path="root.output.switch.cases.2.output.fallback.1"} 0
# HELP output_batch_sent Benthos Counter metric
# TYPE output_batch_sent counter
output_batch_sent{label="",path="root.output.switch.cases.0.output.fallback.0"} 10
output_batch_sent{label="",path="root.output.switch.cases.0.output.fallback.1"} 0
output_batch_sent{label="",path="root.output.switch.cases.1.output.fallback.0"} 7
output_batch_sent{label="",path="root.output.switch.cases.1.output.fallback.1"} 0
output_batch_sent{label="",path="root.output.switch.cases.2.output.fallback.0"} 3
output_batch_sent{label="",path="root.output.switch.cases.2.output.fallback.1"} 0
# HELP output_error Benthos Counter metric
# TYPE output_error counter
output_error{label="",path="root.output.switch.cases.0.output.fallback.0"} 1
output_error{label="",path="root.output.switch.cases.1.output.fallback.0"} 2
output_error{label="",path="root.output.switch.cases.2.output.fallback.0"} 0
# HELP output_connection_up Benthos Counter metric
# TYPE output_connection_up counter
output_connection_up{label="",path="root.output.switch.cases.0.output.fallback.0"} 1
output_connection_up{label="",path="root.output.switch.cases.1.output.fallback.0"} 1
output_connection_up{label="",path="root.output.switch.cases.2.output.fallback.0"} 1
`

const wantSentTotal int64 = 10 + 7 + 3   // 20
const wantBatchSentTotal int64 = 10 + 7 + 3
const wantErrorTotal int64 = 1 + 2 + 0   // 3
const wantConnectionUpTotal int64 = 1 + 1 + 1 // 3

func TestENG5006_SwitchOutput_FastParser_AggregatesAcrossRoutes(t *testing.T) {
	m, err := ParseMetricsFromBytes([]byte(switchOutputMetrics))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Totals across all three switch routes via Total helpers.
	if got := m.OutputSentTotal(); got != wantSentTotal {
		t.Errorf("OutputSentTotal() = %d, want %d (sum across switch routes)", got, wantSentTotal)
	}

	if got := m.OutputBatchSentTotal(); got != wantBatchSentTotal {
		t.Errorf("OutputBatchSentTotal() = %d, want %d", got, wantBatchSentTotal)
	}

	if got := m.OutputConnectionUpTotal(); got != wantConnectionUpTotal {
		t.Errorf("OutputConnectionUpTotal() = %d, want %d", got, wantConnectionUpTotal)
	}

	// Sum the map directly here to keep the per-path assertion explicit
	// alongside the total. OutputErrorTotal() covers the aggregate elsewhere.
	var errorTotal int64
	for _, out := range m.Outputs {
		errorTotal += out.Error
	}

	if errorTotal != wantErrorTotal {
		t.Errorf("sum of Outputs[*].Error = %d, want %d", errorTotal, wantErrorTotal)
	}

	// Map-shape check: the switch fixture has 6 distinct output paths
	// (3 cases x 2 fallback leaves). If the parser collapses paths or
	// drops the map, this fails even when the totals happen to add up.
	if got := len(m.Outputs); got != 6 {
		t.Errorf("len(m.Outputs) = %d, want 6 (3 cases x 2 fallback leaves)", got)
	}

	// Per-leaf-path check: the leading sql_raw sink for case 0 received
	// 10 messages. A parser that summed all routes into one entry would
	// fail this even when the totals are correct.
	const leafPath = "root.output.switch.cases.0.output.fallback.0"

	leaf, ok := m.Outputs[leafPath]
	if !ok {
		t.Fatalf("m.Outputs[%q] missing; got keys: %v", leafPath, mapKeys(m.Outputs))
	}

	if leaf.Sent != 10 {
		t.Errorf("m.Outputs[%q].Sent = %d, want 10 (the case-0 sql_raw leaf)", leafPath, leaf.Sent)
	}

	if leaf.Path != leafPath {
		t.Errorf("m.Outputs[%q].Path = %q, want %q (parser must populate Path on the instance)", leafPath, leaf.Path, leafPath)
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

func TestENG5006_SwitchOutput_SlowParser_AggregatesAcrossRoutes(t *testing.T) {
	t.Skip("slow parser is deleted in C6 of ENG-5006; arm kept for completeness")

	m, err := ParseMetricsFromBytesSlow([]byte(switchOutputMetrics))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got := m.OutputSentTotal(); got != wantSentTotal {
		t.Errorf("OutputSentTotal() = %d, want %d (sum across switch routes)", got, wantSentTotal)
	}

	if got := m.OutputBatchSentTotal(); got != wantBatchSentTotal {
		t.Errorf("OutputBatchSentTotal() = %d, want %d", got, wantBatchSentTotal)
	}

	var errorTotal int64
	for _, out := range m.Outputs {
		errorTotal += out.Error
	}

	if errorTotal != wantErrorTotal {
		t.Errorf("sum of Outputs[*].Error = %d, want %d", errorTotal, wantErrorTotal)
	}

	if got := m.OutputConnectionUpTotal(); got != wantConnectionUpTotal {
		t.Errorf("OutputConnectionUpTotal() = %d, want %d", got, wantConnectionUpTotal)
	}
}
