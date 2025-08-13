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

// DfcsFromSnapshot converts an optional ManagerSnapshot — potentially
// holding **many** instances — into a slice of models.Dfc.
//
// If mgr is nil or no instance could be converted, an empty slice is
// returned.  This plural form is the only difference compared with the
// single-instance helpers (AgentFromSnapshot, ContainerFromSnapshot,
// …) in the other files.
func DfcsFromSnapshot(
	mgr fsm.ManagerSnapshot,
	log *zap.SugaredLogger,
) []models.Dfc {
	if mgr == nil {
		return defaultDfcs()
	}

	var out []models.Dfc

	for _, inst := range mgr.GetInstances() {
		d, err := buildDfc(*inst, log)
		if err == nil {
			out = append(out, d)
		}
	}

	return out
}

// buildDfc translates one FSMInstanceSnapshot into a models.Dfc.  It
// returns an error when the observed state cannot be interpreted.
func buildDfc(
	instance fsm.FSMInstanceSnapshot,
	_ *zap.SugaredLogger,
) (models.Dfc, error) {
	observed, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
	if !ok || observed == nil {
		return models.Dfc{}, fmt.Errorf("observed state %T is not DataflowComponentObservedStateSnapshot", instance.LastObservedState)
	}

	// ---- health ---------------------------------------------------------
	healthCat := models.Neutral

	switch instance.CurrentState {
	case dataflowcomponent.OperationalStateActive:
		healthCat = models.Active
	case dataflowcomponent.OperationalStateDegraded:
		healthCat = models.Degraded
	}

	dfc := models.Dfc{
		Type: "custom",
		UUID: dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String(),
		Name: &instance.ID,
		Health: &models.Health{
			Message:       observed.ServiceInfo.StatusReason,
			ObservedState: instance.CurrentState,
			DesiredState:  instance.DesiredState,
			Category:      healthCat,
		},
	}

	// ---- metrics --------------------------------------------------------
	svcInfo := observed.ServiceInfo
	if m := svcInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState; m != nil &&
		m.Input.LastCount > 0 {
		avgThroughput := m.Input.MessagesPerTick / constants.DefaultTickerTime.Seconds()
		if instance.DesiredState == dataflowcomponent.OperationalStateStopped {
			avgThroughput = 0
		}

		dfc.Metrics = &models.DfcMetrics{
			AvgInputThroughputPerMinuteInMsgSec: avgThroughput,
		}
	}

	return dfc, nil
}

// defaultDfcs returns an empty slice when the manager snapshot is nil
// or unusable.
func defaultDfcs() []models.Dfc { return []models.Dfc{} }
