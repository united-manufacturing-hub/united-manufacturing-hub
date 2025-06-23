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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
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
	if observed.ObservedProtocolConverterRuntimeConfig.ConnectionServiceConfig.NmapServiceConfig.Target != "" {
		connection := models.Connection{
			Name: instance.ID + "-connection",
			UUID: dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID + "-connection").String(), // Derive connection UUID from PC UUID
			URI:  fmt.Sprintf("%s:%d", observed.ObservedProtocolConverterRuntimeConfig.ConnectionServiceConfig.NmapServiceConfig.Target, observed.ObservedProtocolConverterRuntimeConfig.ConnectionServiceConfig.NmapServiceConfig.Port),
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

	// var templatePort string
	// if observed.ObservedProtocolConverterTemplateConfig.ConnectionServiceConfig.NmapTemplate != nil {
	// 	templatePort = observed.ObservedProtocolConverterTemplateConfig.ConnectionServiceConfig.NmapTemplate.Port
	// } else {
	// 	templatePort = "not found"
	// }

	//check if the protocol converter is initialized by checking if a read dfc is present
	isInitialized := false
	input := observed.ObservedProtocolConverterRuntimeConfig.DataflowComponentReadServiceConfig.BenthosConfig.Input
	if len(input) > 0 {
		isInitialized = true
	}

	dfc := models.Dfc{
		Type:        models.DfcTypeProtocolConverter,
		UUID:        uuid.String(),
		Name:        &instance.ID,
		Connections: connections,
		Health: &models.Health{
			Message:       getProtocolConverterStatusMessage(instance.CurrentState, observed.ServiceInfo.StatusReason, observed.ServiceInfo.ConnectionFSMState, observed.ServiceInfo.ConnectionObservedState.ServiceInfo.NmapFSMState),
			ObservedState: instance.CurrentState,
			DesiredState:  instance.DesiredState,
			Category:      healthCat,
		},
		// Metrics are added below
		Metrics: nil,
		// Bridge info is not applicable for protocol converters
		Bridge:        nil,
		IsInitialized: isInitialized,
	}

	// ---- metrics --------------------------------------------------------
	svcInfo := observed.ServiceInfo
	if m := svcInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState; m != nil &&
		m.Input.LastCount > 0 {

		dfc.Metrics = &models.DfcMetrics{
			AvgInputThroughputPerMinuteInMsgSec: m.Input.MessagesPerTick / constants.DefaultTickerTime.Seconds(),
		}
	}

	return dfc, nil
}

// getProtocolConverterStatusMessage returns a human-readable status message for the given state
func getProtocolConverterStatusMessage(state string, statusReason string, connectionState string, nmapState string) string {
	baseMessage := ""
	connectionSuffix := ""

	// Get base message from protocol converter state
	switch state {
	case protocolconverter.OperationalStateActive:
		baseMessage = "Protocol converter is active and processing data"
	case protocolconverter.OperationalStateIdle:
		baseMessage = "Protocol converter is idle"
	case protocolconverter.OperationalStateStopped:
		baseMessage = "Protocol converter is stopped"
	case protocolconverter.OperationalStateDegradedConnection:
		baseMessage = "Protocol converter connection is degraded"
	case protocolconverter.OperationalStateDegradedRedpanda:
		baseMessage = "Protocol converter Redpanda connection is degraded"
	case protocolconverter.OperationalStateDegradedDFC:
		baseMessage = "Protocol converter data flow component is degraded"
	case protocolconverter.OperationalStateDegradedOther:
		baseMessage = "Protocol converter has other degradation issues"
	case protocolconverter.OperationalStateStartingFailedDFCMissing:
		baseMessage = "No DFC added yet"
	default:
		baseMessage = fmt.Sprintf("Protocol converter state: %s", state)
	}

	// Add connection state information if available
	if connectionState != "" {
		switch connectionState {
		case connection.OperationalStateUp:
			if state == protocolconverter.OperationalStateActive {
				connectionSuffix = " with healthy connection"
			}
		case connection.OperationalStateDown:
			connectionSuffix = " - connection is down"
		case connection.OperationalStateDegraded:
			connectionSuffix = " - connection is unstable"
		case connection.OperationalStateStarting:
			connectionSuffix = " - connection is being established"
		default:
			// For specific error states or unknown states, include the raw connection state
			if connectionState != "unknown" && connectionState != "" {
				connectionSuffix = fmt.Sprintf(" - connection: %s", connectionState)
			}
		}
	}

	// Add Nmap state information if available
	if nmapState != "" {
		nmapSuffix := ""
		switch nmapState {
		case nmap.OperationalStateOpen:
			nmapSuffix = " (port is open)"
		case nmap.OperationalStateClosed:
			nmapSuffix = " (port is closed)"
		case nmap.OperationalStateFiltered:
			nmapSuffix = " (port is filtered by firewall)"
		case nmap.OperationalStateUnfiltered:
			nmapSuffix = " (port is unfiltered)"
		case nmap.OperationalStateOpenFiltered:
			nmapSuffix = " (port is open or filtered)"
		case nmap.OperationalStateClosedFiltered:
			nmapSuffix = " (port is closed or filtered)"
		case nmap.OperationalStateStarting:
			nmapSuffix = " (nmap is starting)"
		case nmap.OperationalStateStopped:
			nmapSuffix = " (nmap is stopped)"
		case nmap.OperationalStateDegraded:
			nmapSuffix = " (nmap execution failed)"
		default:
			nmapSuffix = fmt.Sprintf(" (unexpected nmap state: %s)", nmapState)
		}

		// Only add nmap details if they add meaningful information
		if connectionSuffix == "" {
			connectionSuffix = nmapSuffix
		} else if nmapSuffix != "" {
			connectionSuffix += nmapSuffix
		}
	}

	return baseMessage + connectionSuffix + " - " + statusReason
}

// getHealthCategoryFromState converts a FSM state string to models.HealthCategory
func getHealthCategoryFromState(state string) models.HealthCategory {
	switch state {
	case connection.OperationalStateUp:
		return models.Active
	case connection.OperationalStateDown, connection.OperationalStateDegraded, connection.OperationalStateStopped:
		return models.Degraded
	case protocolconverter.OperationalStateIdle:
		return models.Neutral
	default:
		return models.Neutral
	}
}
