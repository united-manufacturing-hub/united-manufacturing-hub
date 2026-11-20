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

package fsmv2benthosmetrics

import (
	"context"
	"errors"
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
)

// NewManager returns the manager that runs one FSMv2
// worker per enabled BenthosMonitorConfig and reports their states as FSMv1
// benthos_monitor states. Pass the FSMv1 monitor's log prefix as managerName,
// so log labels stay the same when the flag flips.
func NewManager(managerName string) *adapter.WorkerManager[config.BenthosMonitorConfig, simple.Status[Scrape]] {
	return adapter.NewWorkerManager(adapter.WorkerManagerSpec[config.BenthosMonitorConfig, simple.Status[Scrape]]{
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

// cfgFor builds the worker's Config from cfg: the worker needs only the port.
func cfgFor(cfg config.BenthosMonitorConfig) (map[string]any, error) {
	return map[string]any{"metricsPort": cfg.MetricsPort}, nil
}

// mapFresh returns benthos_monitor's "active" state word for a fresh, healthy
// observation. The adapter's WorkerManager handles every other case (table in
// pkg/fsmv2/adapter/doc.go).
func mapFresh(_ config.BenthosMonitorConfig, _ simple.Status[Scrape]) string {
	return benthosmonitorfsm.OperationalStateActive
}

// mapObserved converts a stored Scrape into FSMv1's observed state, with
// every pointer non-nil even for a zero Scrape: GetHealthCheckAndMetrics
// dereferences LastScan.BenthosMetrics unchecked.
//
// Failures are not copied; FSMv1 has no field for them (see Failures).
// The worker's health does not read them until ENG-6596.
func mapObserved(cfg config.BenthosMonitorConfig, s simple.Status[Scrape]) publicfsm.ObservedState {
	probes, sample := s.Result.Probes, s.Result.Sample
	fresh := isFresh(probes.At, scrapeClock())

	tick := constants.DefaultTickerTime.Seconds()
	// An Unknown rate reads 0, as FSMv1 reports before its window fills.
	inputPerSecond, _ := sample.InputMessagesPerSecond.Get()
	outputPerSecond, _ := sample.OutputMessagesPerSecond.Get()

	scan := &benthosmonitorservice.BenthosMetricsScan{
		LastUpdatedAt: probes.At,
		BenthosMetrics: &benthosmonitorservice.BenthosMetrics{
			MetricsState: &benthosmonitorservice.BenthosMetricsState{
				// Processors stays nil: nothing outside benthos_monitor
				// reads it.
				Input: benthosmonitorservice.ComponentThroughput{
					LastCount:       sample.Metrics.InputReceivedTotal(),
					MessagesPerTick: inputPerSecond * tick,
				},
				Output: benthosmonitorservice.ComponentThroughput{
					LastCount:       sample.Metrics.OutputSentTotal(),
					MessagesPerTick: outputPerSecond * tick,
				},
				IsActive: sample.IsActive,
			},
			Metrics: toFSMv1Metrics(sample.Metrics),
		},
		HealthCheck: benthosmonitorservice.HealthCheck{
			Version: probes.Version,
			IsLive:  fresh && probes.PingAlive,
			IsReady: fresh && probes.Ready,
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

// toFSMv1Latency copies one latency summary across the metrics types. A
// quantile Benthos did not report reads 0, as FSMv1's parse leaves it.
func toFSMv1Latency(l benthosmetrics.Latency) benthosmonitorservice.Latency {
	return benthosmonitorservice.Latency{
		P50:   quantileOrZero(l.P50),
		P90:   quantileOrZero(l.P90),
		P99:   quantileOrZero(l.P99),
		Sum:   l.Sum,
		Count: l.Count,
	}
}

func quantileOrZero(p *float64) float64 {
	if p == nil {
		return 0
	}

	return *p
}

// staleAfter is the staleness rule: three missed scrapes.
const staleAfter = 3 * benthosmetrics.ScrapeInterval

// isFresh reports whether a scrape that began at takenAt can still vouch for
// liveness at now. A zero takenAt means no scrape has succeeded.
func isFresh(takenAt, now time.Time) bool {
	return !takenAt.IsZero() && now.Sub(takenAt) <= staleAfter
}

// lastGoodFailures, the hold, keeps per Benthos S6 service name the
// Failures of the last Failures read that answered for that name, so an unknown
// read can answer from them (see Failures). The mutex guards the map:
// concurrent Benthos services read their instance's Failures on their own
// tick.
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

	f, err := loadFailures(ctx, c, s6ServiceName)
	if err != nil {
		// No worker answers under this name — nothing was ever stored for
		// it, or it was deleted: fail closed, and drop what the hold has.
		if errors.Is(err, fsmv2client.ErrNotFound) || errors.Is(err, fsmv2client.ErrWorkerDeleted) {
			forgetLastGoodFailures(s6ServiceName)

			return nil, false
		}

		// Any other read failure — a read error or a read timeout — is an
		// unknown read: answer from the last good Failures.
		return heldFailures(c, s6ServiceName)
	}

	if f == nil {
		// The worker stored no sample yet, or its sample carries no
		// Failures: there is nothing good to hold for it.
		forgetLastGoodFailures(s6ServiceName)

		return nil, false
	}

	rememberLastGoodFailures(c, s6ServiceName, f)

	return f, true
}

// loadFailures reads the Failures stored for s6ServiceName's worker. It
// returns nil and no error when the worker stored no Failures yet.
func loadFailures(ctx context.Context, c *fsmv2client.FSMv2Client, s6ServiceName string) (*benthosmetrics.Failures, error) {
	obs, err := fsmv2client.Get[simple.Status[Scrape]](
		ctx, c,
		dynamicchildren.Ref{WorkerType: WorkerType, Name: s6ServiceName},
	)
	if err != nil {
		return nil, err
	}

	return obs.Status.Result.Sample.Failures, nil
}

// rememberLastGoodFailures stores a copy of f as s6ServiceName's last good
// Failures, unless the hold already has a newer one: two readers of the same
// name can race, and a slow one must not put an older sample back. A name
// new to the hold first sweeps out names whose worker is no longer
// registered, so the hold cannot outgrow the fleet it serves.
func rememberLastGoodFailures(c *fsmv2client.FSMv2Client, s6ServiceName string, f *benthosmetrics.Failures) {
	// Snapshot the registry BEFORE taking the mutex: Refs clones every
	// registered spec, and holding lastGoodMu across it would block every
	// other flow's lookup for the length of a full registry copy.
	known, candidates := heldBeforeSweep(s6ServiceName)

	var registered map[string]struct{}
	if !known {
		registered = make(map[string]struct{})
		for _, ref := range c.Refs(WorkerType) {
			registered[ref.Name] = struct{}{}
		}
	}

	lastGoodMu.Lock()
	defer lastGoodMu.Unlock()

	if registered != nil {
		sweepUnregistered(lastGoodFailures, candidates, registered)
	}

	// Ties are the same scrape. A wall clock stepped back stalls the hold
	// until it catches up, which is safe: the health path rejects a sample
	// from the future as "no fresh Benthos metrics".
	if prev, held := lastGoodFailures[s6ServiceName]; held &&
		f.ObservedAt.Before(prev.ObservedAt) {
		return
	}

	// The hold keeps its own copy, so it never shares a pointer with the
	// Failures a caller is reading.
	cp := *f
	cp.Entries = make([]benthosmetrics.FailureEntry, len(f.Entries))
	copy(cp.Entries, f.Entries)
	lastGoodFailures[s6ServiceName] = cp
}

// heldBeforeSweep reports whether the hold already has an entry for
// s6ServiceName. When it does not, it also returns the ObservedAt of every
// entry the hold has now, the candidates a sweep may drop.
func heldBeforeSweep(s6ServiceName string) (bool, map[string]time.Time) {
	lastGoodMu.Lock()
	defer lastGoodMu.Unlock()

	if _, known := lastGoodFailures[s6ServiceName]; known {
		return true, nil
	}

	candidates := make(map[string]time.Time, len(lastGoodFailures))
	for name, f := range lastGoodFailures {
		candidates[name] = f.ObservedAt
	}

	return false, candidates
}

// sweepUnregistered drops from hold the candidates whose worker is not in
// registered. The registry is read without the hold's mutex, so another
// reader may add or update an entry between that read and this sweep; only
// entries still as they were in candidates are dropped, so the sweep never
// removes a sample it has not seen. The caller holds lastGoodMu.
func sweepUnregistered(hold map[string]benthosmetrics.Failures, candidates map[string]time.Time, registered map[string]struct{}) {
	for name, seenAt := range candidates {
		if _, ok := registered[name]; ok {
			continue
		}

		if f, ok := hold[name]; ok && f.ObservedAt.Equal(seenAt) {
			delete(hold, name)
		}
	}
}

// forgetLastGoodFailures drops s6ServiceName's entry from the hold.
func forgetLastGoodFailures(s6ServiceName string) {
	lastGoodMu.Lock()
	defer lastGoodMu.Unlock()

	delete(lastGoodFailures, s6ServiceName)
}

// heldFailures answers an unknown read of s6ServiceName from the hold, when
// its worker is still registered. A worker recreated under the same name may
// get its predecessor's Failures until its first good read; their ObservedAt
// makes the health path reject them once they are stale.
func heldFailures(c *fsmv2client.FSMv2Client, s6ServiceName string) (*benthosmetrics.Failures, bool) {
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
// for NotCleared(margin). It counts every registered worker of this type:
// one per bridge side, stand-alone flow, stream processor and topic browser,
// so a bridge with both sides stuck counts twice. It reads like Failures,
// an unknown read answering from the hold, but never changes the hold.
func ErrorsNotClearedCount(ctx context.Context, margin time.Duration) int {
	c := fsmv2client.GetClient()
	if c == nil {
		return 0
	}

	count := 0

	for _, ref := range c.Refs(WorkerType) {
		f, err := loadFailures(ctx, c, ref.Name)
		if err != nil && !errors.Is(err, fsmv2client.ErrNotFound) && !errors.Is(err, fsmv2client.ErrWorkerDeleted) {
			f, _ = heldFailures(c, ref.Name)
		}

		if f != nil && f.NotCleared(margin) {
			count++
		}
	}

	return count
}
