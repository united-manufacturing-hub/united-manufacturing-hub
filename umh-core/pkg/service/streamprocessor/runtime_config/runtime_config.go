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

package runtime_config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
)

// BuildRuntimeConfig merges all variables (user + agent + global + internal),
// performs the location merge, derives the `bridged_by` header, and finally
// renders the streamprocessor config and the associated DFC config.
// ─────────────
//
//   - *Spec* has been unmarshalled from YAML **and** already passed through the
//     variable-enrichment step performed by the control loop / manager.
//
//     👉  That means `spec.Variables` **already** contains
//     – user-supplied keys                 (flat)
//     – authoritative `.location` map      (merged from agent)
//     – fleet-wide  `.global`  namespace   (injected by central loop)
//     – runtime-only `.internal` namespace (added by the manager)
//
// Returns both the streamprocessor runtime config and the rendered DFC config
func BuildRuntimeConfig(
	spec streamprocessorserviceconfig.StreamProcessorServiceConfigSpec,
	agentLocation map[string]string,
	globalVars map[string]any,
	nodeName string,
	spName string,
) (streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime, dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error) {
	if reflect.DeepEqual(spec, streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{}) {
		return streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{},
			dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
			fmt.Errorf("nil spec")
	}

	spLocation := spec.Location

	//----------------------------------------------------------------------
	// 1. Merge & normalise *location* map
	//----------------------------------------------------------------------
	loc := map[string]string{}

	// 1a) copy agent levels (authoritative)
	for k, v := range agentLocation {
		loc[k] = v
	}

	// 1b) extend with PC-local additions (never overwrite agent keys)
	for k, v := range spLocation {
		if agentValue, exists := loc[k]; !exists || agentValue == "" {
			loc[k] = v
		}
	}

	// 1c) fill gaps up to the highest defined level with "unknown"
	//
	// Design Decision: Fill Missing Levels vs. Return Error
	// ───────────────────────────────────────────────────────────────────────────
	// When location hierarchy has gaps (e.g., levels 0,3 defined but 1,2 missing),
	// we choose to fill the gaps with "unknown" rather than returning an error.
	//
	// Rationale:
	// • FSM Context: This runs in a reconciliation loop. Returning an error here
	//   would cause endless retries without fixing the underlying config issue.
	// • Error Handling Separation: Critical config validation belongs in the
	//   initial parsing phase, not during runtime config building.
	// • Graceful Degradation: A running system with some "unknown" location levels
	//   provides better UX than a completely broken protocol converter.
	// • Observable Failure: "unknown" values are visible in logs/metrics, making
	//   the configuration gap obvious while maintaining system functionality.
	// • Practical Recovery: Users can fix missing levels and the system will
	//   automatically pick up the corrected configuration on the next reconcile.
	maxLevel := -1
	for k := range loc {
		level, err := strconv.Atoi(k)
		if err == nil && level > maxLevel {
			maxLevel = level
		}
	}
	for i := 0; i <= maxLevel; i++ {
		key := strconv.Itoa(i)
		if _, exists := loc[key]; !exists {
			loc[key] = "unknown"
		}
	}

	// 1d) generate location path (dot-separated string)
	var pathParts []string
	for i := 0; i <= maxLevel; i++ {
		key := strconv.Itoa(i)
		if val, exists := loc[key]; exists {
			pathParts = append(pathParts, val)
		}
	}
	locationPath := strings.Join(pathParts, ".")
	// Strip trailing dots
	locationPath = strings.TrimRight(locationPath, ".")

	//----------------------------------------------------------------------
	// 2. Assemble the **complete** variable bundle
	//----------------------------------------------------------------------
	vb := spec.Variables // start with user bundle (flat)
	if vb.User == nil {
		vb.User = map[string]any{}
	}
	vb.User["location"] = loc // merged map
	vb.User["location_path"] = locationPath

	if len(globalVars) != 0 {
		vb.Global = globalVars
	}

	// Internal namespace
	vb.Internal = map[string]any{
		"id": spName,
	}

	//----------------------------------------------------------------------
	// 3. bridged_by header
	//----------------------------------------------------------------------
	if nodeName == "" {
		nodeName = "unknown"
	}
	vb.Internal["bridged_by"] = config.GenerateBridgedBy(config.ComponentTypeStreamProcessor, nodeName, spName)

	//----------------------------------------------------------------------
	// 4. Render both configs
	//----------------------------------------------------------------------
	scope := vb.Flatten()

	// Render the streamprocessor runtime config
	spRuntime, err := renderConfig(spec, scope)
	if err != nil {
		return streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{},
			dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
			fmt.Errorf("failed to render streamprocessor config: %w", err)
	}

	// Render the DFC config
	dfcTemplate := spec.GetDFCServiceConfig(spName)
	dfcRuntime, err := config.RenderTemplate(dfcTemplate, scope)
	if err != nil {
		return streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{},
			dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
			fmt.Errorf("failed to render DFC config: %w", err)
	}

	return spRuntime, dfcRuntime, nil
}

