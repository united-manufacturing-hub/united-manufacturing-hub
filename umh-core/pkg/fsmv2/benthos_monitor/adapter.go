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
// constants/loop.go:24). It converts the worker's tick-free MessagesPerSecond
// into FSMv1's MessagesPerTick at the two call sites below.
func tickSeconds() float64 {
	return constants.DefaultTickerTime.Seconds()
}

// mapObserved builds a benthosmonitorfsm.BenthosMonitorObservedState from the
// stored status and config, the shape every FSMv1 consumer reads. It must never
// produce a nil BenthosMetrics: pkg/service/benthos/benthos.go:699 dereferences
// LastScan.BenthosMetrics without a nil check, so a nil there is a control-loop
// panic. (LastScan itself is guarded, at benthos.go:697.) A zero status, which
// the adapter passes on a non-Fresh read, therefore maps to an all-non-nil empty
// scan rather than to nil pointers.
//
// MessagesPerTick is scaled out of MessagesPerSecond here; IsActive is copied
// through unscaled, because the worker already computes it tick-free.
func mapObserved(cfg config.BenthosMonitorConfig, s simple.Status[BenthosMonitorStatus]) publicfsm.ObservedState {
	status := s.Result

	// Read the verdict through the accessor the framework publishes for the fsmv1
	// side rather than off the field, so this reads as the sanctioned translation
	// it is. See the IsRunning comment below for why the verdict is the right
	// source.
	degraded, _ := s.HealthVerdict()

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
				// IsRunning means "the monitor itself is running", and it is not
				// cosmetic: GetHealthCheckAndMetrics (benthos.go) copies the
				// HealthCheck out of LastScan and then returns a ZERO BenthosStatus
				// plus ErrBenthosMonitorNotRunning when this is false, discarding it.
				//
				// FSMv1 read it off the S6 FSM state of the monitor service
				// (service/benthos_monitor:1486) — a fact about a process. Under this
				// flag there is no S6 monitor service: the worker IS the monitor, so
				// that fact has no referent and the nearest true statement has to be
				// chosen. The framework already computes it. simple.Status.Degraded is
				// documented as "the polled target is unhealthy OR the poll failed",
				// and HealthVerdict is published precisely so the fsmv1 side can read
				// that verdict; Poll's error path sets it before health() is consulted,
				// which is why health() only has to decide freshness.
				//
				// So: running == the observation is trustworthy. Deriving it from
				// ScrapedAt instead cannot work — Poll stamps ScrapedAt before its
				// first request (manager.go), returns that partial status on every
				// error path, and simple.CollectObservedState persists a failed poll's
				// status deliberately, so the timestamp is always fresh and IsRunning
				// was permanently true. The gate above could never fire and a dead
				// monitor's stale HealthCheck was copied out as if live.
				//
				// Both conjuncts are load-bearing and neither implies the other:
				//
				//   ScrapedAt != zero  — an observation exists at all. Degraded's zero
				//     value is false, i.e. "healthy", so a zero Status (nothing stored
				//     yet, or an empty read) would otherwise report a never-polled
				//     worker as running. TestMapObservedSetsIsRunning pins this.
				//   !degraded          — the observation is trustworthy. A failed poll
				//     carries a fresh ScrapedAt by construction, so the timestamp alone
				//     cannot tell a dead monitor from a live one.
				//
				// A successful poll is not degraded (health() sees a fresh scan), so
				// this does NOT resurrect the regression where an unconditional false
				// held every bridge in starting forever with live=false, ready=false.
				//
				// One deliberate collapse: a STALE scan now also reports not-running,
				// where FSMv1 could say running-but-unhealthy. With no process to be
				// running, that distinction has nowhere to live, and "no trustworthy
				// data" is the honest thing to tell fsmv1's consumers.
				IsRunning: !status.ScrapedAt.IsZero() && !degraded,
			},
		},
	}
}

// health decides freshness and nothing else: a scan older than
// BenthosMaxMetricsAndConfigAge, or one that never happened, is degraded. Poll's
// error path drives the worker degraded before this is called, so a nil-error
// status here means the scrape happened and freshness is all that is left to judge.
//
// This is deliberately narrower than FSMv1's isMonitorHealthy
// (pkg/fsm/benthos_monitor/actions.go:199), which also required a non-nil
// BenthosMetrics — a conjunct that cannot fail here, because mapObserved never
// produces one. FSMv1 also measured age from the loop start time, not time.Now().
func health(_ config.BenthosMonitorConfig, status BenthosMonitorStatus) simple.Health {
	if status.ScrapedAt.IsZero() || time.Since(status.ScrapedAt) > constants.BenthosMaxMetricsAndConfigAge {
		return simple.Degraded("benthos monitor scan is stale")
	}

	return simple.Healthy("benthos monitor scan is fresh")
}
