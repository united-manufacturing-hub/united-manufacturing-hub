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

package deps

// Feature identifies the subsystem for Sentry routing and alerting.
// Using a typed enum prevents typos at compile time.
//
// Worker-specific events use [FeatureForWorker] to derive the feature
// from the worker's type string (e.g., "pull", "push", "certfetcher").
// Static constants below cover non-worker subsystems.
type Feature string

const (
	// FeatureFSMv2 covers the FSMv2 supervisor core: lifecycle, reconciliation,
	// tick panics, circuit breakers, and child management.
	// Worker-owned events (action_failed, collector_observation_failed, etc.) use
	// FeatureForWorker(workerType) instead.
	FeatureFSMv2 Feature = "fsmv2"

	// FeatureExamples covers example workers used for testing and documentation.
	FeatureExamples Feature = "examples"

	// FeatureCSE covers the CSE (Convergent State Engine) storage layer.
	FeatureCSE Feature = "cse"

	// FeatureFSMv1ConfigManager covers the FSMv1 config manager: config loading,
	// writing, backup, and validation.
	FeatureFSMv1ConfigManager Feature = "fsmv1_config_manager"

	// FeatureDisableReadFlows controls the feature about activating and deactivating read flows.
	FeatureDisableReadFlows Feature = "disable_read_flows"
)

// FeatureForWorker returns the Feature for a specific worker type.
// The returned feature tag matches the worker type string (e.g., "pull",
// "push", "certfetcher"), so each worker's Sentry events route separately.
func FeatureForWorker(workerType string) Feature {
	return Feature(workerType)
}
