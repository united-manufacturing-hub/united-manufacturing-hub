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
// DEVICE_ID variable (device-0, device-1, …). ChildStartStates mirrors the
// legacy DeriveDesiredState builder so children start when the parent enters
// TryingToStart or Running.
//
// When cfg.ChildConfig == "", each child's UserSpec.Config is an empty string. The
// legacy DeriveDesiredState injected a default template ("address: {{ .IP }}:{{ .PORT }}
// device: {{ .DEVICE_ID }}") for unset ChildConfig; that default is intentionally not
// replicated — ExamplechildConfig has no typed address/device fields so the template
// was never consumed, and no active scenario relies on it.
//
// The enabled parameter controls whether children should be active. Alive-trajectory
// states (TryingToStart, Running, Degraded) pass enabled=true. Stop-trajectory states
// (TryingToStop, Stopped) pass the empty slice []config.ChildSpec{} directly rather
// than calling RenderChildren — exampleparent is stateless (no buffer holders), so
// children are fully despawned rather than resident-disabled.
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
