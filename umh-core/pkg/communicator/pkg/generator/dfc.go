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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

func dfcsFromManagerSnapshot(
	mgr fsm.ManagerSnapshot, // data-flow can have many instances
	log *zap.SugaredLogger,
) []models.Dfc {

	if mgr == nil {
		return buildDefaultDataFlowComponentData()
	}
	var out []models.Dfc
	for _, inst := range mgr.GetInstances() {
		if d, err := buildDataFlowComponentDataFromSnapshot(*inst, log); err == nil {
			out = append(out, d)
		}
	}
	return out
}

func buildDataFlowComponentDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (models.Dfc, error) {
	dfcData := models.Dfc{}

	// Check if we have actual observedState
	if instance.LastObservedState != nil {
		// Try to cast to the right type

		// Create health status based on instance.CurrentState
		extractHealthStatus := func(state string) models.HealthCategory {
			switch state {
			case dataflowcomponent.OperationalStateActive:
				return models.Active
			case dataflowcomponent.OperationalStateDegraded:
				return models.Degraded
			default:
				return models.Neutral
			}
		}

		// get the metrics from the instance
		observed, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
		if !ok {
			err := fmt.Errorf("observed state %T does not match DataflowComponentObservedStateSnapshot", instance.LastObservedState)
			log.Error(err)
			return models.Dfc{}, err
		}
		serviceInfo := observed.ServiceInfo
		inputThroughput := float64(0)
		if serviceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState != nil && serviceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState.Input.LastCount > 0 {
			inputThroughput = serviceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState.Input.MessagesPerTick / constants.DefaultTickerTime.Seconds()
		}

		dfcData.Health = &models.Health{
			Message:       getDataflowHealthMessage(extractHealthStatus(instance.CurrentState), *observed),
			ObservedState: instance.CurrentState,
			DesiredState:  instance.DesiredState,
			Category:      extractHealthStatus(instance.CurrentState),
		}

		dfcData.Type = "custom" // this is a custom DFC; protocol converters will have a separate fsm
		dfcData.UUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String()
		dfcData.Metrics = &models.DfcMetrics{
			AvgInputThroughputPerMinuteInMsgSec: inputThroughput,
		}
		dfcData.Name = &instance.ID
	} else {
		log.Warn("no observed state found for dataflowcomponent", zap.String("instanceID", instance.ID))
		return models.Dfc{}, fmt.Errorf("no observed state found for dataflowcomponent")
	}

	return dfcData, nil
}

func buildDefaultDataFlowComponentData() []models.Dfc {
	return []models.Dfc{}
}

// getDataflowHealthMessage returns an appropriate message for dataflow components
func getDataflowHealthMessage(health models.HealthCategory, serviceInfo dataflowcomponent.DataflowComponentObservedStateSnapshot) string {
	switch health {
	case models.Active:
		return "Component is operating normally"
	case models.Degraded:
		if serviceInfo.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosDegradedLog.Content != "" {
			return fmt.Sprintf("Component is degraded because of a bad log entry. If the problem persists, please check the logs for more information. Log entry: [ %s ] %s",
				serviceInfo.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosDegradedLog.Timestamp.String(),
				serviceInfo.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosDegradedLog.Content)
		}
		return "Component stopped working"
	case models.Neutral:
		return "Component status is neutral"
	default:
		return "Component status unknown"
	}
}
