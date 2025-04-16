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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

// S6ObservedStateSnapshot is a deep-copyable snapshot of S6ObservedState
type S6ObservedStateSnapshot struct {
	Config                  config.S6FSMConfig
	LastStateChange         int64
	ServiceInfo             s6.ServiceInfo
	ObservedS6ServiceConfig s6serviceconfig.S6ServiceConfig
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *S6ObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// S6InstanceConverter implements the fsm.ObservedStateConverter interface for S6Instance
func (s *S6Instance) CreateObservedStateSnapshot() snapshot.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &S6ObservedStateSnapshot{
		LastStateChange: s.ObservedState.LastStateChange,
	}

	// Deep copy config
	deepcopy.Copy(&snapshot.Config, &s.config)

	// Deep copy service info
	deepcopy.Copy(&snapshot.ServiceInfo, &s.ObservedState.ServiceInfo)

	// Deep copy observed config
	deepcopy.Copy(&snapshot.ObservedS6ServiceConfig, &s.ObservedState.ObservedS6ServiceConfig)

	return snapshot
}
