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
// /ping, /version, and /metrics endpoints into a BenthosMonitorStatus.
package fsmv2benthosmonitor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"

	"gopkg.in/yaml.v3"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "benthos_monitor"

	// pollInterval is the cadence at which the framework calls Poll.
	pollInterval = 1 * time.Second

	// monitorClientTimeout bounds each scrape request so one hung connection
	// cannot consume the whole observation frame and starve the remaining
	// endpoints. Four endpoints at ~400ms each fit the ~2.2s observation budget;
	// the fsmv1 curl bound of 1s would let a single poll run past it.
	monitorClientTimeout = 400 * time.Millisecond

	// maxScrapeBody caps the response bodies Poll reads, so a misbehaving
	// monitor cannot be buffered without bound on each poll.
	maxScrapeBody = 4 << 20 // 4 MiB
)

// benthosMonitorDeps holds the HTTP client used to scrape the benthos monitor's
// endpoints and the throughput window that accumulates counter samples across
// polls. Poll receives a pointer to the deps, so the same *http.Client is
// shared across polls. window is a value field: its zero value is a valid empty
// window, so no constructor is needed and a nil window cannot be built.
type benthosMonitorDeps struct {
	client *http.Client
	window throughputWindow
}

// BenthosMetrics carries the counter values scraped from the benthos monitor's
// /metrics endpoint.
type BenthosMetrics struct {
	InputReceived int
	OutputSent    int
}

// ComponentThroughput carries the real-time rate for one direction of the benthos
// pipeline. MessagesPerSecond is the by-time rate the worker computes (never the
// FSMv1 MessagesPerTick, which adapter.go converts on the way out); LastCount is
// the newest counter value seen.
type ComponentThroughput struct {
	MessagesPerSecond float64
	LastCount         int
}

// BenthosMonitorStatus is the result of one scrape of the benthos monitor: the
// time it happened, the /metrics counters, /ping liveness, /ready readiness, the
// /version string, and the per-direction throughput computed over the 60s window.
type BenthosMonitorStatus struct {
	ScrapedAt      time.Time
	BenthosMetrics BenthosMetrics
	Input          ComponentThroughput
	Output         ComponentThroughput
	// IsActive is true when input traffic was observed in the window. It is
	// input-only (FSMv1: s.IsActive = s.Input.MessagesPerTick > 0) and tick-free,
	// with no hysteresis.
	IsActive  bool
	PingAlive bool
	Ready     bool
	Version   string
}

// Poll scrapes the configured benthos monitor's /ping, /ready, /version, and
// /metrics endpoints once. The client comes from d (a nil deps falls back to the
// default client). All requests carry ctx, so cancellation surfaces as a request
// error.
func Poll(ctx context.Context, d *benthosMonitorDeps, cfg config.BenthosMonitorConfig) (BenthosMonitorStatus, error) {
	client := http.DefaultClient
	if d != nil && d.client != nil {
		client = d.client
	}

	base := fmt.Sprintf("http://localhost:%d", cfg.MetricsPort)
	status := BenthosMonitorStatus{ScrapedAt: time.Now()}

	_, _, err := get(ctx, client, base+"/ping")
	if err == nil {
		status.PingAlive = true
	}

	readyBody, _, err := get(ctx, client, base+"/ready")
	if err == nil {
		// The /ready endpoint reports readiness by returning JSON whose error
		// field is empty when every input/output connection is up (fsmv1
		// ParseReadyData: readyResp.Error == ""). A benthos that answers but is
		// not ready (e.g. a broken pipeline that still answers /ping) reports
		// Ready=false while PingAlive=true.
		var r struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(readyBody, &r); err == nil {
			status.Ready = r.Error == ""
		}
	}

	versionBody, _, err := get(ctx, client, base+"/version")
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

	metricsBody, _, err := get(ctx, client, base+"/metrics")
	if err != nil {
		return status, fmt.Errorf("scrape /metrics: %w", err)
	}
	m, err := scrapeMetrics(metricsBody)
	if err != nil {
		return status, fmt.Errorf("parse /metrics: %w", err)
	}
	status.BenthosMetrics = m

	// Feed this poll's counter snapshot into the by-time window and read back the
	// real-time rates. The window lives in the deps and survives across polls
	// (D5); a nil deps falls back to a one-off window whose rate is always 0. The
	// window is keyed to the scrape port so an in-place port change (no worker
	// restart, D5a) wipes the old series instead of delta-ticking across it.
	if d != nil {
		d.window.Add(status.ScrapedAt, int(cfg.MetricsPort), m.InputReceived, m.OutputSent)
		status.Input = ComponentThroughput{
			MessagesPerSecond: d.window.inputRate(),
			LastCount:         d.window.inputCount(),
		}
		status.Output = ComponentThroughput{
			MessagesPerSecond: d.window.outputRate(),
			LastCount:         d.window.outputCount(),
		}
		status.IsActive = status.Input.MessagesPerSecond > 0
	}

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
	defer resp.Body.Close()
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

// scrapeMetrics parses the /metrics text into the two benthos counters of
// interest. benthos emits name{label="...",path="..."} value lines, so each
// name is matched after stripping its `{...}` label block; lines it does not
// recognize are ignored. A benthos with switch/broker/fallback outputs emits one
// series per leaf path with no top-level aggregate, so each counter is the SUM
// across every matching line (the same aggregation fsmv1's InputReceivedTotal
// / OutputSentTotal provide). Counter values may be rendered in scientific
// notation or as bare fractions by prometheus text exposition and are parsed
// accordingly (fsmv1's TailInt tolerates the same shapes). A counter line whose
// value is malformed returns an error rather than silently zeroing, so a
// long-scale or format-drifted scrape is reported instead of being read as no
// traffic.
func scrapeMetrics(body []byte) (BenthosMetrics, error) {
	var m BenthosMetrics
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	// The body budget is maxScrapeBody (4MiB); a single /metrics line (long
	// path/label set or HELP text) can exceed the scanner's 64KiB default token,
	// so size the buffer to match the budget instead of failing the scrape.
	sc.Buffer(make([]byte, 0, 64*1024), maxScrapeBody)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := counterLine(line)
		if !ok {
			continue
		}
		n, err := parseCounterValue(value)
		if err != nil {
			return m, fmt.Errorf("parse %s counter %q: %w", name, value, err)
		}
		switch name {
		case "input_received":
			m.InputReceived += n
		case "output_sent":
			m.OutputSent += n
		}
	}
	if err := sc.Err(); err != nil {
		return m, fmt.Errorf("read /metrics body: %w", err)
	}
	return m, nil
}

