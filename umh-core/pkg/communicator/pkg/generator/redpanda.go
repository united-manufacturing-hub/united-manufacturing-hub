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

// RedpandaFromSnapshot converts an optional FSMInstanceSnapshot into a
// models.Redpanda. Defaults are returned when inst == nil.
func RedpandaFromSnapshot(
	inst *fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) models.Redpanda {

	if inst == nil {
		return defaultRedpanda()
	}

	rp, err := buildRedpanda(*inst, log)
	if err != nil {
		log.Error("unable to build redpanda data", zap.Error(err))
		return defaultRedpanda()
	}
	return rp
}

// buildRedpanda maps a **non-nil** instance snapshot to models.Redpanda.
// It returns an error when the observed state cannot be cast.
func buildRedpanda(
	instance fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) (models.Redpanda, error) {

	snap, ok := instance.LastObservedState.(*redpanda.RedpandaObservedStateSnapshot)
	if !ok || snap == nil {
		return models.Redpanda{}, fmt.Errorf("invalid observed-state")
	}

	// Health ---------------------------------------------------------------
	healthCat := models.Neutral
	switch instance.CurrentState {
	case redpanda.OperationalStateActive:
		healthCat = models.Active
	case redpanda.OperationalStateDegraded:
		healthCat = models.Degraded
	}

	out := models.Redpanda{
		Health: &models.Health{
			Message:       snap.ServiceInfoSnapshot.StatusReason,
			ObservedState: instance.CurrentState,
			DesiredState:  instance.DesiredState,
			Category:      healthCat,
		},
	}

	// Metrics --------------------------------------------------------------
	m := snap.ServiceInfoSnapshot.RedpandaStatus.RedpandaMetrics.MetricsState
	if m != nil {
		out.AvgIncomingThroughputPerMinuteInMsgSec =
			float64(m.Input.BytesPerTick) / constants.DefaultTickerTime.Seconds()
		out.AvgOutgoingThroughputPerMinuteInMsgSec =
			float64(m.Output.BytesPerTick) / constants.DefaultTickerTime.Seconds()
	}
	return out, nil
}

// defaultRedpanda produces an empty Redpanda struct when there is no
// live data to populate it.
func defaultRedpanda() models.Redpanda { return models.Redpanda{} }
