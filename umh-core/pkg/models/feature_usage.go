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

// FeatureUsage reports which feature flags are active on this instance and
// tracks per-feature usage metrics.
//
// Bool fields ending in Enabled are auto-mapped to feature_{snake_case} PostHog
// properties by the MC frontend. To add a new flag: add a bool field ending in
// Enabled here, set it in cmd/main.go, and add the field to the TS interface
// in statusMessage.svelte.ts.
//
// Non-bool usage metrics (like ConfigBackupCount) are mapped manually with a
// usage_ prefix in instanceTelemetry.ts.
type FeatureUsage struct {
	// ConfigBackupCount tracks the number of config backups created since startup.
	ConfigBackupCount int `json:"configBackupCount"`
	// ConfigBackupEnabled reports whether ENABLE_CONFIG_BACKUP is set.
	ConfigBackupEnabled bool `json:"configBackupEnabled"`
	// FSMv2TransportEnabled reports whether USE_FSMV2_TRANSPORT is set.
	FSMv2TransportEnabled bool `json:"fsmv2TransportEnabled"`
	// FSMv2MemoryCleanupEnabled reports whether USE_FSMV2_MEMORY_CLEANUP is set.
	FSMv2MemoryCleanupEnabled bool `json:"fsmv2MemoryCleanupEnabled"`
	// FSMv2ProtocolConverterEnabled reports whether USE_FSMV2_PROTOCOL_CONVERTER is set.
	FSMv2ProtocolConverterEnabled bool `json:"fsmv2ProtocolConverterEnabled"`
	// ResourceLimitBlockingEnabled reports whether agent.enableResourceLimitBlocking is set.
	ResourceLimitBlockingEnabled bool `json:"resourceLimitBlockingEnabled"`
}
