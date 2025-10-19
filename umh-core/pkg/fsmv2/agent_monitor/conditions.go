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

package agent_monitor

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func IsFullyHealthy(observed *AgentMonitorObservedState) bool {
	return observed.ServiceInfo != nil && observed.ServiceInfo.OverallHealth == models.Active
}

func BuildDegradedReason(observed *AgentMonitorObservedState) string {
	if observed.ServiceInfo == nil {
		return "No service info available"
	}

	return fmt.Sprintf("Agent health: %s (Latency: %s, Release: %s)",
		observed.ServiceInfo.OverallHealth,
		observed.ServiceInfo.LatencyHealth,
		observed.ServiceInfo.ReleaseHealth,
	)
}
