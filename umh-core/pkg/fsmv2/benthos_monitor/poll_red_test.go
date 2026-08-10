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
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

// TestPollScrapesEndpoints forces the real three-endpoint scrape through Poll.
// It serves /ping, /version, and /metrics locally, hands Poll a deps with a
// real client pointed at the dev port, and asserts the returned status carries
// a real scrape time, the served /metrics counters, ping liveness, and the
// version string. The /metrics body uses the real benthos exposition format
// (`name{label=...,path=...}` plus a scientific-notation value), so the parser
// is exercised against the same shape production returns. It also pins that the
// worker status marshals inside a real fsmv2.Observation without error, and
// proves ctx routing: an already-cancelled context must surface an error from
// http.NewRequestWithContext.
func TestPollScrapesEndpoints(t *testing.T) {
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
			_, _ = w.Write([]byte(`# HELP input_received Number of messages received.
# TYPE input_received counter
input_received{label="",path="root.input"} 4.2e+01
# HELP output_sent Number of messages sent.
# TYPE output_sent counter
output_sent{label="",path="root.output"} 7
`))
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
		t.Fatalf("Poll errored: %v", err)
	}

	// (a) real scrape time + raw data populated from the served endpoints.
	if status.ScrapedAt.IsZero() {
		t.Errorf("ScrapedAt is zero; Poll did not record a real scrape time")
	}
	if status.BenthosMetrics.InputReceivedTotal() != 42 {
		t.Errorf("InputReceivedTotal() = %d, want 42 (from served /metrics counter)", status.BenthosMetrics.InputReceivedTotal())
	}
	if status.BenthosMetrics.OutputSentTotal() != 7 {
		t.Errorf("OutputSentTotal() = %d, want 7 (from served /metrics counter)", status.BenthosMetrics.OutputSentTotal())
	}
	if !status.PingAlive {
		t.Errorf("PingAlive = false, want true (/ping returned 200)")
	}
	if !status.Ready {
		t.Errorf("Ready = false, want true (/ready returned an empty error field)")
	}
	if status.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", status.Version, "1.2.3")
	}

	// (b) the status must marshal inside a real Observation without error. The
	// reserved-key collision guard lives at registration time (fsmv2
	// DetectFieldCollisions), not here; this only pins that the status is
	// marshalable end-to-end.
	obs := fsmv2.Observation[simple.Status[BenthosMonitorStatus]]{
		CollectedAt: time.Now(),
		Status:      simple.Status[BenthosMonitorStatus]{Result: status},
	}
	if _, err := json.Marshal(obs); err != nil {
		t.Errorf("marshalling an Observation of the worker status failed: %v", err)
	}

	// (c) ctx routing: an already-cancelled context must surface an error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Poll(ctx, deps, cfg); err == nil {
		t.Errorf("Poll with an already-cancelled context returned nil error; expected context cancellation to surface")
	}
}

// TestPollRejectsNon2xxScrapes pins the status gating in get(): a non-2xx
// /version or /metrics surfaces as a Poll error instead of silently scraping an
// error page, and a non-2xx /ping leaves PingAlive false without aborting the
// rest of the scrape.
func TestPollRejectsNon2xxScrapes(t *testing.T) {
	newServer := func(failPaths map[string]int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if code, ok := failPaths[r.URL.Path]; ok {
				w.WriteHeader(code)
				_, _ = w.Write([]byte("boom"))
				return
			}
			switch r.URL.Path {
			case "/version":
				_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
			default:
				_, _ = w.Write([]byte("ok"))
			}
		}))
	}
	cfgFor := func(srv *httptest.Server) config.BenthosMonitorConfig {
		return config.BenthosMonitorConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{Name: "benthos-1", DesiredFSMState: "active"},
			MetricsPort:       uint16(srv.Listener.Addr().(*net.TCPAddr).Port),
		}
	}
	deps := &benthosMonitorDeps{client: &http.Client{Timeout: 400 * time.Millisecond}}

	t.Run("non-2xx /metrics surfaces an error and preserves partial status", func(t *testing.T) {
		srv := newServer(map[string]int{"/metrics": http.StatusInternalServerError})
		defer srv.Close()
		status, err := Poll(context.Background(), deps, cfgFor(srv))
		if err == nil {
			t.Errorf("Poll with a 500 /metrics returned nil error, want an error (an error page must not be scraped as zero counters)")
		}
		if !status.PingAlive {
			t.Errorf("PingAlive = false, want true (partial status preserved across a /metrics failure)")
		}
		if status.Version != "1.2.3" {
			t.Errorf("Version = %q, want %q (partial status preserved across a /metrics failure)", status.Version, "1.2.3")
		}
	})

	t.Run("non-2xx /ping leaves PingAlive false", func(t *testing.T) {
		srv := newServer(map[string]int{"/ping": http.StatusServiceUnavailable})
		defer srv.Close()
		status, err := Poll(context.Background(), deps, cfgFor(srv))
		if err != nil {
			t.Fatalf("Poll errored: %v", err)
		}
		if status.PingAlive {
			t.Errorf("PingAlive = true, want false (/ping returned 503)")
		}
	})
}
