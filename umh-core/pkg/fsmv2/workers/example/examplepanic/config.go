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

package example_panic

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"

// ExamplepanicConfig is the typed configuration for the panic worker.
type ExamplepanicConfig struct {
	config.BaseUserSpec
	ShouldRun   bool `json:"should_run"   yaml:"should_run"`
	ShouldPanic bool `json:"should_panic" yaml:"should_panic"`
}

// ExamplepanicStatus is the observed status for the panic worker.
type ExamplepanicStatus struct {
	ConnectionHealth string `json:"connection_health"`
}
