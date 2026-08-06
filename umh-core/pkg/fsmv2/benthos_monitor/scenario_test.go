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

package fsmv2benthosmonitor_test

// Scenario test: drive the REAL worker through the REAL framework entry points,
// against a real HTTP server on the four scraped endpoints.
//
// Every other test in this package calls Poll, the window, or mapObserved
// directly. None of them proves the worker can be built by the framework and
// observed the way the supervisor observes it every tick. This test closes that
// gap without a container:
//
//	factory.NewWorkerByType("benthos_monitor", …)  ← production instantiation
//	  → worker.DeriveDesiredState(config.UserSpec{Config: <yaml>})
//	      ← the same YAML the adapter's cfgFor emits into a child spec
//	  → worker.CollectObservedState(ctx, desired)
//	      ← what the observation collector calls on every tick: Poll → Health →
//	        Observation
//
// A wiring regression anywhere on that path (worker not registered, config not
// round-tripping through YAML, Poll not reached, Health not applied) fails here.
// The container rig's liveness probe covers the same ground in production; this
// is that assertion at CI speed.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2benthosmonitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthos_monitor"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"gopkg.in/yaml.v3"
)

// benthosStub serves the four endpoints a real benthos exposes. The counters are
// atomic so a test can raise them between observations, the way a live benthos
// would as messages flow.
type benthosStub struct {
	server *httptest.Server
	input  atomic.Int64
	output atomic.Int64
}

func newBenthosStub() *benthosStub {
	s := &benthosStub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"error":"","statuses":[]}`))
		case "/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"4.95.0","built":"scenario"}`))
		case "/metrics":
			w.Header().Set("Content-Type", "text/plain")
			// Real exposition shape: per-leaf series with a path label and no
			// top-level aggregate, so the parser must sum across them.
			fmt.Fprintf(w, "# HELP input_received Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE input_received counter\n")
			fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", s.input.Load())
			fmt.Fprintf(w, "# HELP output_sent Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE output_sent counter\n")
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output.broker.0\"} %d\n", s.output.Load()/2)
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output.broker.1\"} %d\n", s.output.Load()-s.output.Load()/2)
		default:
			http.NotFound(w, r)
		}
	}))

	return s
}

func (s *benthosStub) port() uint16 {
	return uint16(s.server.Listener.Addr().(*net.TCPAddr).Port)
}

func (s *benthosStub) close() { s.server.Close() }

// userSpecFor renders the child spec the adapter's cfgFor would produce: the
// config is YAML with the yaml-tag keys, which is what the worker unmarshals
// back into a config.BenthosMonitorConfig. Using YAML here (not JSON) keeps this
// test honest about the round-trip the adapter actually performs.
func userSpecFor(t *testing.T, name string, port uint16) fsmv2config.UserSpec {
	t.Helper()

	raw, err := yaml.Marshal(config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: name, DesiredFSMState: "active"},
		MetricsPort:       port,
	})
	if err != nil {
		t.Fatalf("marshal child config: %v", err)
	}

	return fsmv2config.UserSpec{Config: string(raw)}
}

// observe runs one full framework observation cycle and returns the worker's
// stored status, exactly as the collector would persist it.
func observe(t *testing.T, w fsmv2.Worker, spec fsmv2config.UserSpec) simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus] {
	t.Helper()

	desired, err := w.DeriveDesiredState(spec)
	if err != nil {
		t.Fatalf("DeriveDesiredState: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	observed, err := w.CollectObservedState(ctx, desired)
	if err != nil {
		t.Fatalf("CollectObservedState: %v", err)
	}

	// The observation is generic to the framework; round-trip its JSON into the
	// worker's stored status type, which is what the adapter reads back out of
	// CSE. A field the worker fails to publish cannot survive this.
	blob, err := json.Marshal(observed)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}

	var status simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]
	if err := json.Unmarshal(blob, &status); err != nil {
		t.Fatalf("unmarshal observation into the worker status: %v", err)
	}

	return status
}

