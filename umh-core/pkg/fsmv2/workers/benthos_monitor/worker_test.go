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

// sampleMetrics is copied verbatim from
// pkg/service/benthos_monitor/benthos_monitor_benchmark_test.go's sampleMetrics
// var (input_received=18, output_sent=18).
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

var _ = Describe("BenthosMonitorWorker CollectObservedState", func() {
	var (
		logger deps.FSMLogger
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
	})

	Describe("scraping a live benthos stub", func() {
		It("publishes a BenthosMonitorStatus carrying the parsed Scan", func() {
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
			DeferCleanup(srv.Close)

			port := portFromServerURL(srv.URL)

			identity := deps.Identity{ID: "benthos-monitor-test", WorkerType: "benthos_monitor"}
			worker, err := benthos_monitor.NewBenthosMonitorWorker(identity, logger, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())

			desired := &fsmv2.WrappedDesiredState[benthos_monitor.BenthosMonitorConfig]{
				Config: benthos_monitor.BenthosMonitorConfig{
					MetricsPort: port,
				},
			}

			obs, err := worker.CollectObservedState(context.Background(), desired)

			Expect(err).NotTo(HaveOccurred())
			typedObs, ok := obs.(fsmv2.Observation[benthos_monitor.BenthosMonitorStatus])
			Expect(ok).To(BeTrue(), "expected fsmv2.Observation[BenthosMonitorStatus], got %T", obs)
			Expect(typedObs.Status.Scan.MetricsAvailable).To(BeTrue())
			Expect(typedObs.Status.Scan.HealthCheck.IsLive).To(BeTrue())
			Expect(typedObs.Status.Scan.Metrics.InputReceivedTotal()).To(Equal(int64(18)))
			Expect(typedObs.Status.Scan.Metrics.OutputSentTotal()).To(Equal(int64(18)))
		})
	})

	Describe("with a cancelled context", func() {
		It("returns context.Canceled and a nil observation", func() {
			identity := deps.Identity{ID: "benthos-monitor-cancel", WorkerType: "benthos_monitor"}
			worker, err := benthos_monitor.NewBenthosMonitorWorker(identity, logger, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			desired := &fsmv2.WrappedDesiredState[benthos_monitor.BenthosMonitorConfig]{
				Config: benthos_monitor.BenthosMonitorConfig{
					MetricsPort: 1, // unreachable, but cancel wins first
				},
			}

			obs, err := worker.CollectObservedState(ctx, desired)
			Expect(err).To(Equal(context.Canceled))
			Expect(obs).To(BeNil())
		})
	})

	Describe("scraping an unreachable port", func() {
		It("reports a down benthos as an observed state with a nil error", func() {
			identity := deps.Identity{ID: "benthos-monitor-down", WorkerType: "benthos_monitor"}
			worker, err := benthos_monitor.NewBenthosMonitorWorker(identity, logger, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			desired := &fsmv2.WrappedDesiredState[benthos_monitor.BenthosMonitorConfig]{
				Config: benthos_monitor.BenthosMonitorConfig{
					MetricsPort: 1, // nothing listening
				},
			}

			obs, err := worker.CollectObservedState(context.Background(), desired)

			Expect(err).NotTo(HaveOccurred())
			Expect(obs).NotTo(BeNil())
			typedObs, ok := obs.(fsmv2.Observation[benthos_monitor.BenthosMonitorStatus])
			Expect(ok).To(BeTrue(), "expected fsmv2.Observation[BenthosMonitorStatus], got %T", obs)
			Expect(typedObs.Status.Scan.HealthCheck.IsLive).To(BeFalse())
		})
	})

	Describe("when the userSpec State is stopped", func() {
		It("skips the scrape entirely and does not call Observe", func() {
			readyBody := `{"statuses":[{"label":"tcp_server","path":"root.input","connected":true},{"label":"http_client","path":"root.output","connected":true}]}`
			versionBody := `{"version":"3.71.0","built":"2023-08-15T12:00:00Z"}`

			var pingHits, readyHits, versionHits, metricsHits int32
			mux := http.NewServeMux()
			mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&pingHits, 1)
				_, _ = w.Write([]byte("pong"))
			})
			mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&readyHits, 1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(readyBody))
			})
			mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&versionHits, 1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(versionBody))
			})
			mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&metricsHits, 1)
				w.Header().Set("Content-Type", "text/plain; version=0.0.4")
				_, _ = w.Write([]byte(sampleMetrics))
			})

			srv := httptest.NewServer(mux)
			DeferCleanup(srv.Close)

			port := portFromServerURL(srv.URL)

			identity := deps.Identity{ID: "benthos-monitor-stopped", WorkerType: "benthos_monitor"}
			worker, err := benthos_monitor.NewBenthosMonitorWorker(identity, logger, nil, http.DefaultClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())

			desired := &fsmv2.WrappedDesiredState[benthos_monitor.BenthosMonitorConfig]{
				Config: benthos_monitor.BenthosMonitorConfig{
					BaseUserSpec: config.BaseUserSpec{State: "stopped"},
					MetricsPort:  port,
				},
			}

			obs, err := worker.CollectObservedState(context.Background(), desired)

			Expect(err).NotTo(HaveOccurred())
			typedObs, ok := obs.(fsmv2.Observation[benthos_monitor.BenthosMonitorStatus])
			Expect(ok).To(BeTrue(), "expected fsmv2.Observation[BenthosMonitorStatus], got %T", obs)
			Expect(typedObs.Status.Scan).To(Equal(benthosmetrics.Scan{}), "a stopped worker must return a fully zero Scan with no stale fields leaking through")
			Expect(atomic.LoadInt32(&pingHits)).To(Equal(int32(0)), "Observe must NOT be called for a stopped worker (no /ping request)")
			Expect(atomic.LoadInt32(&readyHits)).To(Equal(int32(0)), "Observe must NOT be called for a stopped worker (no /ready request)")
			Expect(atomic.LoadInt32(&versionHits)).To(Equal(int32(0)), "Observe must NOT be called for a stopped worker (no /version request)")
			Expect(atomic.LoadInt32(&metricsHits)).To(Equal(int32(0)), "Observe must NOT be called for a stopped worker (no /metrics request)")
		})
	})

})

// portFromServerURL extracts the port from an httptest server URL.
func portFromServerURL(rawURL string) uint16 {
	u, err := url.Parse(rawURL)
	Expect(err).NotTo(HaveOccurred())
	p, err := strconv.ParseUint(u.Port(), 10, 16)
	Expect(err).NotTo(HaveOccurred())
	return uint16(p)
}
