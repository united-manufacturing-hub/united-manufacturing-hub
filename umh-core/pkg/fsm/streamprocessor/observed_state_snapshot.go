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

package streamprocessor

import (
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
)

// ObservedStateSnapshot is a deep-copyable snapshot of the ObservedState
type ObservedStateSnapshot struct {
	ObservedRuntimeConfig streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime
	ObservedSpecConfig    streamprocessorserviceconfig.StreamProcessorServiceConfigSpec
	ServiceInfo           spsvc.ServiceInfo
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *ObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// CreateObservedStateSnapshot implements the fsm.ObservedStateConverter interface for Instance
func (i *Instance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &ObservedStateSnapshot{}

	// Deep copy observed protocol converter runtime config
	err := deepcopy.Copy(&snapshot.ObservedRuntimeConfig, &i.ObservedState.ObservedRuntimeConfig)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, i.baseFSMInstance.GetLogger(), "failed to deep copy observed stream processor runtime config: %v", err)
		return nil
	}

	// Deep copy observed protocol converter spec config
	err = deepcopy.Copy(&snapshot.ObservedSpecConfig, &i.ObservedState.ObservedSpecConfig)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, i.baseFSMInstance.GetLogger(), "failed to deep copy observed stream processor spec config: %v", err)
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
