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

package s6

import (
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
)

// S6ObservedStateSnapshot is a deep-copyable snapshot of S6ObservedState
type S6ObservedStateSnapshot struct {
	Config                  config.S6FSMConfig
	LastStateChange         int64
	ServiceInfo             process_shared.ServiceInfo
	ObservedS6ServiceConfig s6serviceconfig.S6ServiceConfig
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *S6ObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// S6InstanceConverter implements the fsm.ObservedStateConverter interface for S6Instance
func (s *S6Instance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &S6ObservedStateSnapshot{
		LastStateChange: s.ObservedState.LastStateChange,
	}

	// Deep copy config
	err := deepcopy.Copy(&snapshot.Config, &s.config)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.baseFSMInstance.GetLogger(), "failed to deep copy config: %v", err)
		return nil
	}

	// Deep copy service info
	err = deepcopy.Copy(&snapshot.ServiceInfo, &s.ObservedState.ServiceInfo)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.baseFSMInstance.GetLogger(), "failed to deep copy service info: %v", err)
		return nil
	}

	// Deep copy observed config
	err = deepcopy.Copy(&snapshot.ObservedS6ServiceConfig, &s.ObservedState.ObservedS6ServiceConfig)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.baseFSMInstance.GetLogger(), "failed to deep copy observed config: %v", err)
		return nil
	}

	return snapshot
}
