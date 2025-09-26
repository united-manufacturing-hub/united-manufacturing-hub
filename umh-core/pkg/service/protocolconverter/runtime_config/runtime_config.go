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
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// BuildRuntimeConfig merges all variables (user + agent + global + internal),
// performs the location merge, derives the `bridged_by` header, and finally
// renders the three sub-templates.
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
// Workflow
// ──────────────────────────────────────────────────────────────────────────────
// 1. Merge & normalize location maps:
//   - `agentLocation` – authoritative map from agent.location (may be nil)
//   - `pcLocation`    – optional overrides/extension from the PC spec
//   - Fill gaps with "unknown" up to highest defined level
//
// 2. Assemble complete variable bundle:
//   - Start with user bundle (flat)
//   - Add merged location map
//   - Add global variables if present
//   - Add internal namespace with PC ID
//   - Add bridged_by header derived from nodeName and pcName
//
// 3. Render all three sub-templates:
//   - Connection
//   - read-DFC   (with UNS **output** enforced)
//   - write-DFC  (with UNS **input**  enforced)
//
// Notes
// ─────
//   - The function is pure: it performs no side-effects and never mutates *Spec*.
//   - Passing a nil *Spec* results in an explicit error; an empty runtime
//     struct is **never** returned.
//   - After rendering, **no** `{{ … }}` directives remain.
//   - The returned object is ready for diffing or to be handed straight to the
//     Protocol-Converter FSM.
//
// It does NOT belong to the service.
func BuildRuntimeConfig(
	spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec,
	agentLocation map[string]string,
	globalVars map[string]any,
	nodeName string,
	pcName string,
) (protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime, error) {
	if reflect.DeepEqual(spec, protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{}) {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{},
			errors.New("nil spec")
	}

	pcLocation := make(map[string]string, len(spec.Location))

	// Quickly copy spec.Location to pcLocation if it exists
	// For loop is quicker then invoking deepcopy
	if spec.Location != nil {
		for k, v := range spec.Location {
			pcLocation[k] = v
		}
	}

	//----------------------------------------------------------------------
	// 1. Merge & normalise *location* map
	//----------------------------------------------------------------------
	loc := map[string]string{}

	// 1a) copy agent levels (authoritative)
	for k, v := range agentLocation {
		loc[k] = v
	}

	// 1b) extend with PC-local additions (never overwrite agent keys)
	for k, v := range pcLocation {
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
	vb := spec.Variables
	// Deep copy the User map to avoid mutating the original
	if vb.User != nil {
		userCopy := make(map[string]any, len(vb.User)+2) // +2 for location keys
		for k, v := range vb.User {
			userCopy[k] = v
		}

		vb.User = userCopy
	} else {
		vb.User = map[string]any{}
	}

	vb.User["location"] = loc
	vb.User["location_path"] = locationPath

	if len(globalVars) != 0 {
		vb.Global = globalVars
	}

	// Internal namespace
	vb.Internal = map[string]any{
		"id": pcName,
	}

	//----------------------------------------------------------------------
	// 3. bridged_by header
	//----------------------------------------------------------------------
	if nodeName == "" {
		nodeName = "unknown"
	}

	vb.Internal["bridged_by"] = config.GenerateBridgedBy(config.ComponentTypeProtocolConverter, nodeName, pcName)

	//----------------------------------------------------------------------
	// 4. Render all three sub-templates
	//----------------------------------------------------------------------
	scope := vb.Flatten()

	return renderConfig(spec, scope) // unexported helper that enforces UNS
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
	spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec,
	scope map[string]any,
) (
	protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime,
	error,
) {
	if reflect.DeepEqual(spec, protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{}) {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, errors.New("protocolConverter config is nil")
	}

	// ─── Render the three sub-templates ─────────────────────────────
	// Ensure to use GetDFCReadServiceConfig(), etc. to get the uns input/output enforced

	// 1. Render the connection template with variable substitution
	// This converts template strings like "{{ .PORT }}" to actual values
	conn, err := config.RenderTemplate(spec.GetConnectionServiceConfig(), scope)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, err
	}

	// 2. Convert the rendered template to runtime config with proper types
	// This handles the string-to-uint16 port conversion and type safety
	connRuntime, err := connectionserviceconfig.ConvertTemplateToRuntime(conn)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, fmt.Errorf("failed to convert connection template to runtime: %w", err)
	}

	read, err := config.RenderTemplate(spec.GetDFCReadServiceConfig(), scope)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, err
	}

	// the downsampler will be appended only for timeseries data
	read = appendDownsampler(read)

	write, err := config.RenderTemplate(spec.GetDFCWriteServiceConfig(), scope)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, err
	}

	return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{
		ConnectionServiceConfig:             connRuntime,
		DataflowComponentReadServiceConfig:  read,
		DataflowComponentWriteServiceConfig: write,
	}, nil
}

// appendDownsampler appends a downsampler as the last processor if one doesn't already exist.
func appendDownsampler(dfc dataflowcomponentserviceconfig.DataflowComponentServiceConfig) dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	processors := getProcessors(dfc.BenthosConfig.Pipeline)

	processorType := constants.DetermineProcessorType(processors)
	if processorType != constants.TimeseriesProcessor {
		return dfc
	}

	if hasDownsampler(processors) {
		return dfc // User has already configured downsampling
	}

	defaultDownsampler := map[string]any{
		constants.DownsamplerProcessor: map[string]any{}, // Empty config - reads from metadata
	}

	processors = append(processors, defaultDownsampler)

	// update the config
	updatedDFC := dfc
	if updatedDFC.BenthosConfig.Pipeline == nil {
		updatedDFC.BenthosConfig.Pipeline = make(map[string]any)
	}

	updatedDFC.BenthosConfig.Pipeline["processors"] = processors

	return updatedDFC
}

// getProcessors safely extracts the processors array from a pipeline configuration.
func getProcessors(pipeline map[string]any) []any {
	if pipeline == nil {
		return []any{}
	}

	if procs, ok := pipeline["processors"]; ok {
		if procsArray, ok := procs.([]any); ok {
			return procsArray
		}
	}

	return []any{}
}

// hasDownsampler checks if any processor in the pipeline is a downsampler.
func hasDownsampler(processors []any) bool {
	for _, proc := range processors {
		if procMap, ok := proc.(map[string]any); ok {
			if _, hasDownsampler := procMap[constants.DownsamplerProcessor]; hasDownsampler {
				return true
			}
		}
	}

	return false
}
