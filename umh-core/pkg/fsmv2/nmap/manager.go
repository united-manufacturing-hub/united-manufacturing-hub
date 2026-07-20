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

package fsmv2nmap

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	nmapservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// NewFsmv2NmapManager builds the fsmv1-compatible manager that drives the
// fsmv2 nmap workers. It extracts NmapConfig entries from the snapshot, upserts
// enabled workers into the fsmv2 runtime, and maps their stored status back to
// the fsmv1 nmap operational states and NmapObservedState.
func NewFsmv2NmapManager(managerName string) *adapter.WorkerManager[config.NmapConfig, simple.Status[NmapStatus]] {
	return adapter.NewWorkerManager(adapter.WorkerManagerSpec[config.NmapConfig, simple.Status[NmapStatus]]{
		WorkerType: WorkerType,
		Log:        deps.NewFSMLogger(logger.For(managerName)),
		ExtractConfigs: func(s publicfsm.SystemSnapshot) []config.NmapConfig {
			return s.CurrentConfig.Internal.Nmap
		},
		NameOf:      func(c config.NmapConfig) string { return c.Name },
		CfgFor:      cfgFor,
		MapFresh:    mapFresh,
		MapObserved: mapObserved,
	})
}

// cfgFor renders an NmapConfig into the Upsert payload map through YAML, so the
// keys match NmapConfig's yaml tags. This matters because the child-spec
// pipeline YAML-marshals the map into UserSpec.Config and the worker
// YAML-unmarshals it back into an NmapConfig: the adapter's default CfgFor
// (a JSON round-trip) would emit Go field names — NmapConfig carries no json
// tags — and lose target/port on the way back.
func cfgFor(cfg config.NmapConfig) (map[string]any, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal nmap config: %w", err)
	}

	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal nmap config to map: %w", err)
	}

	return out, nil
}

// mapFresh maps a Fresh, healthy nmap observation to its fsmv1 operational
// state. It translates the polled port state to the matching operational-state
// constant; an empty or unknown port state maps to the starting state.
// Degraded, stale, and bootstrap verdicts are framework-owned and handled by
// the adapter, so this only classifies the healthy leaf.
func mapFresh(_ config.NmapConfig, s simple.Status[NmapStatus]) string {
	switch s.Result.PortState {
	case string(nmapfsm.PortStateOpen):
		return nmapfsm.OperationalStateOpen
	case string(nmapfsm.PortStateClosed):
		return nmapfsm.OperationalStateClosed
	case string(nmapfsm.PortStateFiltered):
		return nmapfsm.OperationalStateFiltered
	case string(nmapfsm.PortStateUnfiltered):
		return nmapfsm.OperationalStateUnfiltered
	case string(nmapfsm.PortStateOpenFiltered):
		return nmapfsm.OperationalStateOpenFiltered
	case string(nmapfsm.PortStateClosedFiltered):
		return nmapfsm.OperationalStateClosedFiltered
	default:
		return nmapfsm.OperationalStateStarting
	}
}

// mapObserved builds a nmapfsm.NmapObservedState from the config and the stored
// status: the observed service config comes from the config entry, and the
// ServiceInfo mirrors the last scan (port state, port, latency, running).
func mapObserved(cfg config.NmapConfig, s simple.Status[NmapStatus]) publicfsm.ObservedState {
	return nmapfsm.NmapObservedState{
		ObservedNmapServiceConfig: cfg.NmapServiceConfig,
		ServiceInfo: nmapservice.ServiceInfo{
			NmapStatus: nmapservice.NmapServiceInfo{
				IsRunning: s.Result.IsRunning,
				LastScan: &nmapservice.NmapScanResult{
					Timestamp: time.Now(),
					PortResult: nmapservice.PortResult{
						State:     s.Result.PortState,
						LatencyMs: s.Result.LatencyMs,
						Port:      s.Result.Port,
					},
				},
			},
		},
	}
}
