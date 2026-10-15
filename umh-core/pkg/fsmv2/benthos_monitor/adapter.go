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
// pkg/constants/loop.go). mapObserved multiplies the worker's MessagesPerSecond
// by it to get FSMv1's MessagesPerTick. Delete it with this file when benthos
// moves to fsmv2.
func tickSeconds() float64 {
	return constants.DefaultTickerTime.Seconds()
}

// mapObserved builds the FSMv1 observed state from a stored status. Every
// pointer it returns is non-nil, including for a zero status:
// GetHealthCheckAndMetrics (pkg/service/benthos/benthos.go) dereferences
// LastScan.BenthosMetrics without a check.
//
// adapter.AdaptedInstance.GetLastObservedState passes a zero status when the
// read is Unknown, Unregistered or NeverObserved (see fsmv2client.Freshness). A
// Stale read passes the last stored status, and only the Degraded verdict marks
// it as old.
func mapObserved(cfg config.BenthosMonitorConfig, s simple.Status[BenthosMonitorStatus]) publicfsm.ObservedState {
	status := s.Result

	scan := &benthosmonitorservice.BenthosMetricsScan{
		LastUpdatedAt: status.PollStartedAt,
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
			// Bridge health reads the connection counters from here. A nil map
			// is safe: consumers only read it through the *Total() methods.
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
				// GetHealthCheckAndMetrics discards the scan when IsRunning is
				// false. The fsmv2 worker has no S6 process, so IsRunning means
				// "this observation can be trusted": a stale or failed scan
				// reports false.
				IsRunning: observationIsTrustworthy(s),
			},
		},
	}
}

// observationIsTrustworthy reports whether a poll ran and the framework did not
// mark it degraded. Both checks are needed: a zero Status has Degraded=false,
// and a failed poll still has a PollStartedAt. TestMapObservedSetsIsRunning
// covers the four cases.
func observationIsTrustworthy(s simple.Status[BenthosMonitorStatus]) bool {
	degraded, _ := s.HealthVerdict()

	return !s.Result.PollStartedAt.IsZero() && !degraded
}

// health returns Degraded when the poll started longer ago than
// BenthosMaxMetricsAndConfigAge. The framework calls it only after a successful
// Poll; a failed Poll is marked degraded without it.
func health(_ config.BenthosMonitorConfig, status BenthosMonitorStatus) simple.Health {
	if status.PollStartedAt.IsZero() || time.Since(status.PollStartedAt) > constants.BenthosMaxMetricsAndConfigAge {
		return simple.Degraded("benthos monitor scan is stale")
	}

	return simple.Healthy("benthos monitor scan is fresh")
}
