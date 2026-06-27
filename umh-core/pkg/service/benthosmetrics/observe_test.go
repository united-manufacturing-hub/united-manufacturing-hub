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

package benthosmetrics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthosmetrics"
)

// sampleMetrics is copied verbatim from
// pkg/service/benthos_monitor/benthos_monitor_benchmark_test.go's sampleMetrics var.
var sampleMetrics = `# HELP input_connection_failed Benthos Counter metric
# TYPE input_connection_failed counter
input_connection_failed{label="",path="root.input"} 0
# HELP input_connection_lost Benthos Counter metric
# TYPE input_connection_lost counter
input_connection_lost{label="",path="root.input"} 0
# HELP input_connection_up Benthos Counter metric
# TYPE input_connection_up counter
input_connection_up{label="",path="root.input"} 1
# HELP input_latency_ns Benthos Timing metric
# TYPE input_latency_ns summary
input_latency_ns{label="",path="root.input",quantile="0.5"} 127167
input_latency_ns{label="",path="root.input",quantile="0.9"} 378375
input_latency_ns{label="",path="root.input",quantile="0.99"} 858666
input_latency_ns_sum{label="",path="root.input"} 3.629208e+06
input_latency_ns_count{label="",path="root.input"} 18
# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 18
# HELP output_batch_sent Benthos Counter metric
# TYPE output_batch_sent counter
output_batch_sent{label="",path="root.output"} 18
# HELP output_connection_failed Benthos Counter metric
# TYPE output_connection_failed counter
output_connection_failed{label="",path="root.output"} 0
# HELP output_connection_lost Benthos Counter metric
# TYPE output_connection_lost counter
output_connection_lost{label="",path="root.output"} 0
# HELP output_connection_up Benthos Counter metric
# TYPE output_connection_up counter
output_connection_up{label="",path="root.output"} 1
# HELP output_error Benthos Counter metric
# TYPE output_error counter
output_error{label="",path="root.output"} 0
# HELP output_latency_ns Benthos Timing metric
# TYPE output_latency_ns summary
output_latency_ns{label="",path="root.output",quantile="0.5"} 33250
output_latency_ns{label="",path="root.output",quantile="0.9"} 94709
output_latency_ns{label="",path="root.output",quantile="0.99"} 138250
output_latency_ns_sum{label="",path="root.output"} 816919
output_latency_ns_count{label="",path="root.output"} 18
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output"} 18
`

// portFromURL extracts the port from an httptest server URL.
func portFromURL(t *testing.T, rawURL string) uint16 {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	p, err := strconv.ParseUint(u.Port(), 10, 16)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	return uint16(p)
}

// TestObserve_HappyPath scrapes a benthos instance's 4 HTTP endpoints in
// process via benthosmetrics.Observe and asserts the returned Scan carries
// the parsed metrics and health check (happy path: all endpoints succeed).
func TestObserve_HappyPath(t *testing.T) {
	readyBody := `{"statuses":[{"label":"tcp_server","path":"root.input","connected":true},{"label":"http_client","path":"root.output","connected":true}]}`
	versionBody := `{"version":"3.71.0","built":"2023-08-15T12:00:00Z"}`

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(readyBody))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(versionBody))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(sampleMetrics))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	scan, err := benthosmetrics.Observe(context.Background(), http.DefaultClient, port)
	if err != nil {
		t.Fatalf("Observe returned error: %v", err)
	}

	if !scan.MetricsAvailable {
		t.Errorf("MetricsAvailable = false, want true")
	}

	hc := scan.HealthCheck
	if !hc.IsLive {
		t.Errorf("HealthCheck.IsLive = false, want true (200 /ping)")
	}

	if !hc.IsReady {
		t.Errorf("HealthCheck.IsReady = false, want true (200 /ready, no error)")
	}

	if hc.ReadyError != "" {
		t.Errorf("HealthCheck.ReadyError = %q, want %q (no error on happy path)", hc.ReadyError, "")
	}

	if hc.Version != "3.71.0" {
		t.Errorf("HealthCheck.Version = %q, want %q", hc.Version, "3.71.0")
	}

	if len(hc.ConnectionStatuses) != 2 {
		t.Fatalf("len(ConnectionStatuses) = %d, want 2", len(hc.ConnectionStatuses))
	}

	wantStatuses := []benthosmetrics.ConnStatus{
		{Label: "tcp_server", Path: "root.input", Connected: true},
		{Label: "http_client", Path: "root.output", Connected: true},
	}
	for i, want := range wantStatuses {
		got := hc.ConnectionStatuses[i]
		if got.Label != want.Label || got.Path != want.Path || got.Connected != want.Connected {
			t.Errorf("ConnectionStatuses[%d] = %+v, want %+v", i, got, want)
		}
	}

	m := scan.Metrics

	if got := m.InputReceivedTotal(); got != 18 {
		t.Errorf("Metrics.InputReceivedTotal() = %d, want 18", got)
	}

	if got := m.OutputSentTotal(); got != 18 {
		t.Errorf("Metrics.OutputSentTotal() = %d, want 18", got)
	}

	if got := m.InputConnectionUpTotal(); got != 1 {
		t.Errorf("Metrics.InputConnectionUpTotal() = %d, want 1", got)
	}

	if got := m.OutputBatchSentTotal(); got != 18 {
		t.Errorf("Metrics.OutputBatchSentTotal() = %d, want 18", got)
	}
}

// TestObserve_MetricsHTTPFailureKeepsHealthAndDoesNotClaimMetrics verifies
// that a failed /metrics scrape (HTTP 500) does not set MetricsAvailable and
// does not clear the /ping, /ready and /version fields on the HealthCheck.
func TestObserve_MetricsHTTPFailureKeepsHealthAndDoesNotClaimMetrics(t *testing.T) {
	readyBody := `{"statuses":[{"label":"tcp_server","path":"root.input","connected":true},{"label":"http_client","path":"root.output","connected":true}]}`
	versionBody := `{"version":"3.71.0","built":"2023-08-15T12:00:00Z"}`

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(readyBody))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(versionBody))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	scan, err := benthosmetrics.Observe(context.Background(), http.DefaultClient, port)
	if err != nil {
		t.Fatalf("Observe returned error: %v (a failed /metrics scrape must not be a returned error)", err)
	}

	if scan.MetricsAvailable {
		t.Errorf("MetricsAvailable = true, want false (failed /metrics scrape must not set MetricsAvailable)")
	}

	if !reflect.DeepEqual(scan.Metrics, benthosmetrics.Metrics{}) {
		t.Errorf("Metrics = %+v, want zero value (failed /metrics scrape must not return stale or partial metrics)", scan.Metrics)
	}

	hc := scan.HealthCheck
	if !hc.IsLive {
		t.Errorf("HealthCheck.IsLive = false, want true (200 /ping must still populate despite /metrics failing)")
	}

	if !hc.IsReady {
		t.Errorf("HealthCheck.IsReady = false, want true (200 /ready must still populate despite /metrics failing)")
	}

	if hc.Version != "3.71.0" {
		t.Errorf("HealthCheck.Version = %q, want %q", hc.Version, "3.71.0")
	}

	if len(hc.ConnectionStatuses) != 2 {
		t.Fatalf("len(ConnectionStatuses) = %d, want 2 (health endpoints still populated despite /metrics failing)", len(hc.ConnectionStatuses))
	}
}
