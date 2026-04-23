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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
)

// defaultChildConfig is the fallback YAML used when ExampleparentConfig.ChildConfig
// is empty.
const defaultChildConfig = `address: {{ .IP }}:{{ .PORT }}
device: {{ .DEVICE_ID }}`

// RenderChildren is the parent's children-set emitter for the example parent
// worker. Pure function of the typed WorkerSnapshot: same input yields the
// same ChildSpec values (and ChildSpec.Hash output) across repeated calls
// (idempotency property exercised by P1.8 architecture test #7).
//
// State.Next calls this from each transition; the supervisor consumes the
// resulting children set via NextResult.Children (PR2 P2.4 discriminator).
//
// Per §4-C LOCKED, Enabled MUST be set explicitly to true; the F4⊕G1 trap
// detector in P1.8 architecture test #13 (registry walk, layer 2) catches
// forgotten-Enabled in renderChildren bodies.
//
// Reads spec inputs from snap.Desired.Config (the typed ExampleparentConfig)
// and the parent's user-namespace variables from snap.Desired.OriginalUserSpec
// where available; child-specific DEVICE_ID is injected here.
func RenderChildren(snap fsmv2.WorkerSnapshot[snapshot.ExampleparentConfig, snapshot.ExampleparentStatus]) []config.ChildSpec {
	cfg := snap.Desired.Config
	if cfg.ChildrenCount == 0 {
		return []config.ChildSpec{}
	}

	childrenSpecs := make([]config.ChildSpec, cfg.ChildrenCount)
	childWorkerType := cfg.GetChildWorkerType()

	childConfig := cfg.ChildConfig
	if childConfig == "" {
		childConfig = defaultChildConfig
	}

	for i := range cfg.ChildrenCount {
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
