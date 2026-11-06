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

package fsmv2benthos

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"

	"gopkg.in/yaml.v3"
)

// NewFsmv2BenthosMetricsManager returns the manager that runs one FSMv2
// worker per enabled BenthosMonitorConfig and reports their states as FSMv1
// benthos_monitor states. Pass the FSMv1 monitor's log prefix as managerName,
// so log labels stay the same when the flag flips.
func NewFsmv2BenthosMetricsManager(managerName string) *adapter.WorkerManager[config.BenthosMonitorConfig, simple.Status[benthosmetrics.Sample]] {
	return adapter.NewWorkerManager(adapter.WorkerManagerSpec[config.BenthosMonitorConfig, simple.Status[benthosmetrics.Sample]]{
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
			// benthos_monitor's SetDesiredFSMState accepts only active or
			// stopped, so the fallback word is "active", not "running".
			DesiredRunning: benthosmonitorfsm.OperationalStateActive,
		},
		HoldLastStatusOnUnknown: true,
	})
}

// cfgFor converts cfg to the child-spec map via YAML: BenthosMonitorConfig
// has only yaml tags, so the adapter's default JSON round-trip would drop
// Name, DesiredFSMState and MetricsPort.
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

// mapFresh returns benthos_monitor's "active" state word for a fresh, healthy
// observation. The adapter's WorkerManager handles every other case (table in
// pkg/fsmv2/adapter/doc.go).
func mapFresh(_ config.BenthosMonitorConfig, _ simple.Status[benthosmetrics.Sample]) string {
	return benthosmonitorfsm.OperationalStateActive
}

// mapObserved converts a stored sample into FSMv1's observed state, with
// every pointer non-nil even for a zero sample: GetHealthCheckAndMetrics
// dereferences LastScan.BenthosMetrics unchecked.
func mapObserved(cfg config.BenthosMonitorConfig, s simple.Status[benthosmetrics.Sample]) publicfsm.ObservedState {
	sample := s.Result
	fresh := isFresh(sample.At, scrapeClock())

	tick := constants.DefaultTickerTime.Seconds()

	scan := &benthosmonitorservice.BenthosMetricsScan{
		LastUpdatedAt: sample.At,
		BenthosMetrics: &benthosmonitorservice.BenthosMetrics{
			MetricsState: &benthosmonitorservice.BenthosMetricsState{
				// Processors stays nil: nothing outside benthos_monitor
				// reads it.
				Input: benthosmonitorservice.ComponentThroughput{
					LastCount:       sample.Metrics.InputReceivedTotal(),
					MessagesPerTick: sample.Input.MessagesPerSecond * tick,
				},
				Output: benthosmonitorservice.ComponentThroughput{
					LastCount:       sample.Metrics.OutputSentTotal(),
					MessagesPerTick: sample.Output.MessagesPerSecond * tick,
				},
				IsActive: sample.IsActive,
			},
			Metrics: toFSMv1Metrics(sample.Metrics),
		},
		HealthCheck: benthosmonitorservice.HealthCheck{
			Version: sample.Version,
			IsLive:  fresh && sample.PingAlive,
			IsReady: fresh && sample.Ready,
		},
	}

	return benthosmonitorfsm.BenthosMonitorObservedState{
		ObservedMonitorConfig: cfg,
		ServiceInfo: &benthosmonitorservice.ServiceInfo{
			BenthosStatus: benthosmonitorservice.BenthosMonitorStatus{
				LastScan: scan,
				// Always true: this worker has no process of its own, and the
				// Benthos FSM treats a false IsRunning in its running states as
				// an error and stops updating its observed state. A failed or
				// stale scrape shows as IsLive and IsReady false instead.
				IsRunning: true,
			},
		},
	}
}

// toFSMv1Metrics copies the library's parsed metrics into FSMv1's Metrics
// type. FSMv1 keeps its own metrics types until it is deleted, so the two
// stay in sync only through this copy.
func toFSMv1Metrics(m benthosmetrics.Metrics) benthosmonitorservice.Metrics {
	out := benthosmonitorservice.Metrics{
		Process: benthosmonitorservice.ProcessMetrics{
			Processors: make(map[string]benthosmonitorservice.ProcessorMetrics, len(m.Process.Processors)),
		},
		Inputs:  make(map[string]benthosmonitorservice.InputInstance, len(m.Inputs)),
		Outputs: make(map[string]benthosmonitorservice.OutputInstance, len(m.Outputs)),
	}

	for path, p := range m.Process.Processors {
		out.Process.Processors[path] = benthosmonitorservice.ProcessorMetrics{
			Label:         p.Label,
			Received:      p.Received,
			BatchReceived: p.BatchReceived,
			Sent:          p.Sent,
			BatchSent:     p.BatchSent,
			Error:         p.Error,
			LatencyNS:     toFSMv1Latency(p.LatencyNS),
		}
	}

	for path, in := range m.Inputs {
		out.Inputs[path] = benthosmonitorservice.InputInstance{
			Path:             in.Path,
			Label:            in.Label,
			ConnectionFailed: in.ConnectionFailed,
			ConnectionLost:   in.ConnectionLost,
			ConnectionUp:     in.ConnectionUp,
			LatencyNS:        toFSMv1Latency(in.LatencyNS),
			Received:         in.Received,
		}
	}

	for path, o := range m.Outputs {
		out.Outputs[path] = benthosmonitorservice.OutputInstance{
			Path:             o.Path,
			Label:            o.Label,
			BatchSent:        o.BatchSent,
			ConnectionFailed: o.ConnectionFailed,
			ConnectionLost:   o.ConnectionLost,
			ConnectionUp:     o.ConnectionUp,
			Error:            o.Error,
			LatencyNS:        toFSMv1Latency(o.LatencyNS),
			Sent:             o.Sent,
		}
	}

	return out
}

// toFSMv1Latency copies one latency summary across the metrics types.
func toFSMv1Latency(l benthosmetrics.Latency) benthosmonitorservice.Latency {
	return benthosmonitorservice.Latency{
		P50:   l.P50,
		P90:   l.P90,
		P99:   l.P99,
		Sum:   l.Sum,
		Count: l.Count,
	}
}

// staleAfter is the staleness rule: three missed scrapes.
const staleAfter = 3 * benthosmetrics.ScrapeInterval

// isFresh reports whether a sample taken at takenAt can still vouch for
// liveness at now. A zero takenAt means no scrape has succeeded.
func isFresh(takenAt, now time.Time) bool {
	return !takenAt.IsZero() && now.Sub(takenAt) <= staleAfter
}
