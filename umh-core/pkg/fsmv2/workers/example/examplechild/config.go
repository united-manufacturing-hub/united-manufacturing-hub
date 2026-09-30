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

package example_child

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"

// ExamplechildConfig is the typed configuration for the child worker.
// The yaml tags are read when DeriveDesiredState parses the rendered
// template. The json tags are read when the supervisor persists the
// derived desired state as a JSON document.
type ExamplechildConfig struct {
	config.BaseUserSpec
	Address string `yaml:"address" json:"address"`
	Device  string `yaml:"device" json:"device"`
}

// ExamplechildStatus is the observed status for the child worker.
// Observation flattens these fields to the top level of its JSON output,
// so the json tags set the persisted key spelling.
type ExamplechildStatus struct {
	ConnectionHealth string `json:"connection_health"`
	Address          string `json:"address"`
	Device           string `json:"device"`
}
