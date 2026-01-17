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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ParentUserSpec defines the typed configuration for the parent worker.
// This is parsed from the UserSpec.Config YAML/JSON string.
type ParentUserSpec struct {
	config.BaseUserSpec // Provides State field with GetState() defaulting to "running"
	ChildrenCount       int    `json:"children_count"    yaml:"children_count"`
	ChildWorkerType     string `json:"child_worker_type" yaml:"child_worker_type"` // Optional: defaults to "examplechild"
	ChildConfig         string `json:"child_config"      yaml:"child_config"`      // Optional: config to pass to children
}

// GetChildWorkerType returns the configured child worker type, defaulting to "examplechild".
func (s *ParentUserSpec) GetChildWorkerType() string {
	if s.ChildWorkerType == "" {
		return "examplechild"
	}

	return s.ChildWorkerType
}
