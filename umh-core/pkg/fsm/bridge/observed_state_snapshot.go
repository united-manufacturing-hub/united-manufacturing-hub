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

package bridge

import (
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/bridgeserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/bridge"
)

// ObservedStateSnapshot is a deep-copyable snapshot of ProtocolConverterObservedState
type ObservedStateSnapshot struct {
	ObservedConfigRuntime bridgeserviceconfig.ConfigRuntime
	ObservedConfigSpec    bridgeserviceconfig.ConfigSpec
	ServiceInfo           bridge.ServiceInfo
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *ObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// CreateObservedStateSnapshot implements the fsm.ObservedStateConverter interface for Instance
func (i *Instance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &ObservedStateSnapshot{}

	// Deep copy observed bridge runtime config
	err := deepcopy.Copy(&snapshot.ObservedConfigRuntime, &i.ObservedState.ObservedConfigRuntime)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, i.baseFSMInstance.GetLogger(), "failed to deep copy observed bridge runtime config: %v", err)
		return nil
	}

	// Deep copy observed bridge spec config
	err = deepcopy.Copy(&snapshot.ObservedConfigSpec, &i.ObservedState.ObservedConfigSpec)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, i.baseFSMInstance.GetLogger(), "failed to deep copy observed bridge spec config: %v", err)
		return nil
	}

	// Deep copy service info
	err = deepcopy.Copy(&snapshot.ServiceInfo, &i.ObservedState.ServiceInfo)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, i.baseFSMInstance.GetLogger(), "failed to deep copy service info: %v", err)
		return nil
	}

	return snapshot
}
