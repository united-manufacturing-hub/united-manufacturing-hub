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
	"reflect"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// TestPollRefusesToScrapeWithUnboundDeps asserts that Poll reports unbound deps
// as an error instead of scraping with a substitute client.
//
// The contract exists because simpleWorker.pollDeps (pkg/fsmv2/simple) discards
// the comma-ok of its type assertion, so deps that were never bound — or bound
// to another type — reach Poll as a nil *benthosMonitorDeps rather than
// panicking. A fallback client would then scrape a healthy monitor successfully
// but drop the cross-poll throughput window on the floor, publishing zero Input,
// Output and IsActive: a wiring fault rendered as plausible idle traffic. That is
// why the monitor served below is fully healthy — the fault has to be caught on a
// scrape that would otherwise succeed, not on an unreachable port where any
// client fails anyway.
func TestPollRefusesToScrapeWithUnboundDeps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			_, _ = w.Write([]byte(`{"error":"","statuses":[]}`))
		case "/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/metrics":
			_, _ = w.Write([]byte(`input_received{label="",path="root.input"} 42
output_sent{label="",path="root.output"} 7
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: "benthos-1", DesiredFSMState: "active"},
		MetricsPort:       uint16(srv.Listener.Addr().(*net.TCPAddr).Port),
	}

	t.Run("nil deps", func(t *testing.T) {
		status, err := Poll(context.Background(), nil, cfg)
		if err == nil {
			t.Fatalf("Poll with nil deps returned nil error, want an error (unbound deps must not scrape)")
		}
		if !reflect.DeepEqual(status, BenthosMonitorStatus{}) {
			t.Errorf("Poll with nil deps returned %+v, want the zero status", status)
		}
	})

	t.Run("deps with no client", func(t *testing.T) {
		status, err := Poll(context.Background(), &benthosMonitorDeps{}, cfg)
		if err == nil {
			t.Fatalf("Poll with a clientless deps returned nil error, want an error (unbound deps must not scrape)")
		}
		if !reflect.DeepEqual(status, BenthosMonitorStatus{}) {
			t.Errorf("Poll with a clientless deps returned %+v, want the zero status", status)
		}
	})

	// The control: deps carrying a client is the bound case, and the guard must
	// not fire on it. Without this the two cases above would still pass if the
	// guard rejected every call.
	t.Run("bound deps reach the scrape", func(t *testing.T) {
		status, err := Poll(context.Background(), &benthosMonitorDeps{client: &http.Client{Timeout: monitorClientTimeout}}, cfg)
		if err != nil {
			t.Fatalf("Poll with bound deps errored: %v (the deps guard must not fire on bound deps)", err)
		}
		if status.Version != "1.2.3" {
			t.Errorf("Version = %q, want %q (bound deps must scrape the same monitor the cases above were refused)", status.Version, "1.2.3")
		}
	})
}
