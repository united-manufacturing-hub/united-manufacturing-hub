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
	// Agent health check
	if agentHealth != nil && agentHealth.Category != models.Active {
		unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("Agent: %s", agentHealth.Message))
	}

	// Container health check
	if containerHealth != nil && containerHealth.Category != models.Active {
		unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("Container: %s", containerHealth.Message))
	}

	// Redpanda health check
	if redpandaHealth != nil && redpandaHealth.Category != models.Active {
		unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("Redpanda: %s", redpandaHealth.Message))
	}

	// Release health check
	if releaseHealth != nil && releaseHealth.Category != models.Active {
		unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("Release: %s", releaseHealth.Message))
	}

	// DFCs health check
	for _, dfc := range dfcs {
		if dfc.Health != nil && dfc.Health.Category != models.Active {
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
		coreHealth.Message = fmt.Sprintf("Unhealthy components: %v", unhealthyComponents)
		logger.Warnf("Core health is degraded: %s", coreHealth.Message)
	}

	return coreHealth
}

// GetCoreHealthMessage returns a health message based on the provided health category
func GetCoreHealthMessage(cat models.HealthCategory) string {
	switch cat {
	case models.Active:
		return "Core operating normally"
	case models.Degraded:
		return "Core degraded"
	default:
		return "Core status unknown"
	}
}
