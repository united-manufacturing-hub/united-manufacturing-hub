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

package action

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
)

type RunMaintenanceAction struct{}

func NewRunMaintenanceAction() *RunMaintenanceAction {
	return &RunMaintenanceAction{}
}

func (a *RunMaintenanceAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	d, ok := depsAny.(snapshot.PersistenceDependencies)
	if !ok {
		return fmt.Errorf("unexpected deps type: %T", depsAny)
	}

	start := time.Now()

	if err := d.GetStore().Maintenance(ctx); err != nil {
		return fmt.Errorf("maintenance failed: %w", err)
	}

	now := time.Now()
	duration := now.Sub(start)

	d.SetLastMaintenanceAt(now)
	d.ActionLogger("run_maintenance").Info("maintenance completed",
		deps.Int64("duration_ms", duration.Milliseconds()))
	d.MetricsRecorder().IncrementCounter(deps.CounterMaintenanceCyclesTotal, 1)
	d.MetricsRecorder().SetGauge(deps.GaugeLastMaintenanceDurationMs, float64(duration.Milliseconds()))

	return nil
}

func (a *RunMaintenanceAction) Name() string { return "RunMaintenance" }
