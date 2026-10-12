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

package exampleflow

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"

// ExampleflowConfig is the typed configuration of the example flow worker.
// Flow is rendered by WorkerBase from UserSpec.Names when Names is set.
type ExampleflowConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	// Flow is the flow template a parent hands down. WorkerBase renders it.
	Flow config.Template `json:"flow" yaml:"flow"`
}

// ExampleflowStatus is the observed status of the example flow worker.
type ExampleflowStatus struct {
	// ID is the worker's ID, for observability.
	ID string `json:"id,omitempty"`
	// Started reports whether the start action ran.
	Started bool `json:"started"`
}
