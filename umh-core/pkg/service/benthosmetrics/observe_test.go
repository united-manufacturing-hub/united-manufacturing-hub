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
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

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

// TestObserve_MetricsParseFailurePreservesHealth verifies that a 200 /metrics
// response whose body cannot be parsed soft-skips: MetricsAvailable stays false,
// Metrics stays zero, the error is nil, and the already-collected /ping, /ready
// and /version HealthCheck fields are preserved (a failing /metrics endpoint
// must not clear the others).
func TestObserve_MetricsParseFailurePreservesHealth(t *testing.T) {
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
		_, _ = w.Write([]byte("input_received{path=\"root.input\"} NOTANUMBER\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	scan, err := benthosmetrics.Observe(context.Background(), http.DefaultClient, port)
	if err != nil {
		t.Fatalf("Observe returned error: %v (an unparseable /metrics body must soft-skip, not return an error)", err)
	}

	if scan.MetricsAvailable {
		t.Errorf("MetricsAvailable = true, want false (unparseable /metrics body must not set MetricsAvailable)")
	}

	if !reflect.DeepEqual(scan.Metrics, benthosmetrics.Metrics{}) {
		t.Errorf("Metrics = %+v, want zero value (unparseable /metrics body must not return partial metrics)", scan.Metrics)
	}

	hc := scan.HealthCheck
	if !hc.IsLive {
		t.Errorf("HealthCheck.IsLive = false, want true (200 /ping must be preserved despite /metrics parse failure)")
	}

	if !hc.IsReady {
		t.Errorf("HealthCheck.IsReady = false, want true (200 /ready must be preserved despite /metrics parse failure)")
	}

	if hc.Version != "3.71.0" {
		t.Errorf("HealthCheck.Version = %q, want %q (200 /version must be preserved despite /metrics parse failure)", hc.Version, "3.71.0")
	}

	if len(hc.ConnectionStatuses) != 2 {
		t.Fatalf("len(ConnectionStatuses) = %d, want 2 (HealthCheck must be preserved despite /metrics parse failure)", len(hc.ConnectionStatuses))
	}
}

// TestObserve_MetricsBodyReadFailurePreservesHealth verifies that a 200
// /metrics response whose body read fails (truncated mid-stream) soft-skips:
// MetricsAvailable stays false, Metrics stays zero, the error is nil, and the
// already-collected /ping, /ready and /version HealthCheck fields are
// preserved (a failing /metrics endpoint must not clear the others).
func TestObserve_MetricsBodyReadFailurePreservesHealth(t *testing.T) {
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
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte("partial"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("server does not support hijacking")
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}

		_ = conn.Close()
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	scan, err := benthosmetrics.Observe(context.Background(), http.DefaultClient, port)
	if err != nil {
		t.Fatalf("Observe returned error: %v (a /metrics body-read failure must soft-skip, not return an error)", err)
	}

	if scan.MetricsAvailable {
		t.Errorf("MetricsAvailable = true, want false (/metrics body-read failure must not set MetricsAvailable)")
	}

	if !reflect.DeepEqual(scan.Metrics, benthosmetrics.Metrics{}) {
		t.Errorf("Metrics = %+v, want zero value (/metrics body-read failure must not return partial metrics)", scan.Metrics)
	}

	hc := scan.HealthCheck
	if !hc.IsLive {
		t.Errorf("HealthCheck.IsLive = false, want true (200 /ping must be preserved despite /metrics body-read failure)")
	}

	if !hc.IsReady {
		t.Errorf("HealthCheck.IsReady = false, want true (200 /ready must be preserved despite /metrics body-read failure)")
	}

	if hc.Version != "3.71.0" {
		t.Errorf("HealthCheck.Version = %q, want %q (200 /version must be preserved despite /metrics body-read failure)", hc.Version, "3.71.0")
	}

	if len(hc.ConnectionStatuses) != 2 {
		t.Fatalf("len(ConnectionStatuses) = %d, want 2 (HealthCheck must be preserved despite /metrics body-read failure)", len(hc.ConnectionStatuses))
	}
}

// TestObserve_MetricsTransportFailurePreservesHealth verifies that a /metrics
// transport failure (the GET itself errors, e.g. connection reset) soft-skips:
// MetricsAvailable stays false, Metrics stays zero, the error is nil, and the
// already-collected /ping, /ready and /version HealthCheck fields are
// preserved (a failing /metrics endpoint must not clear the others).
func TestObserve_MetricsTransportFailurePreservesHealth(t *testing.T) {
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
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("server does not support hijacking")
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}

		_ = conn.Close()
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	scan, err := benthosmetrics.Observe(context.Background(), http.DefaultClient, port)
	if err != nil {
		t.Fatalf("Observe returned error: %v (a /metrics transport failure must soft-skip, not return an error)", err)
	}

	if scan.MetricsAvailable {
		t.Errorf("MetricsAvailable = true, want false (/metrics transport failure must not set MetricsAvailable)")
	}

	if !reflect.DeepEqual(scan.Metrics, benthosmetrics.Metrics{}) {
		t.Errorf("Metrics = %+v, want zero value (/metrics transport failure must not return partial metrics)", scan.Metrics)
	}

	hc := scan.HealthCheck
	if !hc.IsLive {
		t.Errorf("HealthCheck.IsLive = false, want true (200 /ping must be preserved despite /metrics transport failure)")
	}

	if !hc.IsReady {
		t.Errorf("HealthCheck.IsReady = false, want true (200 /ready must be preserved despite /metrics transport failure)")
	}

	if hc.Version != "3.71.0" {
		t.Errorf("HealthCheck.Version = %q, want %q (200 /version must be preserved despite /metrics transport failure)", hc.Version, "3.71.0")
	}

	if len(hc.ConnectionStatuses) != 2 {
		t.Fatalf("len(ConnectionStatuses) = %d, want 2 (HealthCheck must be preserved despite /metrics transport failure)", len(hc.ConnectionStatuses))
	}
}

// freePort reserves a TCP port and immediately releases it so that nothing is
// listening: every endpoint GET returns connection-refused, modeling a fully
// down benthos instance.
func freePort(t *testing.T) uint16 {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	port := uint16(l.Addr().(*net.TCPAddr).Port)
	if err := l.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	return port
}

// TestObserve_BenthosFullyDownFoldsIntoScanNoError verifies that a fully down
// benthos (connection refused on every endpoint) folds into the Scan with NO
// returned error: the caller must see an all-false/zero Scan, not an error.
// A down benthos is an observed state, not an aborted observation.
func TestObserve_BenthosFullyDownFoldsIntoScanNoError(t *testing.T) {
	port := freePort(t)

	scan, err := benthosmetrics.Observe(context.Background(), http.DefaultClient, port)
	if err != nil {
		t.Fatalf("Observe returned error: %v (a fully down benthos must fold into the Scan, not return an error)", err)
	}

	if scan.HealthCheck.IsLive {
		t.Errorf("HealthCheck.IsLive = true, want false (no /ping reached a down benthos)")
	}

	if scan.HealthCheck.IsReady {
		t.Errorf("HealthCheck.IsReady = true, want false (no /ready reached a down benthos)")
	}

	if scan.HealthCheck.Version != "" {
		t.Errorf("HealthCheck.Version = %q, want %q (no /version reached a down benthos)", scan.HealthCheck.Version, "")
	}

	if scan.MetricsAvailable {
		t.Errorf("MetricsAvailable = true, want false (no /metrics reached a down benthos)")
	}

	if !reflect.DeepEqual(scan.Metrics, benthosmetrics.Metrics{}) {
		t.Errorf("Metrics = %+v, want zero value (no /metrics reached a down benthos)", scan.Metrics)
	}
}

// TestObserve_ReadyFailureAfterPingFoldsPreservesIsLive verifies that a /ready
// transport failure occurring AFTER a successful /ping is FOLDED into a
// nil-error partial Scan: the already-collected IsLive=true is preserved,
// IsReady stays false, and MetricsAvailable stays false (the contract pins a
// fold, NOT a non-nil error return, on a /ready failure after /ping
// succeeded — mirroring how /metrics failures fold).
func TestObserve_ReadyFailureAfterPingFoldsPreservesIsLive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	// /ready hijacks and closes the connection -> transport failure on /ready
	// only, after /ping already succeeded.
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("server does not support hijacking")
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}

		_ = conn.Close()
	})
	// /metrics also hijacks+closes so MetricsAvailable is deterministically
	// false regardless of whether Observe continues past the /ready failure.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("server does not support hijacking")
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}

		_ = conn.Close()
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	scan, err := benthosmetrics.Observe(context.Background(), http.DefaultClient, port)
	if err != nil {
		t.Fatalf("Observe returned error: %v (a /ready transport failure after a successful /ping must fold into a nil-error Scan, not return an error)", err)
	}

	if !scan.HealthCheck.IsLive {
		t.Errorf("HealthCheck.IsLive = false, want true (the successful /ping result MUST be preserved when /ready fails)")
	}

	if scan.HealthCheck.IsReady {
		t.Errorf("HealthCheck.IsReady = true, want false (/ready failed)")
	}

	if scan.MetricsAvailable {
		t.Errorf("MetricsAvailable = true, want false (/metrics failed)")
	}
}

