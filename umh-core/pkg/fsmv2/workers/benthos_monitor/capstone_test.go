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

package benthos_monitor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthosmetrics"
)

// serverState holds the swappable per-endpoint behavior the handlers read on
// every request, so a single httptest server can transition through benthos
// up/down/stopped states mid-test without being recreated.
type serverState struct {
	// up is true when the benthos instance is reachable and healthy. When
	// false, every endpoint hijacks and closes the connection — a transport
	// failure that models a fully-down benthos against the same port (every
	// endpoint unreachable, so Observe folds at /ping into a nil-error zero
	// Scan without reaching /ready, /version or /metrics).
	up atomic.Bool
	// metricsHits counts /metrics requests so the stopped leg can assert no
	// scrape was issued (the ShouldStop skip beats a live server).
	metricsHits atomic.Int32
}

// capstoneSampleMetrics is a fresh inlined copy of the sampleMetrics Prometheus
// body (input_received=18, output_sent=18) used across the benthos monitor
// tests; it lives here so the capstone test is self-contained in the external
// test package.
const capstoneReadyBody = `{"statuses":[{"label":"tcp_server","path":"root.input","connected":true},{"label":"http_client","path":"root.output","connected":true}]}`
const capstoneVersionBody = `{"version":"3.71.0","built":"2023-08-15T12:00:00Z"}`
const capstoneMetricsBody = `# HELP input_connection_failed Benthos Counter metric
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

// newSwappableServer builds one httptest server whose handlers read serverState
// so the same benthos instance can transition through up/down states mid-test.
// The /metrics handler always increments metricsHits so the stopped leg can
// prove no scrape was issued.
func newSwappableServer(st *serverState) *httptest.Server {
	mux := http.NewServeMux()

	// closeConn hijacks and closes the underlying connection, producing a
	// transport-level failure on that endpoint (models a down benthos where
	// the endpoint is unreachable).
	closeConn := func(w http.ResponseWriter) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			panic("server does not support hijacking")
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			panic("hijack: " + err.Error())
		}

		_ = conn.Close()
	}

	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		if !st.up.Load() {
			closeConn(w)

			return
		}

		_, _ = w.Write([]byte("pong"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if !st.up.Load() {
			closeConn(w)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(capstoneReadyBody))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		if !st.up.Load() {
			closeConn(w)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(capstoneVersionBody))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		st.metricsHits.Add(1)

		if !st.up.Load() {
			closeConn(w)

			return
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(capstoneMetricsBody))
	})

	return httptest.NewServer(mux)
}

// capstonePortFromURL extracts the port from an httptest server URL.
func capstonePortFromURL(rawURL string) uint16 {
	u, err := url.Parse(rawURL)
	Expect(err).NotTo(HaveOccurred())
	p, err := strconv.ParseUint(u.Port(), 10, 16)
	Expect(err).NotTo(HaveOccurred())

	return uint16(p)
}

// cos drives one CollectObservedState call against the worker with the given
// desired State and port, returning the typed observation. It centralizes the
// type assertion so each leg's assertions stay focused on the behavior.
func cos(
	ctx context.Context,
	worker *benthos_monitor.BenthosMonitorWorker,
	state string,
	port uint16,
) fsmv2.Observation[benthos_monitor.BenthosMonitorStatus] {
	desired := &fsmv2.WrappedDesiredState[benthos_monitor.BenthosMonitorConfig]{
		Config: benthos_monitor.BenthosMonitorConfig{
			BaseUserSpec: config.BaseUserSpec{State: state},
			MetricsPort:  port,
		},
	}

	obs, err := worker.CollectObservedState(ctx, desired)
	Expect(err).NotTo(HaveOccurred())
	Expect(obs).NotTo(BeNil())

	typedObs, ok := obs.(fsmv2.Observation[benthos_monitor.BenthosMonitorStatus])
	Expect(ok).To(BeTrue(), "expected fsmv2.Observation[BenthosMonitorStatus], got %T", obs)

	return typedObs
}

var _ = Describe("BenthosMonitorWorker end-to-end across benthos states", func() {
	var (
		logger deps.FSMLogger
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
	})

	It("drives CollectObservedState through up, down, stopped and recovery legs on one server", func() {
		st := &serverState{}
		st.up.Store(true)

		srv := newSwappableServer(st)
		DeferCleanup(srv.Close)

		port := capstonePortFromURL(srv.URL)

		identity := deps.Identity{ID: "benthos-monitor-capstone", WorkerType: "benthos_monitor"}
		worker, err := benthos_monitor.NewBenthosMonitorWorker(identity, logger, nil, http.DefaultClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(worker).NotTo(BeNil())

		ctx := context.Background()

		// Leg 1: UP + running. The benthos instance is reachable and healthy.
		// Expect a full Scan: MetricsAvailable, IsLive, InputReceivedTotal=18.
		obs1 := cos(ctx, worker, "running", port)
		Expect(obs1.Status.Stopped).To(BeFalse(), "leg 1 (up+running): Stopped must be false")
		Expect(obs1.Status.Scan.MetricsAvailable).To(BeTrue(), "leg 1 (up+running): MetricsAvailable must be true")
		Expect(obs1.Status.Scan.HealthCheck.IsLive).To(BeTrue(), "leg 1 (up+running): IsLive must be true")
		Expect(obs1.Status.Scan.Metrics.InputReceivedTotal()).To(Equal(int64(18)), "leg 1 (up+running): InputReceivedTotal must be 18")

		// Leg 2: DOWN + running. Flip the server to down (503 on all endpoints).
		// The down benthos must fold into the Scan with a NIL error (down is an
		// observed state, not an aborted observation) and IsLive false.
		st.up.Store(false)

		obs2 := cos(ctx, worker, "running", port)
		Expect(obs2.Status.Stopped).To(BeFalse(), "leg 2 (down+running): Stopped must be false (down is not admin-pause)")
		Expect(obs2.Status.Scan.MetricsAvailable).To(BeFalse(), "leg 2 (down+running): MetricsAvailable must be false")
		Expect(obs2.Status.Scan.HealthCheck.IsLive).To(BeFalse(), "leg 2 (down+running): IsLive must be false (down-folds-no-error contract)")

		// Leg 3: STOPPED. Flip the server back to up (the server is fine). The
		// WORKER's desired State is "stopped", so the scrape must be skipped
		// entirely: Stopped=true, Scan is the zero value, AND the /metrics hit
		// counter must NOT increment on this leg (the Stopped skip beats a
		// live server).
		st.up.Store(true)
		metricsHitsBeforeLeg3 := st.metricsHits.Load()

		obs3 := cos(ctx, worker, "stopped", port)
		Expect(obs3.Status.Stopped).To(BeTrue(), "leg 3 (stopped): Stopped must be true (the no-scrape contract)")
		Expect(obs3.Status.Scan).To(Equal(benthosmetrics.Scan{}), "leg 3 (stopped): Scan must be the zero value")
		Expect(st.metricsHits.Load()).To(Equal(metricsHitsBeforeLeg3), "leg 3 (stopped): /metrics hit counter must NOT increment (no scrape issued)")

		// Leg 4: RECOVERY. desired State back to "running", same port. After
		// the stopped and down legs, the worker must return to a healthy Scan
		// with no stale state leaking from legs 2 or 3.
		obs4 := cos(ctx, worker, "running", port)
		Expect(obs4.Status.Stopped).To(BeFalse(), "leg 4 (recovery): Stopped must be false")
		Expect(obs4.Status.Scan.MetricsAvailable).To(BeTrue(), "leg 4 (recovery): MetricsAvailable must be true (recovery, no stale state)")
		Expect(obs4.Status.Scan.HealthCheck.IsLive).To(BeTrue(), "leg 4 (recovery): IsLive must be true")
		Expect(obs4.Status.Scan.Metrics.InputReceivedTotal()).To(Equal(int64(18)), "leg 4 (recovery): InputReceivedTotal must be 18 (back to healthy)")
	})
})
