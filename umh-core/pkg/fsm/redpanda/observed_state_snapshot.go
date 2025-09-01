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

package redpanda

import (
	"github.com/tiendc/go-deepcopy"
	redpandaserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
)

// RedpandaObservedStateSnapshot is a deep-copyable snapshot of BenthosObservedState.
type RedpandaObservedStateSnapshot struct {
	Config              redpandaserviceconfig.RedpandaServiceConfig
	ServiceInfoSnapshot redpanda.ServiceInfo
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface.
func (s *RedpandaObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// CreateObservedStateSnapshot implements the fsm.ObservedStateConverter interface for RedpandaInstance.
func (d *RedpandaInstance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &RedpandaObservedStateSnapshot{}

	// Deep copy config
	err := deepcopy.Copy(&snapshot.Config, &d.config)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, d.baseFSMInstance.GetLogger(), "failed to deep copy config: %v", err)

		return nil
	}

	// Deep copy service info
	err = deepcopy.Copy(&snapshot.ServiceInfoSnapshot, &d.PreviousObservedState.ServiceInfo)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, d.baseFSMInstance.GetLogger(), "failed to deep copy service info: %v", err)

		return nil
	}

	return snapshot
}
