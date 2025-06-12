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

package topicbrowser

import (
	"github.com/tiendc/go-deepcopy"
	topicbrowsersvccfg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

// ObservedStateSnapshot is a deep-copyable snapshot of BenthosObservedState
type ObservedStateSnapshot struct {
	Config      topicbrowsersvccfg.Config
	ServiceInfo topicbrowsersvc.ServiceInfo
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *ObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// CreateObservedStateSnapshot implements the fsm.ObservedStateConverter interface for the topic browser Instance
func (i *Instance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &ObservedStateSnapshot{}

	// Deep copy config
	err := deepcopy.Copy(&snapshot.Config, &i.config)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, i.baseFSMInstance.GetLogger(), "failed to deep copy config: %v", err)
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
