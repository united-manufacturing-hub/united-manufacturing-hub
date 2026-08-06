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

// tickSeconds is the FSMv1 control-loop period in seconds. It converts the
// worker's tick-free MessagesPerSecond into FSMv1's MessagesPerTick on the way
// out of mapObserved — the single conversion the design pins; constants/benthos.go:49.
func tickSeconds() float64 {
	return constants.DefaultTickerTime.Seconds()
}

// mapObserved builds a benthosmonitorfsm.BenthosMonitorObservedState from the
// stored status and config, the shape every FSMv1 consumer reads. It must never
// produce nil inner structs: benthos.go:657 + 669 dereference
// ServiceInfo.BenthosStatus.LastScan.BenthosMetrics unguarded, so a nil here is
// a control-loop panic. A zero status (the adapter passes one on a non-Fresh
// read, instance.go GetLastObservedState) therefore maps to an all-non-nil
// empty scan rather than to nil pointers.
//
// The only tick vocabulary lives here, on the way out: the worker is tick-free
// (MessagesPerSecond in real seconds), and adapter.go converts that to FSMv1's
// MessagesPerTick on this single line (MessagesPerSecond x DefaultTickerTime).
// It scales only the numeric MessagesPerTick field, never IsActive, which the
// worker already computes tick-free. When benthos itself moves to fsmv2, this
// file is deleted and the tick vocabulary dies with it.
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
			Metrics: benthosmonitorservice.Metrics{},
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
				// when this is false, discarding it. FSMv1 set it from the S6 FSM
				// state of the monitor service (service/benthos_monitor:1486); under
				// this flag there is no S6 monitor service, the worker IS the
				// monitor, so a scan that produced a timestamp is the evidence it
				// ran. Leaving it false made every consumer read live=false,
				// ready=false and held every bridge in starting forever.
				IsRunning: !status.ScrapedAt.IsZero(),
			},
		},
	}
}

// health reproduces FSMv1's isMonitorHealthy (pkg/fsm/benthos_monitor/actions.go): the
// monitor is healthy iff its last scan is fresh (within BenthosMaxMetricsAndConfigAge)
// and non-empty. A scrape failure drives the worker degraded through Poll's error
// path before this is ever called, so a nil-error status here means the scan
// happened; the residual decision is freshness. The reason stays coarse.
func health(_ config.BenthosMonitorConfig, status BenthosMonitorStatus) simple.Health {
	if status.ScrapedAt.IsZero() || time.Since(status.ScrapedAt) > constants.BenthosMaxMetricsAndConfigAge {
		return simple.Degraded("benthos monitor scan is stale")
	}

	return simple.Healthy("benthos monitor scan is fresh")
}
