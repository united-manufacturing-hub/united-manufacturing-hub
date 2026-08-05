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
// framework. The worker registers itself on import. The metrics-endpoint fetch
// is a later-stage addition; for now Poll returns an empty, nil-error status
// that keeps the worker healthy.
package fsmv2benthosmonitor

import (
	"context"
	"fmt"
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
)

// BenthosMonitorStatus is the result of one metrics observation of the benthos
// monitor.
type BenthosMonitorStatus struct{}

// Poll observes the benthos monitor once. The metrics-endpoint fetch is a
// later-stage addition; Poll currently returns an empty, nil-error status that
// keeps an unwired worker healthy.
func Poll(ctx context.Context, _ struct{}, _ config.BenthosMonitorConfig) (BenthosMonitorStatus, error) {
	return BenthosMonitorStatus{}, nil
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
	simple.Register(simple.MonitorSpec[config.BenthosMonitorConfig, BenthosMonitorStatus, struct{}]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		Poll:       Poll,
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

// mapObserved holds the place of the later mapping from a stored status to a
// benthosmonitorfsm.BenthosMonitorObservedState; it returns an empty state
// until the adapter path that consumes observed states is added.
func mapObserved(_ config.BenthosMonitorConfig, _ simple.Status[BenthosMonitorStatus]) publicfsm.ObservedState {
	return benthosmonitorfsm.BenthosMonitorObservedState{}
}
