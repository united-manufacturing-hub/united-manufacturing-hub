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
// Each child receives cfg.ChildConfig verbatim as UserSpec.Config and a
// per-child DEVICE_ID variable (device-0, device-1, ...).
//
// When cfg.ChildConfig is empty, the child's Config is also empty.
// No fallback template is injected — ExamplechildConfig has no address or
// device fields that would consume one.
//
// enabled=false here is unused: exampleparent's stop-states pass empty
// slices directly because the children are stateless. See workers/transport
// for the variant where stopped children stay resident.
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
			Enabled: enabled,
		})
	}

	return specs, nil
}
