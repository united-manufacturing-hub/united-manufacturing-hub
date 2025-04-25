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

package benthos

import (
	"github.com/tiendc/go-deepcopy"
	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// BenthosObservedStateSnapshot is a deep-copyable snapshot of BenthosObservedState
type BenthosObservedStateSnapshot struct {
	Config      benthosserviceconfig.BenthosServiceConfig
	ServiceInfo benthos.ServiceInfo
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *BenthosObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// CreateObservedStateSnapshot implements the fsm.ObservedStateConverter interface for BenthosInstance
func (b *BenthosInstance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &BenthosObservedStateSnapshot{}

	// Deep copy config
	err := deepcopy.Copy(&snapshot.Config, &b.config)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, b.baseFSMInstance.GetLogger(), "failed to deep copy config: %v", err)
		return nil
	}

	// Initialize BenthosLogs to empty slice if nil to prevent "slice index out of range" panic
	if b.ObservedState.ServiceInfo.BenthosStatus.BenthosLogs == nil {
		b.ObservedState.ServiceInfo.BenthosStatus.BenthosLogs = []s6service.LogEntry{}
	}

	// Deep copy service info
	err = deepcopy.Copy(&snapshot.ServiceInfo, &b.ObservedState.ServiceInfo)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, b.baseFSMInstance.GetLogger(), "failed to deep copy service info: %v", err)
		return nil
	}

	return snapshot
}
