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

// defaultChildConfig is the fallback YAML used when ParentUserSpec.ChildConfig
// is empty. Kept as a package-level constant so RenderChildren and the legacy
// DeriveDesiredState body share a single source of truth for the default.
const defaultChildConfig = `address: {{ .IP }}:{{ .PORT }}
device: {{ .DEVICE_ID }}`

// RenderChildren is the parent's children-set emitter for the example parent
// worker. Pure function of the parsed ParentUserSpec: same input yields the
// same ChildSpec values (and ChildSpec.Hash output) across repeated calls
// (idempotency property exercised by P1.8 architecture test #7).
//
// Per §4-C LOCKED, Enabled MUST be set explicitly to true; the F4⊕G1 trap
// detector in P1.8 architecture test #13 (registry walk, layer 2) catches
// forgotten-Enabled in renderChildren bodies.
//
// State.Next will adopt this emitter when P2.2 wires renderChildren into the
// state-machine return path; until then DeriveDesiredState calls this helper
// to populate ChildrenSpecs.
func RenderChildren(spec *ParentUserSpec) []config.ChildSpec {
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

		childrenSpecs[i] = config.ChildSpec{
			Name:       fmt.Sprintf("child-%d", i),
			WorkerType: childWorkerType,
			UserSpec: config.UserSpec{
				Config:    childConfig,
				Variables: childVariables,
			},
			ChildStartStates: []string{"TryingToStart", "Running"},
			Enabled:          true,
		}
	}

	return childrenSpecs
}
