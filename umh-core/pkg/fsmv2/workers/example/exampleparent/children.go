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
// Each child receives a config.BaseUserSpec (ExamplechildConfig has no additional
// fields beyond BaseUserSpec). ChildStartStates mirrors the legacy DeriveDesiredState
// builder so children start when the parent enters TryingToStart or Running.
//
// The enabled parameter controls whether children should be active. Alive-trajectory
// states (TryingToStart, Running, Degraded) pass enabled=true. Stop-trajectory states
// (TryingToStop, Stopped) pass the empty slice []config.ChildSpec{} directly rather
// than calling RenderChildren — exampleparent is stateless (no buffer holders), so
// children are fully despawned rather than resident-disabled.
// cfg.ChildConfig is intentionally NOT threaded into the child UserSpec: ExamplechildConfig
// embeds only BaseUserSpec with no additional fields, so there is no target field to receive it.
// The legacy DeriveDesiredState path wires ChildConfig for template rendering; that path is
// preserved intact until the cutover removes it.
func RenderChildren(cfg ExampleparentConfig, enabled bool) ([]config.ChildSpec, error) {
	childWorkerType := cfg.GetChildWorkerType()
	count := cfg.ChildrenCount

	specs := make([]config.ChildSpec, 0, count)

	for i := range count {
		spec, err := config.NewChildSpec(fmt.Sprintf("child-%d", i), childWorkerType, config.BaseUserSpec{}, enabled)
		if err != nil {
			return nil, fmt.Errorf("exampleparent RenderChildren: child-%d: %w", i, err)
		}

		spec.ChildStartStates = []string{"TryingToStart", "Running"}
		specs = append(specs, spec)
	}

	return specs, nil
}
