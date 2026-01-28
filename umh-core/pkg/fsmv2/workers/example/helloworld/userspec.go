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

package hello_world

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// HelloworldUserSpec defines the user configuration for the helloworld worker.
// This is parsed from the UserSpec.Config YAML/JSON string in scenarios.
//
// Example YAML config:
//
//	config: |
//	  state: running
//	  message: "Hello, {{ .Name }}!"
type HelloworldUserSpec struct {
	config.BaseUserSpec // Provides State field with GetState() defaulting to "running"

	// Message is the optional greeting message (can use template variables)
	Message string `json:"message" yaml:"message"`
}
