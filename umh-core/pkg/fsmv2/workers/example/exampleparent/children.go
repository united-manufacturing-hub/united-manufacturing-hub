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

package exampleparent

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// RenderChildren returns the ChildSpec set for the exampleparent worker.
//
// Each child receives cfg.ChildConfig as its UserSpec.Config and a per-child
// DEVICE_ID variable (device-0, device-1, ...). ChildStartStates is set to
// ["TryingToStart", "Running"] so each child starts when the parent enters
// either of those states.
//
// When cfg.ChildConfig is empty, each child's UserSpec.Config is empty as
// well. No default template is injected; ExamplechildConfig has no address
// or device fields that would consume one.
//
// The enabled parameter controls whether children should be active. Alive-
// trajectory states (TryingToStart, Running, Degraded) pass enabled=true.
// Stop-trajectory states (TryingToStop, Stopped) pass an empty slice
// directly rather than calling RenderChildren: exampleparent's children
// are stateless.
//
// For the resident-disable variant (enabled=false keeps children in
// Stopped instead of despawning), see workers/example/examplechild/state/state_stopped.go
// and workers/transport.
func RenderChildren(cfg ExampleparentConfig, enabled bool) ([]config.ChildSpec, error) {
	childWorkerType := cfg.GetChildWorkerType()
	count := cfg.ChildrenCount

	specs := make([]config.ChildSpec, 0, count)

	for i := range count {
		childVariables := config.VariableBundle{
			User: map[string]any{
				"DEVICE_ID": fmt.Sprintf("device-%d", i),
			},
		}

		specs = append(specs, config.ChildSpec{
			Name:       fmt.Sprintf("child-%d", i),
			WorkerType: childWorkerType,
			UserSpec: config.UserSpec{
				Config:    cfg.ChildConfig,
				Variables: childVariables,
			},
			ChildStartStates: []string{"TryingToStart", "Running"},
			Enabled:          enabled,
		})
	}

	return specs, nil
}
