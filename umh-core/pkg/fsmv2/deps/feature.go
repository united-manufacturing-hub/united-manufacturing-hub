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
// Sentry uses this tag to group issues and route alerts to the right owner.
//
// # Choosing a feature tag
//
// Use [FeatureForWorker] when a worker interface method executes in the
// failing path. Worker interface methods include Execute, Name (on actions),
// CollectObservedState, DeriveDesiredState, and Next (on states).
// The worker team investigates these errors.
//
// Use [FeatureFSMv2] when only supervisor infrastructure executes. No worker
// interface method runs. A framework engineer investigates these errors.
//
// Ask: "does a worker interface method execute before this error?" If yes,
// use [FeatureForWorker]. If only supervisor machinery runs (storage loading,
// goroutine lifecycle, circuit breakers, signal processing), use [FeatureFSMv2].
//
// # Examples
//
// FeatureForWorker (worker interface method executes):
//   - action_failed: the action's Execute() returns an error
//   - authentication_failed: the auth action's Execute() runs
//   - collector_observation_failed: CollectObservedState returns an error
//
// FeatureFSMv2 (supervisor infrastructure only):
//   - snapshot_load_failed: CSE deserialization, no worker method involved
//   - collector_start_failed: supervisor starts a goroutine, no worker method runs yet
//   - tick_panic: panic recovery in the supervisor tick loop
//   - circuit_breaker_opened: supervisor health tracking
//
// # Gray areas
//
// Collector lifecycle operations (start, restart, stop) are supervisor
// infrastructure. Some collector restart events in reconciliation.go use
// [FeatureForWorker] because they are triggered by stale observations from
// a specific worker, even though the restart itself is supervisor machinery.
// When in doubt, route to the worker team — they have the most context to
// investigate.
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

	// FeatureDisableReadFlows covers errors from the read-flow and write-flow
	// features (activating/deactivating individual DFCs on protocol converters).
	// Both features share this tag because they are implemented together and
	// route to the same owner.
	FeatureDisableReadFlows Feature = "disable_read_flows"
)

// FeatureForWorker returns the Feature for a specific worker type.
// The returned feature tag matches the worker type string (e.g., "pull",
// "push", "certfetcher"), so each worker's Sentry events route separately.
//
// See the [Feature] type documentation for when to use this vs [FeatureFSMv2].
func FeatureForWorker(workerType string) Feature {
	return Feature(workerType)
}
