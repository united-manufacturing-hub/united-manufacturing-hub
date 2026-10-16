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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
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

	// monitorClientTimeout bounds each request. Poll makes four requests in
	// sequence, and 4 x 400ms stays under supervisor.DefaultObservationTimeout
	// (~2.2s).
	monitorClientTimeout = 400 * time.Millisecond

	// maxScrapeBody caps the response bodies Poll reads, so a misbehaving
	// monitor cannot be buffered without bound on each poll.
	maxScrapeBody = 4 << 20 // 4 MiB
)

// benthosMonitorDeps holds one worker's scrape state. Poll receives a pointer to
// the deps, so the same *http.Client is shared across polls.
type benthosMonitorDeps struct {
	// client scrapes the benthos monitor's endpoints.
	client *http.Client

	// engine holds the windows that turn counters into rates across polls.
	// observe builds it on first use; observe's doc says when it is rebuilt.
	engine *diagnosis.Engine[sample]

	// prev is the previous observed scrape, nil before the first.
	prev *observation

	// now stamps each scrape; see scrapeTime.
	now func() time.Time
}

// scrapeClock is the clock NewDeps gives each worker. Tests replace it to
// drive the failure window without waiting one constants.BenthosLogWindow.
var scrapeClock = time.Now

// scrapeTime falls back to time.Now for deps built without a clock, as tests
// that construct benthosMonitorDeps directly do.
func (d *benthosMonitorDeps) scrapeTime() time.Time {
	if d.now == nil {
		return time.Now()
	}

	return d.now()
}

// ComponentThroughput carries the rate for one direction of the benthos pipeline.
type ComponentThroughput struct {
	// MessagesPerSecond is the by-time rate the worker computes; see tickSeconds
	// for the conversion to FSMv1's MessagesPerTick.
	MessagesPerSecond float64

	// LastCount is the newest counter value seen.
	LastCount int
}

// BenthosMonitorStatus is what one Poll observed. After a failed Poll it holds
// only the fields filled before the error; the worker's Degraded flag says which
// case it is.
type BenthosMonitorStatus struct {
	// PollStartedAt is when Poll began. Poll sets it before the first request, so
	// a failed poll also carries a fresh value.
	PollStartedAt time.Time

	// BenthosMetrics is the parsed /metrics, kept whole: bridge health reads its
	// connection counters (see
	// TestMapObservedDeliversParsedConnectionCountersToBridgeHealth).
	BenthosMetrics benthosmonitorservice.Metrics

	// Input is the input-direction throughput computed over windowSpan.
	Input ComponentThroughput

	// Output is the output-direction throughput computed over windowSpan.
	Output ComponentThroughput

	// IsActive is Input.MessagesPerSecond > 0. Output traffic does not count.
	IsActive bool

	// Failures is the scrape's failure verdict.
	Failures *benthosmonitorservice.FailureVerdict

	// PingAlive is whether /ping answered.
	PingAlive bool

	// Ready is whether /ready reported every connection up. Poll's /ready branch
	// states how that is read off the response.
	Ready bool

	// Version is the version string /version reported.
	Version string
}

// Poll scrapes the configured benthos monitor's /ping, /ready, /version, and
// /metrics endpoints once, using the HTTP client d carries. All requests carry
// ctx, so cancellation surfaces as a request error.
//
// Poll returns an error without sending a request when d or d.client is nil.
// That only happens through a programming error, such as registering the worker
// without NewDeps.
func Poll(ctx context.Context, d *benthosMonitorDeps, cfg config.BenthosMonitorConfig) (BenthosMonitorStatus, error) {
	if d == nil || d.client == nil {
		return BenthosMonitorStatus{}, fmt.Errorf("benthos monitor has no HTTP client (deps nil: %t): the worker was registered without NewDeps", d == nil)
	}

	base := fmt.Sprintf("http://localhost:%d", cfg.MetricsPort)
	status := BenthosMonitorStatus{PollStartedAt: d.scrapeTime()}

	_, err := get(ctx, d.client, base+"/ping")
	if err == nil {
		status.PingAlive = true
	}

	readyBody, err := get(ctx, d.client, base+"/ready")
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

	versionBody, err := get(ctx, d.client, base+"/version")
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

	metricsBody, err := get(ctx, d.client, base+"/metrics")
	if err != nil {
		return status, fmt.Errorf("scrape /metrics: %w", err)
	}
	m, err := benthosmonitorservice.ParseMetricsFromBytes(metricsBody)
	if err != nil {
		return status, fmt.Errorf("parse /metrics: %w", err)
	}
	status.BenthosMetrics = m

	// The port is passed because a config update can change MetricsPort without
	// restarting the worker.
	t, verdict := observe(d, int(cfg.MetricsPort), m, status.PollStartedAt)
	status.Input = t.Input
	status.Output = t.Output
	status.IsActive = t.IsActive
	status.Failures = &verdict

	return status, nil
}

// get performs a GET request with ctx and returns the body. A non-2xx response
// is returned as an error: a benthos that answers a 500 error page is not
// healthy, and parsing its HTML as /metrics or /version yields garbage.
func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	// nilaway cannot prove resp is non-nil when Do returns a nil error.
	if resp == nil {
		return nil, fmt.Errorf("GET %s: nil response", url)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: unexpected status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxScrapeBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxScrapeBody {
		return nil, fmt.Errorf("GET %s: response body exceeds %d bytes", url, maxScrapeBody)
	}
	return body, nil
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
			return &benthosMonitorDeps{client: &http.Client{Timeout: monitorClientTimeout}, now: scrapeClock}
		},
		Poll:   Poll,
		Health: health,
	})
}

// NewFsmv2BenthosMonitorManager builds the fsmv1-compatible manager that drives
// the fsmv2 benthos monitor workers. It extracts BenthosMonitorConfig entries
// from the snapshot, adds or updates a worker for each enabled config, and maps
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
		States: adapter.StateVocabulary{
			Starting: benthosmonitorfsm.OperationalStateStarting,
			Degraded: benthosmonitorfsm.OperationalStateDegraded,
			Stopped:  benthosmonitorfsm.OperationalStateStopped,
			// DesiredRunning is used when a config leaves desiredState empty.
			// SetDesiredFSMState in pkg/fsm/benthos_monitor/machine.go accepts
			// only active or stopped.
			DesiredRunning: benthosmonitorfsm.OperationalStateActive,
		},
	})
}

// mapFresh returns "active" for a fresh, healthy observation.
// adapter.WorkerManager handles every other case (table in
// pkg/fsmv2/adapter/doc.go).
func mapFresh(_ config.BenthosMonitorConfig, _ simple.Status[BenthosMonitorStatus]) string {
	return benthosmonitorfsm.OperationalStateActive
}
