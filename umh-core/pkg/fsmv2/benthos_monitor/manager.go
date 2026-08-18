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

// Package fsmv2benthosmonitor is the benthos monitor built on the fsmv2 simple
// framework. The worker registers itself on import. Poll scrapes the monitor's
// HTTP endpoints into a BenthosMonitorStatus; Poll's godoc names the set.
package fsmv2benthosmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"

	"gopkg.in/yaml.v3"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE
	// (Convergent State Engine) storage.
	WorkerType = "benthos_monitor"

	// pollInterval is the cadence at which the framework calls Poll.
	pollInterval = 1 * time.Second

	// monitorClientTimeout bounds each scrape request so one hung connection
	// cannot consume the whole observation budget and starve the remaining
	// endpoints. Four endpoints at ~400ms each fit the ~2.2s budget
	// (supervisor.DefaultObservationTimeout); the fsmv1 curl bound of 1s would
	// let a single poll run past it.
	monitorClientTimeout = 400 * time.Millisecond

	// maxScrapeBody caps the response bodies Poll reads, so a misbehaving
	// monitor cannot be buffered without bound on each poll.
	maxScrapeBody = 4 << 20 // 4 MiB
)

// benthosMonitorDeps holds the HTTP client used to scrape the benthos monitor's
// endpoints and the throughput window that accumulates counter samples across
// polls. Poll receives a pointer to the deps, so the same *http.Client is
// shared across polls.
type benthosMonitorDeps struct {
	client *http.Client
	window throughputWindow
}

// ComponentThroughput carries the rate for one direction of the benthos
// pipeline. MessagesPerSecond is the by-time rate the worker computes; LastCount
// is the newest counter value seen.
type ComponentThroughput struct {
	MessagesPerSecond float64
	LastCount         int
}

// BenthosMonitorStatus is the result of one scrape of the benthos monitor: the
// time it happened, the /metrics counters, /ping liveness, /ready readiness, the
// /version string, and the per-direction throughput computed over windowSpan.
//
// BenthosMetrics is the per-path struct the fsmv1 parser produces, carried
// whole. Narrowing it to scalars breaks the four connection-counter reads that
// decide whether a bridge is connected; those reads, their access path and their
// two consumers are named in
// TestMapObservedDeliversParsedConnectionCountersToBridgeHealth.
type BenthosMonitorStatus struct {
	ScrapedAt      time.Time
	BenthosMetrics benthosmonitorservice.Metrics
	Input          ComponentThroughput
	Output         ComponentThroughput
	// IsActive is true when input traffic was observed in the window: Poll sets it
	// from Input.MessagesPerSecond > 0, input-only and with no hysteresis. FSMv1
	// computed the same rule from a tick delta (s.IsActive =
	// s.Input.MessagesPerTick > 0).
	IsActive  bool
	PingAlive bool
	Ready     bool
	Version   string
}

