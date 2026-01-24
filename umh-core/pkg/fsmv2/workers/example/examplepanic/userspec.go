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

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExamplepanicUserSpec is the typed configuration for the panic worker.
type ExamplepanicUserSpec struct {
	config.BaseUserSpec // Provides State field with GetState() defaulting to "running"
	ShouldRun           bool `yaml:"should_run"`
	ShouldPanic         bool `yaml:"should_panic"`
}
