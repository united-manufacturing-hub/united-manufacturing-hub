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

package example_slow

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExampleslowUserSpec defines the typed configuration for the slow worker.
// This is parsed from the UserSpec.Config YAML/JSON string.
//
// Note: Do NOT add ShouldRun or similar lifecycle fields here.
// Lifecycle is controlled by config.BaseUserSpec.State ("running"/"stopped")
// and BaseDesiredState.ShutdownRequested.
type ExampleslowUserSpec struct {
	config.BaseUserSpec // Provides State field with GetState() defaulting to "running"
	DelaySeconds        int `yaml:"delaySeconds"`
}