// renderConfig turns the **author-facing** specification (*Spec*) into the
// **fully rendered** runtime configuration that the FSM compares against the
// live system.
//
// This function performs template rendering and type conversion from template
// types (which use strings for YAML compatibility) to runtime types (which use
// proper Go types for type safety and validation).
//
// Preconditions
// ─────────────
//
//   - *Spec* has been unmarshalled from YAML **and** already passed through the
//     variable-enrichment step performed by the control loop / manager.
//
//     👉  That means `spec.Variables` **already** contains
//     – user-supplied keys                 (flat)
//     – authoritative `.location` map      (merged from agent)
//     – fleet-wide  `.global`  namespace   (injected by central loop)
//     – runtime-only `.internal` namespace (added by the manager)
//
//     renderConfig does **not** add or override any variables.
//     If a key is missing, template rendering will fail.
//
// Workflow
// ──────────────────────────────────────────────────────────────────────────────
// 1. Retrieve the three subordinate blueprints from *Spec*
//
//   - Connection
//
//   - read-DFC   (with UNS **output** enforced via GetDFCServiceConfig)
//
//   - write-DFC  (with UNS **input**  enforced via GetDFCWriteServiceConfig)
//
//     2. Render each blueprint with the already-enriched variable scope using
//     `config.RenderTemplate`. After this step **no** `{{ … }}` directives
//     remain.
//
// 3. Convert template types to runtime types:
//
//   - Connection: Convert rendered string port (e.g., "443" or "{{ .PORT }}") to uint16
//
//   - DFC configs: Already in final form after template rendering
//
//     4. Assemble the concrete pieces into a
//     `ProtocolConverterServiceConfigRuntime` value and return it.
//
// Template vs Runtime Types
// ─────────────────────────
// - Template types use strings for all templatable fields to enable YAML templating
// - Runtime types use proper Go types (uint16 for ports) for type safety
//
// Notes
// ─────
//   - The function is pure: it performs no side-effects and never mutates *Spec*.
//   - Passing a nil *Spec* results in an explicit error; an empty runtime
//     struct is **never** returned.
//
// The returned object is ready for diffing or to be handed straight to the
// Protocol-Converter FSM.
func renderConfig(
	spec streamprocessorserviceconfig.StreamProcessorServiceConfigSpec,
	scope map[string]any,
) (
	streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime,
	error,
) {
	if reflect.DeepEqual(spec, streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{}) {
		return streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{}, fmt.Errorf("streamprocessor config is nil")
	}

	// ─── Render the streamprocessor template with variable substitution ─────────────────────────────
	// This converts template strings like "{{ .location_path }}" to actual values
	rendered, err := config.RenderTemplate(spec.Config, scope)
	if err != nil {
		return streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{}, fmt.Errorf("failed to render streamprocessor template: %w", err)
	}

	// ─── Convert template to runtime config ─────────────────────────────
	// All template variables have been resolved, now convert to runtime format
	runtime := streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime(rendered)

	return runtime, nil
}
