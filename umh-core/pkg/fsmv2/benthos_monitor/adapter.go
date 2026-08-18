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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// tickSeconds is the FSMv1 control-loop period in seconds (DefaultTickerTime,
// pkg/constants/loop.go). It converts the worker's tick-free
// MessagesPerSecond — a rate in real seconds, not per tick — into FSMv1's
// MessagesPerTick at the two call sites below.
//
// It is the only per-tick rate this package produces, and it is temporary: it
// goes when benthos itself moves to fsmv2 and this adapter file is deleted, so
// do not extend the vocabulary further.
func tickSeconds() float64 {
	return constants.DefaultTickerTime.Seconds()
}

// mapObserved builds a benthosmonitorfsm.BenthosMonitorObservedState from the
// stored status and config, the shape every FSMv1 consumer reads. It must never
// produce a nil BenthosMetrics: GetHealthCheckAndMetrics
// (pkg/service/benthos/benthos.go) dereferences LastScan.BenthosMetrics
// without a nil check, so a nil there is a control-loop panic. (It does guard
// LastScan itself, in the same function.)
//
// It must tolerate two inputs. A zero status arrives whenever
// fsmv2client.GetFresh reports Unknown, Unregistered or NeverObserved, and maps
// to an all-non-nil empty scan rather than to nil pointers. A Stale read
// arrives carrying the last real status instead, so stale content reaches every
// consumer with only the Degraded verdict to mark it.
//
// MessagesPerTick is scaled out of MessagesPerSecond here; IsActive is copied
// through unscaled, because the worker already computes it tick-free.
func mapObserved(cfg config.BenthosMonitorConfig, s simple.Status[BenthosMonitorStatus]) publicfsm.ObservedState {
	status := s.Result

	scan := &benthosmonitorservice.BenthosMetricsScan{
		LastUpdatedAt: status.ScrapedAt,
		BenthosMetrics: &benthosmonitorservice.BenthosMetrics{
			MetricsState: &benthosmonitorservice.BenthosMetricsState{
				Input: benthosmonitorservice.ComponentThroughput{
					LastCount:       int64(status.Input.LastCount),
					MessagesPerTick: status.Input.MessagesPerSecond * tickSeconds(),
				},
				Output: benthosmonitorservice.ComponentThroughput{
					LastCount:       int64(status.Output.LastCount),
					MessagesPerTick: status.Output.MessagesPerSecond * tickSeconds(),
				},
				IsActive: status.IsActive,
			},
			// The per-path counters, carried through untouched. Bridge health is
			// decided from these: protocolconverter and streamprocessor both read
			// connection up/lost off this struct and judge a direction connected
			// when up exceeds lost. A zero status maps to a zero Metrics, whose
			// maps are nil — safe, because every consumer reads it through the
			// summing accessors, which range over a nil map without dereferencing.
			Metrics: status.BenthosMetrics,
		},
		HealthCheck: benthosmonitorservice.HealthCheck{
			Version: status.Version,
			IsLive:  status.PingAlive,
			IsReady: status.Ready,
		},
	}

	return benthosmonitorfsm.BenthosMonitorObservedState{
		ObservedMonitorConfig: cfg,
		ServiceInfo: &benthosmonitorservice.ServiceInfo{
			BenthosStatus: benthosmonitorservice.BenthosMonitorStatus{
				LastScan: scan,
				// IsRunning gates every consumer: GetHealthCheckAndMetrics
				// (pkg/service/benthos/benthos.go) discards the HealthCheck it
				// just read and returns ErrBenthosMonitorNotRunning when this is false.
				// FSMv1 sourced it from the monitor's own S6 FSM state
				// (BenthosMonitorService.Status,
				// pkg/service/benthos_monitor/benthos_monitor.go); under the
				// USE_FSMV2_BENTHOS_MONITOR flag (envUseFsmv2BenthosMonitor,
				// pkg/service/benthos/benthos.go) no such process exists, so the
				// nearest true statement is that the observation can be trusted.
				//
				// This deliberately collapses FSMv1's running-but-unhealthy case: a
				// stale scan reports not-running, because with no process there is
				// nothing left for "running" to describe.
				IsRunning: observationIsTrustworthy(s),
			},
		},
	}
}

// observationIsTrustworthy reports whether the stored status carries a scrape that
// happened and whose data the framework has not marked degraded. Both conjuncts are
// load-bearing and neither implies the other. simple.Status.Degraded
// (pkg/fsmv2/simple/status.go) is set both when the polled target is unhealthy
// and when the poll failed, and its zero value is false — i.e. "healthy" — so a zero
// Status would otherwise report a never-polled worker as running. And a failed
// scrape carries a fresh ScrapedAt by construction, because Poll (manager.go)
// stamps it before its first request and returns that partial status on every
// path below the stamp, so the timestamp alone cannot tell a dead monitor from a
// live one. (Poll's deps-not-bound guard returns before the stamp, so a wiring
// fault is the one failure the timestamp does catch.)
//
// TestMapObservedSetsIsRunning asserts all four cases. Do not simplify this
// predicate to a constant: an unconditional false here held every bridge in
// starting, which is the failure that test's live case exists to catch.
func observationIsTrustworthy(s simple.Status[BenthosMonitorStatus]) bool {
	degraded, _ := s.HealthVerdict()

	return !s.Result.ScrapedAt.IsZero() && !degraded
}

// health decides freshness and nothing else: a scan older than
// BenthosMaxMetricsAndConfigAge, or one that never happened, is degraded. Poll's
// error path drives the worker degraded before this is called, so a nil-error
// status here means the scrape happened and freshness is all that is left to judge.
//
// This is deliberately narrower than FSMv1's isMonitorHealthy
// (pkg/fsm/benthos_monitor/actions.go), which also required a non-nil
// BenthosMetrics — a conjunct that cannot fail here, because mapObserved never
// produces one. FSMv1 also measured age from the loop start time, not time.Now().
func health(_ config.BenthosMonitorConfig, status BenthosMonitorStatus) simple.Health {
	if status.ScrapedAt.IsZero() || time.Since(status.ScrapedAt) > constants.BenthosMaxMetricsAndConfigAge {
		return simple.Degraded("benthos monitor scan is stale")
	}

	return simple.Healthy("benthos monitor scan is fresh")
}
