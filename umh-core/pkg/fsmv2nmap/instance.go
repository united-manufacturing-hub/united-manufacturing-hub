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
	fsmv2adapter "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	nmap_worker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/nmap"
	nmapservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

const (
	// nmapMinRequiredTime is how long the fsmv1 control loop waits before
	// considering a state transition stable.
	nmapMinRequiredTime = 5 * time.Second

	// nmapMaxAge is how old a nmap observation may be before the child is
	// considered stale (unreachable). Three missed 1-second scan intervals.
	nmapMaxAge = 3 * time.Second
)

// newNmapInstance creates an AdaptedInstance for one nmap scan target.
// It reads via the global FSMv2Client; no local store is needed.
func newNmapInstance(cfg config.NmapConfig) publicfsm.FSMInstance {
	ref := dynamicchildren.Ref{WorkerType: "nmap", Name: cfg.Name}

	return fsmv2adapter.NewAdaptedInstance(
		ref,
		cfg,
		cfg.DesiredFSMState,
		nmapMinRequiredTime,
		nmapMaxAge,
		nmapMapState,
		nmapMapObservedState,
	)
}

func nmapMapState(obs fsmv2.Observation[nmap_worker.NmapStatus], freshness fsmv2client.Freshness, _ config.NmapConfig) string {
	switch freshness {
	case fsmv2client.Unregistered, fsmv2client.NeverObserved, fsmv2client.Unknown:
		return nmapfsm.OperationalStateStarting
	case fsmv2client.Stale:
		return "degraded"
	}

	// Fresh — map port state.
	portState := obs.Status.PortState
	switch portState {
	case "open", "closed", "filtered", "unfiltered", "open|filtered", "closed|filtered", "degraded":
		return portState
	default:
		return nmapfsm.OperationalStateStarting
	}
}

func nmapMapObservedState(cfg config.NmapConfig, obs fsmv2.Observation[nmap_worker.NmapStatus], freshness fsmv2client.Freshness) publicfsm.ObservedState {
	result := nmapfsm.NmapObservedState{
		ObservedNmapServiceConfig: cfg.NmapServiceConfig,
		LastStateChange:           time.Now().Unix(),
	}

	if freshness != fsmv2client.Fresh && freshness != fsmv2client.Stale {
		return result
	}

	inner := obs.Status
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
