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

package benthos_monitor

import (
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	benthos_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// BenthosMonitorObservedStateSnapshot is a copy of the current metrics so the manager can store snapshots.
type BenthosMonitorObservedStateSnapshot struct {
	ServiceInfoSnapshot benthos_monitor_service.ServiceInfo
	// No need for the config, as it is basically empty
}

// Ensure it satisfies fsm.ObservedStateSnapshot
func (b *BenthosMonitorObservedStateSnapshot) IsObservedStateSnapshot() {}

// CreateObservedStateSnapshot is called by the manager to record the state
func (b *BenthosMonitorInstance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	snapshot := &BenthosMonitorObservedStateSnapshot{}
	if b.ObservedState.ServiceInfo != nil {
		err := deepcopy.Copy(&snapshot.ServiceInfoSnapshot, b.ObservedState.ServiceInfo)
		if err != nil {
			sentry.ReportIssue(err, sentry.IssueTypeError, b.baseFSMInstance.GetLogger())
		}
	}
	return snapshot
}
