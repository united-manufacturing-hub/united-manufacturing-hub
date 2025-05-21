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

package generator

import (
	"fmt"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// DeriveCoreHealth determines the overall health of the core component
// based on the health of all child components
func DeriveCoreHealth(
	agentHealth *models.Health,
	containerHealth *models.Health,
	redpandaHealth *models.Health,
	releaseHealth *models.Health,
	dfcs []models.Dfc,
	logger *zap.SugaredLogger,
) *models.Health {
	var unhealthyComponents []string

	// Check each component's health
	// Agent health check - only consider degraded as unhealthy
	if agentHealth != nil && agentHealth.Category == models.Degraded {
		unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("Agent: %s", agentHealth.Message))
	}

	// Container health check - only consider degraded as unhealthy
	if containerHealth != nil && containerHealth.Category == models.Degraded {
		unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("Container: %s", containerHealth.Message))
	}

	// Redpanda health check - both active and neutral (idle) states are acceptable
	if redpandaHealth != nil && !(redpandaHealth.Category == models.Active || redpandaHealth.Category == models.Neutral) {
		unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("Redpanda: %s", redpandaHealth.Message))
	}

	// Release health check - only consider degraded as unhealthy
	if releaseHealth != nil && releaseHealth.Category == models.Degraded {
		unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("Release: %s", releaseHealth.Message))
	}

	// DFCs health check
	for _, dfc := range dfcs {
		// Both Active and Idle are considered healthy states for DFCs
		// Also ignore components that are starting
		if dfc.Health != nil &&
			!(dfc.Health.ObservedState == dataflowcomponent.OperationalStateActive ||
				dfc.Health.ObservedState == dataflowcomponent.OperationalStateIdle ||
				strings.Contains(strings.ToLower(dfc.Health.ObservedState), "starting")) {
			unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("DFC %s: %s", dfc.UUID, dfc.Health.Message))
		}
	}

	// Determine overall health
	coreHealth := &models.Health{
		ObservedState: "running",
		DesiredState:  "running",
		Category:      models.Active,
		Message:       "All core components are healthy",
	}

	if len(unhealthyComponents) > 0 {
		coreHealth.Category = models.Degraded
		coreHealth.Message = fmt.Sprintf("Unhealthy components:\n%s", strings.Join(unhealthyComponents, "\n"))
		logger.Debugf("Core health is degraded: %s", coreHealth.Message)
	}

	return coreHealth
}
