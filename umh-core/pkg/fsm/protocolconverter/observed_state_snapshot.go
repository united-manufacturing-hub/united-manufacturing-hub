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

package protocolconverter

import (
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
)

// ProtocolConverterObservedStateSnapshot is a deep-copyable snapshot of BenthosObservedState
type ProtocolConverterObservedStateSnapshot struct {
	Config      dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	ServiceInfo dataflowcomponent.ServiceInfo
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *ProtocolConverterObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// CreateObservedStateSnapshot implements the fsm.ObservedStateConverter interface for DataflowComponentInstance
func (d *ProtocolConverterInstance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &ProtocolConverterObservedStateSnapshot{}

	// Deep copy config
	err := deepcopy.Copy(&snapshot.Config, &d.config)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, d.baseFSMInstance.GetLogger(), "failed to deep copy config: %v", err)
		return nil
	}

	// Deep copy service info
	err = deepcopy.Copy(&snapshot.ServiceInfo, &d.ObservedState.ServiceInfo)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, d.baseFSMInstance.GetLogger(), "failed to deep copy service info: %v", err)
		return nil
	}

	return snapshot
}
