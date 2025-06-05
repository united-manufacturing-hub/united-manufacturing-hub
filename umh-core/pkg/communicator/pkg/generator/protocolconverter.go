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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// ProtocolConvertersFromSnapshot converts an optional ManagerSnapshot — potentially
// holding **many** protocol converter instances — into a slice of models.Dfc with type "protocol-converter".
//
// If mgr is nil or no instance could be converted, an empty slice is
// returned.
func ProtocolConvertersFromSnapshot(
	mgr fsm.ManagerSnapshot,
	log *zap.SugaredLogger,
) []models.Dfc {

	if mgr == nil {
		return []models.Dfc{}
	}

	var out []models.Dfc
	for _, inst := range mgr.GetInstances() {
		if pc, err := buildProtocolConverterAsDfc(*inst, log); err == nil {
			out = append(out, pc)
		}
	}
	return out
}

// buildProtocolConverterAsDfc translates one Protocol Converter FSMInstanceSnapshot into a models.Dfc
// with type "protocol-converter". It returns an error when the observed state cannot be interpreted.
func buildProtocolConverterAsDfc(
	instance fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) (models.Dfc, error) {

	observed, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot)
	if !ok || observed == nil {
		return models.Dfc{}, fmt.Errorf("observed state %T is not ProtocolConverterObservedStateSnapshot", instance.LastObservedState)
	}

	// ---- health ---------------------------------------------------------
	healthCat := models.Neutral
	switch instance.CurrentState {
	case protocolconverter.OperationalStateActive:
		healthCat = models.Active
	case protocolconverter.OperationalStateDegradedConnection,
		protocolconverter.OperationalStateDegradedRedpanda,
		protocolconverter.OperationalStateDegradedDFC,
		protocolconverter.OperationalStateDegradedOther:
		healthCat = models.Degraded
	case protocolconverter.OperationalStateIdle:
		healthCat = models.Neutral
	}

	// Generate UUID from the protocol converter name
	uuid := dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID)

	// Create connection info for protocol converter
	var connections []models.Connection
	if observed.ObservedProtocolConverterConfig.ConnectionServiceConfig.NmapServiceConfig.Target != "" {
		connection := models.Connection{
			Name: instance.ID + "-connection",
			UUID: dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID + "-connection").String(), // Derive connection UUID from PC UUID
			URI:  fmt.Sprintf("%s:%d", observed.ObservedProtocolConverterConfig.ConnectionServiceConfig.NmapServiceConfig.Target, observed.ObservedProtocolConverterConfig.ConnectionServiceConfig.NmapServiceConfig.Port),
			Health: &models.Health{
				Message:       observed.ServiceInfo.StatusReason,
				ObservedState: observed.ServiceInfo.ConnectionFSMState,
				DesiredState:  "up", // Connection desired state is typically "up"
				Category:      getHealthCategoryFromState(observed.ServiceInfo.ConnectionFSMState),
			},
			LastLatencyMs: 0.0, // TODO: Add actual latency data when available
		}
		connections = append(connections, connection)
	}

	//check if the protocol converter is initialized by checking if a read dfc is present
	isInitialized := false
	input := observed.ObservedProtocolConverterConfig.DataflowComponentReadServiceConfig.BenthosConfig.Input
	if input != nil && len(input) > 0 {
		isInitialized = true
	}

	dfc := models.Dfc{
		Type:        models.DfcTypeProtocolConverter,
		UUID:        uuid.String(),
		Name:        &instance.ID,
		Connections: connections,
		Health: &models.Health{
			Message:       getProtocolConverterStatusMessage(instance.CurrentState),
			ObservedState: instance.CurrentState,
			DesiredState:  instance.DesiredState,
			Category:      healthCat,
		},
		// Metrics are not implemented yet for protocol converters
		Metrics: nil,
		// Bridge info is not applicable for protocol converters
		Bridge:        nil,
		IsInitialized: isInitialized,
	}

	return dfc, nil
}

// getProtocolConverterStatusMessage returns a human-readable status message for the given state
func getProtocolConverterStatusMessage(state string) string {
	switch state {
	case protocolconverter.OperationalStateActive:
		return "Protocol converter is active and processing data"
	case protocolconverter.OperationalStateIdle:
		return "Protocol converter is idle"
	case protocolconverter.OperationalStateStopped:
		return "Protocol converter is stopped"
	case protocolconverter.OperationalStateDegradedConnection:
		return "Protocol converter connection is degraded"
	case protocolconverter.OperationalStateDegradedRedpanda:
		return "Protocol converter Redpanda connection is degraded"
	case protocolconverter.OperationalStateDegradedDFC:
		return "Protocol converter data flow component is degraded"
	case protocolconverter.OperationalStateDegradedOther:
		return "Protocol converter has other degradation issues"
	default:
		return fmt.Sprintf("Protocol converter state: %s", state)
	}
}

// getHealthCategoryFromState converts a FSM state string to models.HealthCategory
func getHealthCategoryFromState(state string) models.HealthCategory {
	switch state {
	case "up", "active":
		return models.Active
	case "down", "degraded", "stopped":
		return models.Degraded
	case "idle":
		return models.Neutral
	default:
		return models.Neutral
	}
}
