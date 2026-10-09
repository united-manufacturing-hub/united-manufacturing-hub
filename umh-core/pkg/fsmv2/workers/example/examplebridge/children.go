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

package examplebridge

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// flowNames are the flow template fields the bridge hands down to a flow child.
var flowNames = []string{"read", "write"}

// RenderFlows returns one flow child spec per non-empty flow template. Each
// child carries its template as the flow key of a YAML mapping, so quotes and
// colons in the template survive the child's config load unchanged, and the
// bridge's names as a deep copy.
func RenderFlows(cfg ExamplebridgeConfig) ([]config.ChildSpec, error) {
	specs := make([]config.ChildSpec, 0, len(flowNames))

	for _, flow := range flowNames {
		template := cfg.Template(flow)
		if template == "" {
			continue
		}

		encoded, err := yaml.Marshal(map[string]config.Template{"flow": template})
		if err != nil {
			return nil, fmt.Errorf("failed to build the %s flow config: %w", flow, err)
		}

		specs = append(specs, config.ChildSpec{
			Name:       flow,
			WorkerType: "exampleflow",
			UserSpec: config.UserSpec{
				Config: string(encoded),
				Names:  config.DeepCloneMap(cfg.Names),
			},
			Enabled: true,
		})
	}

	return specs, nil
}
