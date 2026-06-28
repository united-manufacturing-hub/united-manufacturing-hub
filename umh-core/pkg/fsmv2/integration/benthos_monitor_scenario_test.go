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

package integration_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
	benthos_monitor_state "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"

	// Blank-import the kernel config worker and its state so their init()
	// registrations exist before the supervisor ticks. The benthos_monitor
	// worker and state packages are imported by name above (for their types),
	// which already runs their init(), so they are not re-imported here.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
)

// sampleBenthosMetrics is the raw Prometheus text body a live benthos instance
// serves at /metrics. It mirrors the benchmark fixture in
// pkg/service/benthos_monitor/benthos_monitor_benchmark_test.go and yields
// InputReceivedTotal()==18 once parsed.
const sampleBenthosMetrics = `# HELP input_connection_failed Benthos Counter metric
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

// altBenthosMetrics is a second metrics body served by the port-change stub in
// leg E. Its input_received counter is 42, distinct from the primary stub's 18,
// so the test can prove the Scan reflects the new port.
const altBenthosMetrics = `# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 42
# HELP input_latency_ns Benthos Timing metric
# TYPE input_latency_ns summary
input_latency_ns_sum{label="",path="root.input"} 100
input_latency_ns_count{label="",path="root.input"} 42
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output"} 42
`

const benthosReadyBody = `{"statuses":[{"label":"tcp_server","path":"root.input","connected":true},{"label":"http_client","path":"root.output","connected":true}]}`
const benthosVersionBody = `{"version":"3.71.0","built":"2023-08-15T12:00:00Z"}`

// stubState carries the swappable behavior of the benthos stub server. When
// down is set, every endpoint hijacks and closes the TCP connection so the
// worker's http.Client receives a transport error (the J2 fold path).
// metricsHits counts only successful 200 /metrics responses, so a Stopped
// worker (whose COS guard skips the scrape) leaves it unchanged.
type stubState struct {
	down        atomic.Bool
	metricsHits atomic.Int64
	metricsBody string
}

func newBenthosStub(metricsBody string) (*httptest.Server, *stubState) {
	st := &stubState{metricsBody: metricsBody}
	mux := http.NewServeMux()

	closeConn := func(w http.ResponseWriter) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}

		conn, _, _ := hj.Hijack()
		if conn != nil {
			_ = conn.Close()
		}
	}

	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		if st.down.Load() {
			closeConn(w)

			return
		}

		_, _ = w.Write([]byte("pong"))
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if st.down.Load() {
			closeConn(w)

			return
		}

		_, _ = w.Write([]byte(benthosReadyBody))
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		if st.down.Load() {
			closeConn(w)

			return
		}

		_, _ = w.Write([]byte(benthosVersionBody))
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		if st.down.Load() {
			closeConn(w)

			return
		}

		st.metricsHits.Add(1)
		_, _ = w.Write([]byte(st.metricsBody))
	})

	srv := httptest.NewServer(mux)

	return srv, st
}

// stubPort extracts the TCP port from a httptest server's listener address.
func stubPort(srv *httptest.Server) uint16 {
	_, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	Expect(err).ToNot(HaveOccurred())

	port64, err := strconv.ParseUint(portStr, 10, 16)
	Expect(err).ToNot(HaveOccurred())

	return uint16(port64)
}

var _ = Describe("benthos_monitor worker end-to-end across benthos states", func() {
	const (
		configWorkerKey = "configworker"
		childName       = "bm-scenario"
	)

	var (
		ctx      context.Context
		cancel   context.CancelFunc
		stubSrv  *httptest.Server
		stubSrv2 *httptest.Server
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		register.ClearDeps(configWorkerKey)
		if stubSrv != nil {
			stubSrv.Close()
			stubSrv = nil
		}
		if stubSrv2 != nil {
			stubSrv2.Close()
			stubSrv2 = nil
		}
	})

	It("drives create, down, stopped, recovery, port-change and despawn", func() {
		logger := deps.NewNopFSMLogger()

		stubSrv, stub := newBenthosStub(sampleBenthosMetrics)
		port := stubPort(stubSrv)

		reg := dynamicchildren.NewWriter()
		register.SetDeps[*dynamicchildren.Registry](configWorkerKey, reg.Registry())

		s, store, _ := newAppSupervisorWithStore(logger)
		s.TestMarkAsStarted()
		cli := fsmv2client.NewFSMv2Client(reg, store)
		ref := dynamicchildren.Ref{WorkerType: benthos_monitor.WorkerTypeName, Name: childName}

		// tickLoad ticks the supervisor once and returns the latest observation
		// or its load error. ErrNotObserved (child not yet observed) is returned
		// to the caller so the Eventually predicate can treat it as retry.
		tickLoad := func() (fsmv2.Observation[benthos_monitor.BenthosMonitorStatus], error) {
			_ = s.TestTick(ctx)

			return fsmv2client.Get[benthos_monitor.BenthosMonitorStatus](ctx, cli, ref)
		}

		// Leg A: create -> Running -> Fresh Scan.
		Expect(cli.Upsert(ref, map[string]any{"state": "running", "metricsPort": port})).To(Succeed())

		var t1 time.Time
		Eventually(func(g Gomega) bool {
			obs, err := tickLoad()
			if err != nil {
				return false
			}
			if obs.State != benthos_monitor_state.StateNameRunning {
				return false
			}
			if obs.Status.Stopped {
				return false
			}
			if !obs.Status.Scan.MetricsAvailable || !obs.Status.Scan.HealthCheck.IsLive {
				return false
			}
			if obs.Status.Scan.Metrics.InputReceivedTotal() != 18 {
				return false
			}
			if obs.GetTimestamp().IsZero() {
				return false
			}
			t1 = obs.GetTimestamp()

			return true
		}, "10s", "50ms").Should(BeTrue(),
			"leg A: worker must reach Running with a fresh, healthy Scan (InputReceivedTotal==18)")

		// Leg B: benthos down -> Fresh + IsLive=false (NOT Stale). CollectedAt
		// must advance across the down-tick (T2 > T1).
		stub.down.Store(true)
		Expect(cli.Upsert(ref, map[string]any{"state": "running", "metricsPort": port})).To(Succeed())

		var t2 time.Time
		Eventually(func(g Gomega) bool {
			obs, err := tickLoad()
			if err != nil {
				return false
			}
			if obs.Status.Scan.HealthCheck.IsLive {
				return false
			}
			if obs.Status.Scan.MetricsAvailable {
				return false
			}
			ts := obs.GetTimestamp()
			if ts.IsZero() || !ts.After(t1) {
				return false
			}
			t2 = ts

			return true
		}, "10s", "50ms").Should(BeTrue(),
			"leg B: a down benthos must yield IsLive=false, MetricsAvailable=false, "+
				"with CollectedAt advanced across the down-tick (Fresh, not Stale)")

		// Leg C: state=stopped -> Fresh + Stopped=true (NOT Stale). The COS guard
		// skips the scrape, so the /metrics hit counter must not advance on the
		// post-Stopped ticks. Flip the server back up first so a scrape WOULD
		// succeed if the guard did not short-circuit.
		stub.down.Store(false)
		Expect(cli.Upsert(ref, map[string]any{"state": "stopped", "metricsPort": port})).To(Succeed())

		Eventually(func(g Gomega) bool {
			obs, err := tickLoad()
			if err != nil {
				return false
			}

			return obs.Status.Stopped && obs.GetTimestamp().After(t2)
		}, "10s", "50ms").Should(BeTrue(),
			"leg C: a stopped worker must report Stopped=true with CollectedAt advanced")

		hitsBeforeStop := stub.metricsHits.Load()
		var t3 time.Time
		// Drive a few more ticks while Stopped; CollectedAt keeps advancing (the
		// collector COS's regardless of FSM phase) but /metrics is never hit.
		Eventually(func(g Gomega) bool {
			obs, err := tickLoad()
			if err != nil {
				return false
			}
			ts := obs.GetTimestamp()
			if !obs.Status.Stopped || ts.IsZero() {
				return false
			}
			if ts.After(t2) {
				t3 = ts

				return true
			}

			return false
		}, "5s", "50ms").Should(BeTrue(),
			"leg C: the Stopped worker's CollectedAt must keep advancing while the scrape stays skipped")
		Expect(stub.metricsHits.Load()).To(Equal(hitsBeforeStop),
			"leg C: the /metrics endpoint must not be hit while the worker is Stopped")

		// Leg D: recovery -> Fresh + healthy.
		Expect(cli.Upsert(ref, map[string]any{"state": "running", "metricsPort": port})).To(Succeed())
		Eventually(func(g Gomega) bool {
			obs, err := tickLoad()
			if err != nil {
				return false
			}

			return obs.State == benthos_monitor_state.StateNameRunning &&
				obs.Status.Scan.HealthCheck.IsLive &&
				obs.Status.Scan.MetricsAvailable &&
				obs.Status.Scan.Metrics.InputReceivedTotal() == 18 &&
				obs.GetTimestamp().After(t3)
		}, "10s", "50ms").Should(BeTrue(),
			"leg D: after recovery the worker must scrape healthy again (InputReceivedTotal==18)")

		// Leg E: port-change -> Scan reflects the NEW stub's counters.
		stubSrv2, stub2 := newBenthosStub(altBenthosMetrics)
		port2 := stubPort(stubSrv2)
		Expect(cli.Upsert(ref, map[string]any{"state": "running", "metricsPort": port2})).To(Succeed())
		Eventually(func(g Gomega) bool {
			obs, err := tickLoad()
			if err != nil {
				return false
			}

			return obs.State == benthos_monitor_state.StateNameRunning &&
				obs.Status.Scan.MetricsAvailable &&
				obs.Status.Scan.Metrics.InputReceivedTotal() == 42
		}, "10s", "50ms").Should(BeTrue(),
			"leg E: after a port change the Scan must reflect the new stub (InputReceivedTotal==42)")
		Expect(stub2.metricsHits.Load()).To(BeNumerically(">", 0),
			"leg E: the new stub must actually have been scraped")

		// Leg F: idempotent Upsert keeps one child, then Delete despawns.
		// GetChildren at the app supervisor level is flat: it includes both the
		// config-worker kernel child and the spawned benthos_monitor child, so
		// "still one benthos_monitor child" means exactly one entry keyed by
		// childName and that child is Running.
		spec := map[string]any{"state": "running", "metricsPort": port2}
		Expect(cli.Upsert(ref, spec)).To(Succeed())
		Expect(cli.Upsert(ref, spec)).To(Succeed())
		Eventually(func(g Gomega) bool {
			_ = s.TestTick(ctx)
			child, ok := s.GetChildren()[childName]
			if !ok || child == nil {
				return false
			}

			return child.GetCurrentStateName() == benthos_monitor_state.StateNameRunning
		}, "5s", "50ms").Should(BeTrue(),
			"leg F: a second idempotent Upsert must keep exactly one benthos_monitor child in Running")

		// Leg F (despawn): Delete exercises the despawn path without error. The
		// store-side reap — the deleted ref returning ErrNotObserved and the
		// worker gone from the store — is gated on the ENG-5107 despawn-tombstone
		// subsystem, which is not built on this branch (the same boundary the
		// dynamic_scenario_v2 test documents). PR2 proves the CollectedAt-
		// advancement foundation; the reap assertion belongs to ENG-5107.
		Expect(func() { cli.Delete(ref) }).ToNot(Panic())
		_ = s.TestTick(ctx) // drive one tick so the delete propagates if the reap were wired
	})
})
