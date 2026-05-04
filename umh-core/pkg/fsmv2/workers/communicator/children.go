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

package communicator

import (
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// RenderChildren is the parent's children-set emitter for the communicator
// worker. It is a pure function of the typed snapshot: same input yields the
// same ChildSpec values, including identical ChildSpec.Hash output across
// repeated calls (idempotency property exercised by P1.8 architecture
// test #7).
//
// The communicator parent currently manages a single transport child. The
// transport child runs whenever the parent is in Syncing or Recovering. Per
// §4-C LOCKED, Enabled MUST be set explicitly to true; the F4⊕G1 trap
// detector in P1.8 architecture test #13 (registry walk, layer 2) catches
// forgotten-Enabled in renderChildren bodies.
//
// State.Next emits this set via NextResult.Children (wired in P2.2 and made
// authoritative for the supervisor in P2.4); the legacy DDS-derived path was
// retired in P2.5.
func RenderChildren(snap fsmv2.WorkerSnapshot[CommunicatorConfig, CommunicatorStatus]) []config.ChildSpec {
	return []config.ChildSpec{{
		Name:             "transport",
		WorkerType:       "transport",
		UserSpec:         snapshotUserSpec(snap),
		ChildStartStates: []string{"Syncing", "Recovering"},
		Enabled:          true,
	}}
}

// snapshotUserSpec builds the transport child's UserSpec from the communicator's
// parsed config. Returns zero-value UserSpec when RelayURL is empty (nil-spec
// startup path before the first DeriveDesiredState call).
func snapshotUserSpec(snap fsmv2.WorkerSnapshot[CommunicatorConfig, CommunicatorStatus]) config.UserSpec {
	cfg := snap.Desired.Config
	if cfg.RelayURL == "" {
		return config.UserSpec{}
	}

	type childCfg struct {
		RelayURL     string `yaml:"relayURL"`
		InstanceUUID string `yaml:"instanceUUID"`
		AuthToken    string `yaml:"authToken"`
	}

	cc := childCfg{
		RelayURL:     cfg.RelayURL,
		InstanceUUID: cfg.InstanceUUID,
		AuthToken:    cfg.AuthToken,
	}

	yamlBytes, err := yaml.Marshal(cc)
	if err != nil {
		return config.UserSpec{}
	}

	return config.UserSpec{Config: string(yamlBytes)}
}
