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

package nmap

import (
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	nmap_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// NmapObservedStateSnapshot is a copy of the current metrics so the manager can store snapshots.
type NmapObservedStateSnapshot struct {
	Config                    config.NmapConfig
	ObservedNmapServiceConfig nmapserviceconfig.NmapServiceConfig
	ServiceInfo               nmap_service.ServiceInfo
	// LastStateChange is not needed for Nmap but impl for consistency
	LastStateChange int64
}

// Ensure it satisfies fsm.ObservedStateSnapshot
func (n *NmapObservedStateSnapshot) IsObservedStateSnapshot() {}

// CreateObservedStateSnapshot is called by the manager to record the state
func (n *NmapInstance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	snapshot := &NmapObservedStateSnapshot{
		LastStateChange: n.ObservedState.LastStateChange,
	}

	// Deep copy config
	err := deepcopy.Copy(&snapshot.Config, &n.config)
	if err != nil {
		sentry.ReportFSMError(n.baseFSMInstance.GetLogger(), n.baseFSMInstance.GetID(), "nmap", "CreateObservedStateSnapshot", err)
		return nil
	}

	// Deep copy service info
	err = deepcopy.Copy(&snapshot.ServiceInfo, &n.ObservedState.ServiceInfo)
	if err != nil {
		sentry.ReportFSMError(n.baseFSMInstance.GetLogger(), n.baseFSMInstance.GetID(), "nmap", "CreateObservedStateSnapshot", err)
		return nil
	}

	// Deep copy observed config
	err = deepcopy.Copy(&snapshot.ObservedNmapServiceConfig, &n.ObservedState.ObservedNmapServiceConfig)
	if err != nil {
		sentry.ReportFSMError(n.baseFSMInstance.GetLogger(), n.baseFSMInstance.GetID(), "nmap", "CreateObservedStateSnapshot", err)
		return nil
	}

	return snapshot
}
