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

package models

import "time"

// FeatureUsage captures which feature flags are enabled on this instance.
// Populated once at startup from env vars and YAML config. Immutable after construction.
type FeatureUsage struct {
	ConfigBackupEnabledSince *time.Time `json:"configBackupEnabledSince,omitempty"`
	ConfigBackupEnabled      bool       `json:"configBackupEnabled"`
	FSMv2Transport           bool       `json:"fsmv2Transport"`
	FSMv2MemoryCleanup       bool       `json:"fsmv2MemoryCleanup"`
	FSMv2ProtocolConverter   bool       `json:"fsmv2ProtocolConverter"`
	ResourceLimitBlocking    bool       `json:"resourceLimitBlocking"`
}