// Poll scrapes the configured benthos monitor's /ping, /ready, /version, and
// /metrics endpoints once, using the HTTP client d carries. All requests carry
// ctx, so cancellation surfaces as a request error. Deps that were never bound
// are rejected before any request goes out: substituting a client would report a
// wiring fault as ordinary idle traffic.
func Poll(ctx context.Context, d *benthosMonitorDeps, cfg config.BenthosMonitorConfig) (BenthosMonitorStatus, error) {
	if d == nil || d.client == nil {
		return BenthosMonitorStatus{}, fmt.Errorf("benthos monitor dependencies not bound (deps present: %t, HTTP client present: %t): worker wiring fault, not a scrape failure", d != nil, d != nil && d.client != nil)
	}

	base := fmt.Sprintf("http://localhost:%d", cfg.MetricsPort)
	status := BenthosMonitorStatus{ScrapedAt: time.Now()}

	_, _, err := get(ctx, d.client, base+"/ping")
	if err == nil {
		status.PingAlive = true
	}

	readyBody, _, err := get(ctx, d.client, base+"/ready")
	if err == nil {
		// The /ready endpoint reports readiness by returning JSON whose error
		// field is empty when every input/output connection is up (fsmv1
		// benthosmonitorservice.ParseReadyData: readyResp.Error == ""). A benthos
		// that answers but is not ready (e.g. a broken pipeline that still answers
		// /ping) reports Ready=false while PingAlive=true.
		var r struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(readyBody, &r); err == nil {
			status.Ready = r.Error == ""
		}
	}

	versionBody, _, err := get(ctx, d.client, base+"/version")
	if err != nil {
		return status, fmt.Errorf("scrape /version: %w", err)
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(versionBody, &v); err != nil {
		return status, fmt.Errorf("decode /version: %w", err)
	}
	status.Version = v.Version

	metricsBody, _, err := get(ctx, d.client, base+"/metrics")
	if err != nil {
		return status, fmt.Errorf("scrape /metrics: %w", err)
	}
	m, err := benthosmonitorservice.ParseMetricsFromBytes(metricsBody)
	if err != nil {
		return status, fmt.Errorf("parse /metrics: %w", err)
	}
	status.BenthosMetrics = m

	// The window lives in d and survives across polls, so a rate is computable
	// from per-poll counter snapshots. It is keyed on the scrape port: an
	// in-place config update can re-point this child at a new MetricsPort with no
	// worker restart, and throughputWindow.port explains the wipe that follows.
	d.window.Add(status.ScrapedAt, int(cfg.MetricsPort), int(m.InputReceivedTotal()), int(m.OutputSentTotal()))
	status.Input = ComponentThroughput{
		MessagesPerSecond: d.window.inputRate(),
		LastCount:         d.window.inputCount(),
	}
	status.Output = ComponentThroughput{
		MessagesPerSecond: d.window.outputRate(),
		LastCount:         d.window.outputCount(),
	}
	status.IsActive = status.Input.MessagesPerSecond > 0

	return status, nil
}

// get performs a GET request with ctx, returning the body, the HTTP status, and
// any error. A non-2xx response is returned as an error: a benthos that answers
// a 500 error page is not healthy, and parsing its HTML as /metrics or /version
// yields garbage.
func get(ctx context.Context, client *http.Client, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	// Unreachable via net/http: Do returns a non-nil error whenever resp is nil,
	// so the check above already covers it. The guard stays as a cheap floor on
	// the dereferences below.
	if resp == nil {
		return nil, 0, fmt.Errorf("GET %s: nil response", url)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("GET %s: unexpected status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxScrapeBody+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(body) > maxScrapeBody {
		return nil, resp.StatusCode, fmt.Errorf("GET %s: response body exceeds %d bytes", url, maxScrapeBody)
	}
	return body, resp.StatusCode, nil
}

// cfgFor renders a BenthosMonitorConfig into the map the adapter hands to
// dynamicchildren.Writer.Upsert, through YAML, so the keys match the config's
// yaml tags.
//
// Upsert marshals that map into ChildSpec.UserSpec.Config as YAML (see
// NewChildSpec in pkg/fsmv2/config) and the worker unmarshals it back. The
// adapter's default CfgFor is a JSON round-trip; BenthosMonitorConfig carries no
// json tags, so it would emit Go field names and lose Name, DesiredFSMState, and
// MetricsPort.
func cfgFor(cfg config.BenthosMonitorConfig) (map[string]any, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal benthos monitor config: %w", err)
	}

	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal benthos monitor config to map: %w", err)
	}

	return out, nil
}

func init() {
	simple.Register(simple.MonitorSpec[config.BenthosMonitorConfig, BenthosMonitorStatus, *benthosMonitorDeps]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		NewDeps: func(_ deps.Identity, _ *deps.BaseDependencies) *benthosMonitorDeps {
			return &benthosMonitorDeps{client: &http.Client{Timeout: monitorClientTimeout}}
		},
		Poll:   Poll,
		Health: health,
	})
}

// NewFsmv2BenthosMonitorManager builds the fsmv1-compatible manager that drives
// the fsmv2 benthos monitor workers. It extracts BenthosMonitorConfig entries
// from the snapshot, upserts enabled workers into the fsmv2 runtime, and maps
// their stored status back to the fsmv1 benthos monitor operational states.
func NewFsmv2BenthosMonitorManager(managerName string) *adapter.WorkerManager[config.BenthosMonitorConfig, simple.Status[BenthosMonitorStatus]] {
	return adapter.NewWorkerManager(adapter.WorkerManagerSpec[config.BenthosMonitorConfig, simple.Status[BenthosMonitorStatus]]{
		WorkerType: WorkerType,
		Log:        deps.NewFSMLogger(logger.For(managerName)),
		ExtractConfigs: func(s publicfsm.SystemSnapshot) []config.BenthosMonitorConfig {
			return s.CurrentConfig.Internal.BenthosMonitor
		},
		NameOf:      func(c config.BenthosMonitorConfig) string { return c.Name },
		CfgFor:      cfgFor,
		MapFresh:    mapFresh,
		MapObserved: mapObserved,
		// DesiredRunning is the state reported when a config leaves desiredState
		// empty. benthos_monitor's FSM accepts only active/stopped as a desired
		// state — SetDesiredFSMState in pkg/fsm/benthos_monitor/machine.go rejects
		// anything else.
		States: adapter.StateVocabulary{
			Starting:       benthosmonitorfsm.OperationalStateStarting,
			Degraded:       benthosmonitorfsm.OperationalStateDegraded,
			Stopped:        benthosmonitorfsm.OperationalStateStopped,
			DesiredRunning: benthosmonitorfsm.OperationalStateActive,
		},
	})
}

// mapFresh maps a Fresh, healthy observation to its fsmv1 operational state:
// metrics are OK, so the monitor is active. The Degraded, stale and bootstrap
// verdicts are owned by adapter.WorkerManager (its verdict-to-state table is in
// pkg/fsmv2/adapter/doc.go), so this only classifies the healthy leaf.
func mapFresh(_ config.BenthosMonitorConfig, _ simple.Status[BenthosMonitorStatus]) string {
	return benthosmonitorfsm.OperationalStateActive
}
