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
// The MC frontend auto-maps these fields to PostHog properties by JSON type: every
// bool becomes feature_{snake_case}, every number becomes usage_{snake_case} (see
// buildProperties in instanceTelemetry.ts). To add a metric: add a field here, set
// it in cmd/main.go, and add the field to the TS FeatureUsage interface.
type FeatureUsage struct {
	// ConfigBackupCount tracks the number of config backups created since startup.
	ConfigBackupCount int `json:"configBackupCount"`
	// HistorianBridgeCount reports the number of bridges writing to the historian.
	HistorianBridgeCount int `json:"historianBridgeCount"`
	// ConfigBackupEnabled reports whether ENABLE_CONFIG_BACKUP is set.
	ConfigBackupEnabled bool `json:"configBackupEnabled"`
	// FSMv2TransportEnabled reports whether USE_FSMV2_TRANSPORT is set.
	FSMv2TransportEnabled bool `json:"fsmv2TransportEnabled"`
	// FSMv2MemoryCleanupEnabled reports whether USE_FSMV2_MEMORY_CLEANUP is set.
	FSMv2MemoryCleanupEnabled bool `json:"fsmv2MemoryCleanupEnabled"`
	// FSMv2ProtocolConverterEnabled reports whether USE_FSMV2_PROTOCOL_CONVERTER is set.
	FSMv2ProtocolConverterEnabled bool `json:"fsmv2ProtocolConverterEnabled"`
	// FSMv2CPUEnabled reports whether the fsmv2 CPU path is effectively running,
	// which is not the same as USE_FSMV2_CPU being set. The FSMv2CPUEnabled
	// function below computes it and says why the two differ.
	FSMv2CPUEnabled bool `json:"fsmv2CpuEnabled"`
	// ResourceLimitBlockingEnabled reports the value of agent.enableResourceLimitBlocking in config.yaml (defaults to true).
	ResourceLimitBlockingEnabled bool `json:"resourceLimitBlockingEnabled"`
	// HistorianConfigured reports whether a historian section exists in config.yaml.
	HistorianConfigured bool `json:"historianConfigured"`
}

// FSMv2CPUEnabled reports the effective state of the fsmv2 CPU path: the flag must
// be on and every prerequisite the seam needs must be present (USE_FSMV2_TRANSPORT
// on, API_URL and AUTH_TOKEN set). cmd/main.go calls it with the same values the
// FSMv2 supervisor gate reads.
//
// It deviates from the sibling flags on purpose. They report "env var set", but
// USE_FSMV2_CPU falls back to legacy without its prerequisites, so counting the
// env var alone would report an instance as enabled where GetClient() returns nil
// and no fsmv2 code runs at all.
func FSMv2CPUEnabled(flag, transport, apiURLSet, authTokenSet bool) bool {
	return flag && transport && apiURLSet && authTokenSet
}