// TestScenarioWorkerObservedThroughFramework builds the worker the way production
// builds it and observes it the way the supervisor observes it.
func TestScenarioWorkerObservedThroughFramework(t *testing.T) {
	stub := newBenthosStub()
	defer stub.close()

	stub.input.Store(100)
	stub.output.Store(80)

	spec := userSpecFor(t, "scenario-benthos", stub.port())

	// Production instantiation: the registry must know this worker type, and its
	// factory must build a usable worker. A missing registration fails here.
	worker, err := factory.NewWorkerByType(
		fsmv2benthosmonitor.WorkerType,
		deps.Identity{ID: "scenario-benthos", Name: "scenario-benthos", WorkerType: fsmv2benthosmonitor.WorkerType},
		deps.NewNopFSMLogger(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("factory.NewWorkerByType(%q): %v", fsmv2benthosmonitor.WorkerType, err)
	}
	if worker == nil {
		t.Fatal("factory returned a nil worker")
	}

	// --- observation 1: the cold tick -------------------------------------
	// The scrape must reach all four endpoints and land in the status. With a
	// single window sample the rate is not yet computable, so the worker reports
	// no throughput and stays inactive — never FSMv1's cumulative-count-as-rate.
	first := observe(t, worker, spec)

	if first.Degraded {
		t.Errorf("cold observation is degraded (%q); a reachable benthos must be healthy", first.Reason)
	}
	if first.Result.ScrapedAt.IsZero() {
		t.Error("ScrapedAt is zero: Poll did not record a scrape time through the framework")
	}
	if !first.Result.PingAlive {
		t.Error("PingAlive is false: /ping was not scraped through the framework")
	}
	if !first.Result.Ready {
		t.Error("Ready is false: /ready was not scraped or its empty error field was misread")
	}
	if first.Result.Version != "4.95.0" {
		t.Errorf("Version = %q, want %q: /version did not reach the status", first.Result.Version, "4.95.0")
	}
	// 100 input, and output summed across the two broker legs (40 + 40).
	if first.Result.BenthosMetrics.InputReceived != 100 {
		t.Errorf("InputReceived = %d, want 100", first.Result.BenthosMetrics.InputReceived)
	}
	if first.Result.BenthosMetrics.OutputSent != 80 {
		t.Errorf("OutputSent = %d, want 80 (summed across both broker legs, not last-wins)", first.Result.BenthosMetrics.OutputSent)
	}
	if first.Result.Input.MessagesPerSecond != 0 {
		t.Errorf("cold Input.MessagesPerSecond = %v, want 0 (one sample cannot be delta-ed)", first.Result.Input.MessagesPerSecond)
	}
	if first.Result.IsActive {
		t.Error("cold IsActive is true; a single sample must read inactive")
	}
	if first.Result.Input.LastCount != 100 {
		t.Errorf("cold Input.LastCount = %d, want 100 (it gates every MC throughput panel)", first.Result.Input.LastCount)
	}

	// --- observation 2: traffic arrives -----------------------------------
	// The window lives in the worker's deps and must survive across observations
	// (it is built once per instance). Raising the counters and observing again
	// must move LastCount and flip IsActive on.
	stub.input.Store(140)
	stub.output.Store(120)

	second := observe(t, worker, spec)

	if second.Degraded {
		t.Errorf("second observation is degraded (%q)", second.Reason)
	}
	if second.Result.Input.LastCount != 140 {
		t.Errorf("Input.LastCount = %d, want 140: the window did not survive the previous observation", second.Result.Input.LastCount)
	}
	if second.Result.Output.LastCount != 120 {
		t.Errorf("Output.LastCount = %d, want 120", second.Result.Output.LastCount)
	}
	if second.Result.Input.MessagesPerSecond <= 0 {
		t.Errorf("Input.MessagesPerSecond = %v, want > 0 after a 40-message rise", second.Result.Input.MessagesPerSecond)
	}
	if !second.Result.IsActive {
		t.Error("IsActive is false after input rose: the tick-free derivation did not reach the status")
	}
}

// TestScenarioUnreachableBenthosIsDegraded pins the other half of the contract:
// when the scraped benthos is gone, the framework must drive the worker degraded
// rather than publishing a healthy status with stale numbers. This is the
// condition the adapter maps to the degraded operational state.
//
// Which layer produces the verdict matters, so it is stated here rather than
// left to be rediscovered: a failed scrape makes Poll return an error, and the
// framework marks the observation degraded on its poll-error path
// (simple/worker.go) WITHOUT consulting the worker's Health function. Deleting
// Health does not change this test's outcome — verified by mutation. Health is
// exercised at the unit level (TestHealthReproducesIsMonitorHealthy), and
// staleness of an otherwise-successful scrape is caught one layer up by the
// adapter's freshness ladder (staleAfter = 3x the observation interval), which is
// where the design places it.
func TestScenarioUnreachableBenthosIsDegraded(t *testing.T) {
	stub := newBenthosStub()
	port := stub.port()
	stub.close() // nothing is listening now

	spec := userSpecFor(t, "scenario-dead", port)

	worker, err := factory.NewWorkerByType(
		fsmv2benthosmonitor.WorkerType,
		deps.Identity{ID: "scenario-dead", Name: "scenario-dead", WorkerType: fsmv2benthosmonitor.WorkerType},
		deps.NewNopFSMLogger(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("factory.NewWorkerByType: %v", err)
	}

	desired, err := worker.DeriveDesiredState(spec)
	if err != nil {
		t.Fatalf("DeriveDesiredState: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	observed, err := worker.CollectObservedState(ctx, desired)
	if err != nil {
		t.Fatalf("CollectObservedState returned a hard error: %v", err)
	}

	blob, err := json.Marshal(observed)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}

	var status simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]
	if err := json.Unmarshal(blob, &status); err != nil {
		t.Fatalf("unmarshal observation: %v", err)
	}

	if !status.Degraded {
		t.Error("an unreachable benthos produced a non-degraded observation: a dead scrape must not read healthy")
	}
	if status.Result.IsActive {
		t.Error("an unreachable benthos reported IsActive")
	}
}
