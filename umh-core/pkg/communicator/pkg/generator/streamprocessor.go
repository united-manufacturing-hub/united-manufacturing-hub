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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// StreamProcessorsFromSnapshot converts an optional ManagerSnapshot — potentially
// holding **many** stream processor instances — into a slice of models.Dfc with type "stream-processor".
//
// If mgr is nil or no instance could be converted, an empty slice is
// returned.
func StreamProcessorsFromSnapshot(
	mgr fsm.ManagerSnapshot,
	log *zap.SugaredLogger,
) []models.Dfc {

	if mgr == nil {
		return []models.Dfc{}
	}

	var out []models.Dfc
	for _, inst := range mgr.GetInstances() {
		if sp, err := buildStreamProcessorAsDfc(*inst); err == nil {
			out = append(out, sp)
		}
	}
	return out
}

// buildStreamProcessorAsDfc translates one Stream Processor FSMInstanceSnapshot into a models.Dfc
// with type "stream-processor". It returns an error when the observed state cannot be interpreted.
func buildStreamProcessorAsDfc(
	instance fsm.FSMInstanceSnapshot,
) (models.Dfc, error) {

	observed, ok := instance.LastObservedState.(*streamprocessor.ObservedStateSnapshot)
	if !ok || observed == nil {
		return models.Dfc{}, fmt.Errorf("observed state %T is not ObservedStateSnapshot", instance.LastObservedState)
	}

	// ---- health ---------------------------------------------------------
	healthCat := models.Neutral
	switch instance.CurrentState {
	case streamprocessor.OperationalStateActive:
		healthCat = models.Active
	case streamprocessor.OperationalStateDegradedRedpanda,
		streamprocessor.OperationalStateDegradedDFC,
		streamprocessor.OperationalStateDegradedOther:
		healthCat = models.Degraded
	case streamprocessor.OperationalStateIdle:
		healthCat = models.Neutral
	}

	// Generate UUID from the stream processor name
	uuid := dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID)

	health := &models.Health{
		Message:       fmt.Sprintf("Stream processor %s", instance.CurrentState),
		ObservedState: instance.CurrentState,
		DesiredState:  instance.DesiredState,
		Category:      healthCat,
	}

	// Create DFC with stream processor type
	dfc := models.Dfc{
		Name:          &instance.ID,
		UUID:          uuid.String(),
		Health:        health,
		Type:          models.DfcTypeStreamProcessor,
		Metrics:       nil,
		Connections:   []models.Connection{}, // Stream processors don't have direct connections
		IsInitialized: true,
	}

	svcInfo := observed.ServiceInfo
	if m := svcInfo.DFCObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState; m != nil &&
		m.Input.LastCount > 0 {

		dfc.Metrics = &models.DfcMetrics{
			AvgInputThroughputPerMinuteInMsgSec: m.Output.MessagesPerTick / constants.DefaultTickerTime.Seconds(),
		}
	}

	return dfc, nil
}
