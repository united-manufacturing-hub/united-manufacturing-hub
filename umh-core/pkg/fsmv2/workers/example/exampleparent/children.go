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

// defaultChildConfig is the fallback YAML used when ExampleparentConfig.ChildConfig
// is empty.
const defaultChildConfig = `address: {{ .IP }}:{{ .PORT }}
device: {{ .DEVICE_ID }}`

// State.Next emits children via snapshot.RenderChildren(snap); this helper
// is the parent-spec variant called from DeriveDesiredState to populate
// ChildrenSpecs.
func RenderChildren(spec *ExampleparentConfig) []config.ChildSpec {
	if spec == nil || spec.ChildrenCount == 0 {
		return []config.ChildSpec{}
	}

	childrenSpecs := make([]config.ChildSpec, spec.ChildrenCount)
	childWorkerType := spec.GetChildWorkerType()

	childConfig := spec.ChildConfig
	if childConfig == "" {
		childConfig = defaultChildConfig
	}

	for i := range spec.ChildrenCount {
		childVariables := config.VariableBundle{
			User: map[string]any{
				"DEVICE_ID": fmt.Sprintf("device-%d", i),
			},
		}

		childrenSpecs[i] = config.NewChildSpec(
			fmt.Sprintf("child-%d", i),
			childWorkerType,
			config.UserSpec{
				Config:    childConfig,
				Variables: childVariables,
			},
		)
	}

	return childrenSpecs
}
