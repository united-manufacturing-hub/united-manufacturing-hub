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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	nmap_worker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/nmap"
	nmapservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	v2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// nmapStatusType is the observation type stored by the simple nmap worker.
type nmapStatusType = simple.Status[nmap_worker.NmapStatus]

// newNmapInstance creates an AdaptedInstance for one nmap scan target.
func newNmapInstance(cfg config.NmapConfig, sr deps.StateReader) publicfsm.FSMInstance {
	childID := v2config.ChildID(cfg.Name)

	return fsmv2config.NewAdaptedInstance[nmapStatusType, config.NmapConfig](
		sr,
		"nmap",
		childID,
		cfg,
		cfg.DesiredFSMState,
		5*time.Second,
		nmapMapState,
		nmapMapObservedState,
	)
}

func nmapMapState(obs fsmv2.Observation[nmapStatusType], _ config.NmapConfig) string {
	portState := obs.Status.Inner.PortState

	switch portState {
	case "open", "closed", "filtered", "unfiltered", "open|filtered", "closed|filtered", "degraded":
		return portState
	default:
		return nmapfsm.OperationalStateStarting
	}
}

func nmapMapObservedState(cfg config.NmapConfig, obs fsmv2.Observation[nmapStatusType]) publicfsm.ObservedState {
	result := nmapfsm.NmapObservedState{
		ObservedNmapServiceConfig: cfg.NmapServiceConfig,
		LastStateChange:           time.Now().Unix(),
	}

	inner := obs.Status.Inner
	if inner.PortState != "" || inner.ScanError != "" {
		result.ServiceInfo = nmapservice.ServiceInfo{
			NmapStatus: nmapservice.NmapServiceInfo{
				LastScan: &nmapservice.NmapScanResult{
					Timestamp: obs.CollectedAt,
					Error:     inner.ScanError,
					PortResult: nmapservice.PortResult{
						State:     inner.PortState,
						LatencyMs: inner.LatencyMs,
						Port:      inner.Port,
					},
				},
				IsRunning: inner.IsRunning,
			},
		}
	}

	return result
}
