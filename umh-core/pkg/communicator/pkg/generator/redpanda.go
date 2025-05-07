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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

func redpandaFromInstanceSnapshot(
	inst *fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) models.Redpanda {

	if inst == nil {
		return buildDefaultRedpandaData()
	}

	if rp, err := buildRedpandaDataFromInstanceSnapshot(*inst, log); err == nil {
		return rp
	}

	return models.Redpanda{}
}

// buildRedpandaDataFromSnapshot creates redpanda data from a FSM instance snapshot
// the instance will give us currentState, desiredState, etc. and the last observed state
func buildRedpandaDataFromInstanceSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (models.Redpanda, error) {
	redpandaData := models.Redpanda{}

	// Check if we have actual observedState
	if instance.LastObservedState != nil {

		// Step 1: Assemble Health Information

		// extract the current and desired state from the instance
		currentState := instance.CurrentState
		desiredState := instance.DesiredState

		// Now derive the health category from the current state
		healthCategory := models.Neutral
		switch currentState {
		case redpanda.OperationalStateActive:
			healthCategory = models.Active
		case redpanda.OperationalStateDegraded:
			healthCategory = models.Degraded
		}

		// Now derive the health message from the health category
		healthMessage := getHealthMessage(healthCategory)

		// Now assemble the health status
		redpandaData.Health = &models.Health{
			Message:       healthMessage,
			ObservedState: currentState,
			DesiredState:  desiredState,
			Category:      healthCategory,
		}

		// Step 2: Assemble Redpanda Metrics (AvgIncomingThroughputPerMinuteInMsgSec, AvgOutgoingThroughputPerMinuteInMsgSec)
		// Try to cast to the right type
		observedState, ok := instance.LastObservedState.(*redpanda.RedpandaObservedStateSnapshot)
		if !ok {
			err := fmt.Errorf("observed state %T does not match RedpandaObservedStateSnapshot", instance.LastObservedState)
			log.Error(err)
			return models.Redpanda{}, err
		}

		// Now fetch the last observed service info
		observedStateServiceInfo := observedState.ServiceInfoSnapshot

		// Now calculate the throughput metrics if available
		if observedStateServiceInfo.RedpandaStatus.RedpandaMetrics.MetricsState != nil {
			redpandaData.AvgIncomingThroughputPerMinuteInMsgSec = float64(observedStateServiceInfo.RedpandaStatus.RedpandaMetrics.MetricsState.Input.BytesPerTick) / constants.DefaultTickerTime.Seconds()
			redpandaData.AvgOutgoingThroughputPerMinuteInMsgSec = float64(observedStateServiceInfo.RedpandaStatus.RedpandaMetrics.MetricsState.Output.BytesPerTick) / constants.DefaultTickerTime.Seconds()
		}

	} else {
		log.Warn("no observed state found for redpanda", zap.String("instanceID", instance.ID))
		return models.Redpanda{}, fmt.Errorf("no observed state found for redpanda")
	}

	return redpandaData, nil
}

// buildDefaultRedpandaData creates default redpanda data when no real data is available
func buildDefaultRedpandaData() models.Redpanda {
	return models.Redpanda{}
}