// counterLine splits one /metrics line into its bare metric name (with any
// `{...}` label block removed) and its value. Prometheus text exposition emits
// `name{label="...",path="..."} value`; matching the bare name after the label
// block keeps the parser aligned with the real payload. Lines without exactly a
// name and one value are ignored.
func counterLine(line string) (string, string, bool) {
	parts := strings.Fields(line)
	if len(parts) != 2 {
		return "", "", false
	}
	field := parts[0]
	if i := strings.IndexByte(field, '{'); i != -1 {
		field = field[:i]
	}
	return field, parts[1], true
}

// parseCounterValue parses a prometheus counter value into an int. Large whole
// counters are rendered in scientific notation and small ones as bare integers,
// but prometheus text exposition may also render a value as a bare fraction
// (e.g. "0.5"), which fsmv1's TailInt tolerates by truncating. Any value that
// carries an exponent, a decimal point, or an Inf/NaN marker is parsed as a
// float first (the same branch fsmv1's TailInt uses), then truncated to int;
// the conversion is guarded so an out-of-range or non-finite float errors rather
// than silently producing a garbage value.
func parseCounterValue(s string) (int, error) {
	if strings.ContainsAny(s, "eE.") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, err
		}
		if math.IsInf(f, 0) || math.IsNaN(f) || f < math.MinInt64 || f > math.MaxInt64 {
			return 0, fmt.Errorf("counter %q is not a finite int64 value", s)
		}
		return int(f), nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// cfgFor renders a BenthosMonitorConfig into the Upsert payload map through
// YAML, so the keys match BenthosMonitorConfig's yaml tags. This matters
// because the child-spec pipeline YAML-marshals the map into UserSpec.Config
// and the worker YAML-unmarshals it back into a BenthosMonitorConfig: the
// adapter's default CfgFor (a JSON round-trip) would emit Go field names —
// BenthosMonitorConfig carries no json tags — and lose Name,
// DesiredFSMState, and MetricsPort on the way back.
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
		// empty. It is benthos_monitor's own "active", not the adapter default
		// "running": benthos_monitor's FSM accepts only active/stopped as a
		// desired state (fsm/benthos_monitor/machine.go).
		States: adapter.StateVocabulary{
			Starting:       benthosmonitorfsm.OperationalStateStarting,
			Degraded:       benthosmonitorfsm.OperationalStateDegraded,
			Stopped:        benthosmonitorfsm.OperationalStateStopped,
			DesiredRunning: benthosmonitorfsm.OperationalStateActive,
		},
	})
}

// mapFresh maps a Fresh, healthy observation to its fsmv1 operational state:
// metrics are OK, so the monitor is active. Degraded, stale, and bootstrap
// verdicts are framework-owned and handled by the adapter, so this only
// classifies the healthy leaf.
func mapFresh(_ config.BenthosMonitorConfig, _ simple.Status[BenthosMonitorStatus]) string {
	return benthosmonitorfsm.OperationalStateActive
}
