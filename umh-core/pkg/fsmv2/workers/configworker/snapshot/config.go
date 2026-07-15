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

// Package snapshot holds the config worker's config and status types. It lives
// in its own package so the worker package and the state package both depend on
// it without a cycle (the worker package blank-imports the state package).
package snapshot

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ConfigworkerConfig holds the user-provided configuration for the config worker.
// Embeds BaseUserSpec so the framework's DeriveDesiredState can read the YAML
// "state" field (defaults to running).
type ConfigworkerConfig struct {
	config.BaseUserSpec `yaml:",inline"`
}

// ConfigworkerStatus holds the runtime observation data for the config worker.
type ConfigworkerStatus struct {
}
