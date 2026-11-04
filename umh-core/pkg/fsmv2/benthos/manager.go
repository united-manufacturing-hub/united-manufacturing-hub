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
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
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
//
// Failures are not copied; FSMv1 has no field for them (see Failures).
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

// StaleAfter is the staleness rule: three missed scrapes.
const StaleAfter = 3 * benthosmetrics.ScrapeInterval

// isFresh reports whether a sample taken at takenAt can still vouch for
// liveness at now. A zero takenAt means no scrape has succeeded.
func isFresh(takenAt, now time.Time) bool {
	return !takenAt.IsZero() && now.Sub(takenAt) <= StaleAfter
}

// lastGoodFailures holds, per Benthos S6 service name, the Failures of the
// last store read that answered for that name, so an unknown read can
// answer from them (see Failures). The mutex guards the map: concurrent
// Benthos services read their instance's Failures on their own tick.
var (
	lastGoodMu       sync.Mutex
	lastGoodFailures = map[string]benthosmetrics.Failures{}
)

// Failures returns the failures last stored for the Benthos instance named
// s6ServiceName ("benthos-<name>"), whatever their age, and false when it
// cannot answer. An unknown read (a read error or a read timeout) answers
// from the last good Failures held for that name, the way
// HoldLastStatusOnUnknown holds observed state; a missing or deleted
// worker, or one with no stored sample, answers false.
func Failures(ctx context.Context, s6ServiceName string) (*benthosmetrics.Failures, bool) {
	c := fsmv2client.GetClient()
	if c == nil {
		return nil, false
	}

	obs, err := fsmv2client.Get[simple.Status[benthosmetrics.Sample]](
		ctx, c,
		dynamicchildren.Ref{WorkerType: WorkerType, Name: s6ServiceName},
	)
	if err != nil {
		// No worker answers under this name — nothing was ever stored for
		// it, or it was deleted: fail closed, and drop what the hold has.
		if errors.Is(err, fsmv2client.ErrNotFound) || errors.Is(err, fsmv2client.ErrWorkerDeleted) {
			forgetLastGoodFailures(s6ServiceName)
			return nil, false
		}

		// Any other read failure — a read error or a read timeout — is an
		// unknown read: answer from the last good Failures.
		return holdLastGoodFailures(c, s6ServiceName)
	}

	f := obs.Status.Result.Failures
	if f == nil {
		// The worker stored no sample yet, or its sample carries no
		// Failures: there is nothing good to hold for it.
		forgetLastGoodFailures(s6ServiceName)
		return nil, false
	}

	rememberLastGoodFailures(c, s6ServiceName, f)

	return f, true
}

// rememberLastGoodFailures stores f as the last good Failures read for
// s6ServiceName. A name new to the hold first sweeps it: entries whose
// worker is no longer registered are dropped, so the hold cannot outgrow
// the fleet it serves.
func rememberLastGoodFailures(c *fsmv2client.FSMv2Client, s6ServiceName string, f *benthosmetrics.Failures) {
	lastGoodMu.Lock()
	defer lastGoodMu.Unlock()

	if _, known := lastGoodFailures[s6ServiceName]; !known {
		registered := make(map[string]struct{})
		for _, ref := range c.Refs(WorkerType) {
			registered[ref.Name] = struct{}{}
		}
		for name := range lastGoodFailures {
			if _, ok := registered[name]; !ok {
				delete(lastGoodFailures, name)
			}
		}
	}

	// The hold keeps its own copy, so it never shares a pointer with the
	// Failures a caller is reading.
	cp := *f
	cp.Entries = make([]benthosmetrics.FailureEntry, len(f.Entries))
	copy(cp.Entries, f.Entries)
	lastGoodFailures[s6ServiceName] = cp
}

// forgetLastGoodFailures drops s6ServiceName's entry from the hold.
func forgetLastGoodFailures(s6ServiceName string) {
	lastGoodMu.Lock()
	defer lastGoodMu.Unlock()

	delete(lastGoodFailures, s6ServiceName)
}

// holdLastGoodFailures answers an unknown read of s6ServiceName from the
// last good Failures the hold has for it, when its worker is still
// registered and the hold has an entry; otherwise the read stays without
// an answer.
func holdLastGoodFailures(c *fsmv2client.FSMv2Client, s6ServiceName string) (*benthosmetrics.Failures, bool) {
	registered := false
	for _, ref := range c.Refs(WorkerType) {
		if ref.Name == s6ServiceName {
			registered = true
			break
		}
	}
	if !registered {
		return nil, false
	}

	lastGoodMu.Lock()
	defer lastGoodMu.Unlock()

	f, held := lastGoodFailures[s6ServiceName]
	if !held {
		return nil, false
	}

	// The caller gets its own copy of the entries, so a reader that changes
	// its Failures cannot change what the hold later serves.
	entries := make([]benthosmetrics.FailureEntry, len(f.Entries))
	copy(entries, f.Entries)
	f.Entries = entries

	return &f, true
}

// ErrorsNotClearedCount returns how many of the registered Benthos workers
// hold an error that should have cleared: their stored Failures answer true
// for NotCleared(margin). It counts every registered worker of this type —
// the Benthos instance of a data flow component, a protocol converter, a
// stream processor, a topic browser or a stand-alone flow alike — and a
// protocol converter with both sides stuck counts twice.
func ErrorsNotClearedCount(ctx context.Context, margin time.Duration) int {
	c := fsmv2client.GetClient()
	if c == nil {
		return 0
	}

	count := 0
	for _, ref := range c.Refs(WorkerType) {
		if failures, ok := Failures(ctx, ref.Name); ok && failures.NotCleared(margin) {
			count++
		}
	}

	return count
}
