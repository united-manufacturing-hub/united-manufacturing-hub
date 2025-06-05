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

// import (
// 	"fmt"

// 	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
// 	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
// 	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
// 	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
// 	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
// 	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
// 	"go.uber.org/zap"
// )

// // ProtocolConvertersFromSnapshot converts an optional ManagerSnapshot — potentially
// // holding **many** protocol converter instances — into a slice of models.Dfc with
// // type "protocol-converter".
// //
// // If mgr is nil or no instance could be converted, an empty slice is
// // returned.
// func ProtocolConvertersFromSnapshot(
// 	mgr fsm.ManagerSnapshot,
// 	log *zap.SugaredLogger,
// ) []models.Dfc {

// 	if mgr == nil {
// 		return []models.Dfc{}
// 	}

// 	var out []models.Dfc
// 	for _, inst := range mgr.GetInstances() {
// 		if d, err := buildProtocolConverter(*inst, log); err == nil {
// 			out = append(out, d)
// 		}
// 	}
// 	return out
// }

// // buildProtocolConverter translates one FSMInstanceSnapshot into a models.Dfc
// // with type "protocol-converter". It returns an error when the observed state
// // cannot be interpreted.
// func buildProtocolConverter(
// 	instance fsm.FSMInstanceSnapshot,
// 	log *zap.SugaredLogger,
// ) (models.Dfc, error) {

// 	observed, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot)
// 	if !ok || observed == nil {
// 		return models.Dfc{}, fmt.Errorf("observed state %T is not ProtocolConverterObservedStateSnapshot", instance.LastObservedState)
// 	}

// 	// Cast ServiceInfo to ProtocolConverter ServiceInfo which contains connection details
// 	serviceInfo, ok := observed.ServiceInfo.(protocolconvertersvc.ServiceInfo)
// 	if !ok {
// 		return models.Dfc{}, fmt.Errorf("serviceInfo %T is not protocolconverter.ServiceInfo", observed.ServiceInfo)
// 	}

// 	// ---- health ---------------------------------------------------------
// 	healthCat := models.Neutral
// 	healthMessage := serviceInfo.StatusReason
// 	if healthMessage == "" {
// 		healthMessage = "Protocol converter status unknown"
// 	}

// 	switch instance.CurrentState {
// 	case protocolconverter.OperationalStateActive:
// 		healthCat = models.Active
// 		if healthMessage == "Protocol converter status unknown" {
// 			healthMessage = "Protocol converter is active"
// 		}
// 	case protocolconverter.OperationalStateIdle:
// 		healthCat = models.Active
// 		if healthMessage == "Protocol converter status unknown" {
// 			healthMessage = "Protocol converter is idle"
// 		}
// 	case protocolconverter.OperationalStateDegradedConnection:
// 		healthCat = models.Degraded
// 		if healthMessage == "Protocol converter status unknown" {
// 			healthMessage = "Protocol converter connection degraded"
// 		}
// 	case protocolconverter.OperationalStateDegradedRedpanda:
// 		healthCat = models.Degraded
// 		if healthMessage == "Protocol converter status unknown" {
// 			healthMessage = "Protocol converter redpanda degraded"
// 		}
// 	case protocolconverter.OperationalStateDegradedDFC:
// 		healthCat = models.Degraded
// 		if healthMessage == "Protocol converter status unknown" {
// 			healthMessage = "Protocol converter dataflow component degraded"
// 		}
// 	case protocolconverter.OperationalStateDegradedOther:
// 		healthCat = models.Degraded
// 		if healthMessage == "Protocol converter status unknown" {
// 			healthMessage = "Protocol converter degraded"
// 		}
// 	}

// 	dfc := models.Dfc{
// 		Type: models.DfcTypeProtocolConverter,
// 		UUID: dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String(),
// 		Name: &instance.ID,
// 		Health: &models.Health{
// 			Message:       healthMessage,
// 			ObservedState: instance.CurrentState,
// 			DesiredState:  instance.DesiredState,
// 			Category:      healthCat,
// 		},
// 	}

// 	// ---- connections --------------------------------------------------------
// 	// For protocol-converter type, we need exactly one connection based on the
// 	// underlying connection information from the service info
// 	if serviceInfo.ConnectionObservedState.ServiceInfo.NmapConfig.Target != "" {
// 		connectionUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(
// 			fmt.Sprintf("%s-connection", instance.ID)).String()

// 		// Build URI from connection config
// 		target := serviceInfo.ConnectionObservedState.ServiceInfo.NmapConfig.Target
// 		port := serviceInfo.ConnectionObservedState.ServiceInfo.NmapConfig.Port
// 		uri := fmt.Sprintf("tcp://%s:%s", target, port)

// 		// Determine connection health based on connection FSM state
// 		connectionHealth := &models.Health{
// 			Message:       serviceInfo.ConnectionObservedState.ServiceInfo.StatusReason,
// 			ObservedState: serviceInfo.ConnectionFSMState,
// 			DesiredState:  "up", // Connections typically want to be "up"
// 			Category:      models.Neutral,
// 		}

// 		// Set connection health category based on state
// 		switch serviceInfo.ConnectionFSMState {
// 		case "up":
// 			connectionHealth.Category = models.Active
// 		case "degraded":
// 			connectionHealth.Category = models.Degraded
// 		}

// 		connection := models.Connection{
// 			Name:          fmt.Sprintf("%s-connection", instance.ID),
// 			UUID:          connectionUUID,
// 			Health:        connectionHealth,
// 			URI:           uri,
// 			LastLatencyMs: serviceInfo.ConnectionObservedState.ServiceInfo.NmapStatus.LastLatencyMs,
// 		}

// 		dfc.Connections = []models.Connection{connection}
// 	}

// 	// ---- metrics --------------------------------------------------------
// 	// Protocol converters can have metrics from their underlying DFC components

// 	// Try to get metrics from read DFC if it exists and has metrics
// 	if serviceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState != nil {
// 		m := serviceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState
// 		if m.Input.LastCount > 0 {
// 			dfc.Metrics = &models.DfcMetrics{
// 				AvgInputThroughputPerMinuteInMsgSec: m.Input.MessagesPerTick / constants.DefaultTickerTime.Seconds(),
// 			}
// 		}
// 	}

// 	return dfc, nil
// }