// TestObserve_CanceledContextPropagatedAsError verifies that a canceled context
// is PROPAGATED as a non-nil error wrapping context.Canceled — NOT folded into
// a nil-error Scan. A canceled context is a programming/cancellation fault, not
// an observed benthos-down state, and must surface to the caller via
// errors.Is(err, context.Canceled).
func TestObserve_CanceledContextPropagatedAsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	port := freePort(t)

	_, err := benthosmetrics.Observe(ctx, http.DefaultClient, port)
	if err == nil {
		t.Fatalf("Observe returned nil error for a canceled context; want a non-nil error wrapping context.Canceled (a canceled context must be propagated, not folded)")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err does not wrap context.Canceled; got %v, want errors.Is(err, context.Canceled) == true", err)
	}
}

// TestObserve_ContextCanceledDuringReadyAfterPingIsPropagated verifies that a
// context canceled MID-SCRAPE — after /ping has already succeeded (200 pong),
// while /ready is in flight — is PROPAGATED as a non-nil error wrapping
// context.Canceled, NOT folded into a nil-error all-false Scan.
//
// The godoc on Observe promises UNCONDITIONALLY that a canceled context is
// propagated as a non-nil error wrapping ctx.Err() (errors.Is(err,
// context.Canceled) true). The current implementation folds ctx.Canceled at
// the /ready transport branch (return scan, nil), swallowing a mid-scrape
// cancel. This matters because the FSMv2 worker calls Observe with the
// supervisor's per-tick context, and shutdown drains by canceling that context;
// a folded cancel would publish a fabricated all-false Scan as if benthos went
// down during shutdown instead of aborting the observation.
func TestObserve_ContextCanceledDuringReadyAfterPingIsPropagated(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	// /ready blocks until the request's context is done (i.e. the client
	// canceled), then drops the connection without writing a response.
	mux.HandleFunc("/ready", func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	// 2s timeout so the test fails fast if the cancel never lands on /ready.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := benthosmetrics.Observe(ctx, http.DefaultClient, port)
	if err == nil {
		t.Fatalf("Observe returned nil error for a context canceled mid-scrape during /ready (after /ping succeeded); want a non-nil error wrapping context.Canceled (a mid-scrape cancel must be propagated, not folded into a nil-error Scan)")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err does not wrap context.Canceled; got %v, want errors.Is(err, context.Canceled) == true", err)
	}
}

// TestObserve_ContextCanceledDuringMetricsAfterPingIsPropagated verifies
// that a context canceled mid-scrape during /metrics (after /ping, /ready and
// /version succeeded) is propagated as a non-nil error wrapping
// context.Canceled, not folded into a nil-error Scan. Mirrors the /ready
// mid-scrape case: a cancel during shutdown drain must abort the observation,
// not publish a fabricated all-false Scan as if benthos went down.
func TestObserve_ContextCanceledDuringMetricsAfterPingIsPropagated(t *testing.T) {
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
	// /metrics blocks until the request's context is done (the client canceled),
	// then drops the connection without writing a response.
	mux.HandleFunc("/metrics", func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := benthosmetrics.Observe(ctx, http.DefaultClient, port)
	if err == nil {
		t.Fatalf("Observe returned nil error for a context canceled mid-scrape during /metrics (after /ping, /ready and /version succeeded); want a non-nil error wrapping context.Canceled (a mid-scrape cancel must be propagated, not folded into a nil-error Scan)")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err does not wrap context.Canceled; got %v, want errors.Is(err, context.Canceled) == true", err)
	}
}

// TestObserve_ContextCanceledDuringMetricsBodyReadAfterPingIsPropagated
// verifies that a context canceled after /ping, /ready and /version succeeded
// and /metrics returned a 200 with headers, but before io.ReadAll returns the
// /metrics body, is propagated as a non-nil error wrapping context.Canceled
// rather than folded into a nil-error partial Scan.
func TestObserve_ContextCanceledDuringMetricsBodyReadAfterPingIsPropagated(t *testing.T) {
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
	// /metrics writes a 200 status + headers (so the GET succeeds and the
	// transport branch is NOT exercised), then blocks on the request's context
	// being done (the client canceled) before writing the body. The client's
	// io.ReadAll therefore fails mid-body-read — exercising the body-read
	// branch rather than the transport branch.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Header().Set("Content-Length", "999999")
		w.WriteHeader(http.StatusOK)

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		<-r.Context().Done()
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	// 2s timeout so the test fails fast if the cancel never lands on /metrics.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := benthosmetrics.Observe(ctx, http.DefaultClient, port)
	if err == nil {
		t.Fatalf("Observe returned nil error for a context canceled mid-scrape during the /metrics body read (after /ping, /ready and /version succeeded and /metrics returned 200); want a non-nil error wrapping context.Canceled (a mid-scrape cancel during the body read must be propagated, not folded into a nil-error partial Scan)")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err does not wrap context.Canceled; got %v, want errors.Is(err, context.Canceled) == true", err)
	}
}
