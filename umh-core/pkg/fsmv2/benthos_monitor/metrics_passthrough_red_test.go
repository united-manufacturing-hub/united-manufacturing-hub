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

package fsmv2benthosmonitor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

// connectionCounterMetrics is the prometheus exposition of a bridge whose input
// is a broker and whose output is a fallback, each wrapping two child
// components. Every connection counter appears once per child `path` and never
// as a top-level aggregate, so this exposition carries two series for each
// counter. Benthos labels each child with its own `path`, and the
// broker/fallback wrapper emits nothing of its own — the shape the ENG-5006 fix
// already handles for output_sent.
//
// Every counter here is deliberately non-zero on BOTH sides, including the two
// *_connection_lost series. A fixture whose lost counters were zero would let an
// all-empty Metrics satisfy the "lost" half of the assertions by accident.
//
// Connections are up more often than they were lost, so the verdict the two
// bridge-health consumers compute (connection up minus connection lost, greater
// than zero — see isActive in pkg/fsm/protocolconverter/actions.go) is true for
// both directions: this fixture describes a healthy bridge.
const connectionCounterMetrics = `# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input.broker.0"} 60
input_received{label="",path="root.input.broker.1"} 40
# HELP input_connection_up Benthos Counter metric
# TYPE input_connection_up counter
input_connection_up{label="",path="root.input.broker.0"} 3
input_connection_up{label="",path="root.input.broker.1"} 2
# HELP input_connection_lost Benthos Counter metric
# TYPE input_connection_lost counter
input_connection_lost{label="",path="root.input.broker.0"} 1
input_connection_lost{label="",path="root.input.broker.1"} 1
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output.fallback.0"} 70
output_sent{label="",path="root.output.fallback.1"} 30
# HELP output_connection_up Benthos Counter metric
# TYPE output_connection_up counter
output_connection_up{label="",path="root.output.fallback.0"} 4
output_connection_up{label="",path="root.output.fallback.1"} 1
# HELP output_connection_lost Benthos Counter metric
# TYPE output_connection_lost counter
output_connection_lost{label="",path="root.output.fallback.0"} 2
output_connection_lost{label="",path="root.output.fallback.1"} 0
`

// Written as sums so each total can be checked against the series it came from,
// rather than against whatever the code happens to produce.
const (
	wantInputConnectionUp    int64 = 3 + 2
	wantInputConnectionLost  int64 = 1 + 1
	wantOutputConnectionUp   int64 = 4 + 1
	wantOutputConnectionLost int64 = 2 + 0

	// The two scrape totals the throughput window records.
	wantInputReceived = 60 + 40
	wantOutputSent    = 70 + 30
)

// TestMapObservedDeliversParsedConnectionCountersToBridgeHealth asserts that the
// connection counters scraped from /metrics arrive, non-zero and per-path
// summed, at the four reads that decide whether a bridge is connected.
//
// Those four reads are InputConnectionUpTotal, InputConnectionLostTotal,
// OutputConnectionUpTotal and OutputConnectionLostTotal, called on
// ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.Metrics. Two consumers
// perform them on exactly this object, reached through different outer nesting:
// pkg/fsm/protocolconverter/actions.go (safeBenthosMetricsFrom) and
// pkg/fsm/streamprocessor/actions.go (safeBenthosMetrics). Both then judge a
// direction connected when connection-up exceeds connection-lost, which this
// test mirrors on the same values.
//
// The whole path runs for real: a local server answers the four endpoints, Poll
// scrapes it, and mapObserved maps the resulting status. No metric value is
// hand-built, so every number asserted here can only come from the served
// exposition.
func TestMapObservedDeliversParsedConnectionCountersToBridgeHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"error":"","statuses":[]}`))
		case "/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"1.2.3","build":"abc123"}`))
		case "/metrics":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(connectionCounterMetrics))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	port := srv.Listener.Addr().(*net.TCPAddr).Port
	cfg := config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: "benthos-1", DesiredFSMState: "active"},
		MetricsPort:       uint16(port),
	}
	deps := &benthosMonitorDeps{client: &http.Client{Timeout: 400 * time.Millisecond}}

	status, err := Poll(context.Background(), deps, cfg)
	if err != nil {
		t.Fatalf("Poll errored against the served endpoints: %v", err)
	}

	// LastCount is the newest counter the window recorded.
	if status.Input.LastCount != wantInputReceived {
		t.Fatalf("Input.LastCount = %d, want %d: the served /metrics was not scraped as expected, so this test cannot say anything about the mapping",
			status.Input.LastCount, wantInputReceived)
	}

	if status.Output.LastCount != wantOutputSent {
		t.Fatalf("Output.LastCount = %d, want %d: the served /metrics was not scraped as expected, so this test cannot say anything about the mapping",
			status.Output.LastCount, wantOutputSent)
	}

	observed := mapObserved(cfg, simple.Status[BenthosMonitorStatus]{Result: status})

	bmState, ok := observed.(benthosmonitorfsm.BenthosMonitorObservedState)
	if !ok {
		t.Fatalf("mapObserved returned %T, want benthosmonitorfsm.BenthosMonitorObservedState", observed)
	}

	if bmState.ServiceInfo == nil {
		t.Fatal("ServiceInfo is nil")
	}

	scan := bmState.ServiceInfo.BenthosStatus.LastScan
	if scan == nil || scan.BenthosMetrics == nil {
		t.Fatal("LastScan or LastScan.BenthosMetrics is nil")
	}

	// The exact object both consumers read.
	metrics := scan.BenthosMetrics.Metrics

	// MetricsState is non-nil while the metrics behind it are empty, so the nil
	// guard in both consumers does not fire.
	if got := metrics.InputConnectionUpTotal(); got != wantInputConnectionUp {
		t.Errorf("InputConnectionUpTotal() = %d, want %d (input_connection_up summed over root.input.broker.0 and .1)",
			got, wantInputConnectionUp)
	}

	if got := metrics.InputConnectionLostTotal(); got != wantInputConnectionLost {
		t.Errorf("InputConnectionLostTotal() = %d, want %d (input_connection_lost summed over root.input.broker.0 and .1)",
			got, wantInputConnectionLost)
	}

	if got := metrics.OutputConnectionUpTotal(); got != wantOutputConnectionUp {
		t.Errorf("OutputConnectionUpTotal() = %d, want %d (output_connection_up summed over root.output.fallback.0 and .1)",
			got, wantOutputConnectionUp)
	}

	if got := metrics.OutputConnectionLostTotal(); got != wantOutputConnectionLost {
		t.Errorf("OutputConnectionLostTotal() = %d, want %d (output_connection_lost summed over root.output.fallback.0 and .1)",
			got, wantOutputConnectionLost)
	}

	// The consumers' own verdict, computed the way they compute it. The fixture
	// describes a healthy bridge, so both directions must read connected.
	if metrics.InputConnectionUpTotal()-metrics.InputConnectionLostTotal() <= 0 {
		t.Errorf("the input direction reads as not connected (up %d - lost %d), but the served scrape describes a connected input",
			metrics.InputConnectionUpTotal(), metrics.InputConnectionLostTotal())
	}

	if metrics.OutputConnectionUpTotal()-metrics.OutputConnectionLostTotal() <= 0 {
		t.Errorf("the output direction reads as not connected (up %d - lost %d), but the served scrape describes a connected output",
			metrics.OutputConnectionUpTotal(), metrics.OutputConnectionLostTotal())
	}
}
