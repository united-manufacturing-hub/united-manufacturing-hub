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

// # Automatic Downsampler Injection
//
// This package automatically injects a default downsampler processor after any tag_processor
// in the READ dataflow component during configuration normalization. This provides automatic
// data compression for time-series data without requiring manual configuration.
//
// The injection logic:
//   - Only applies to READ dataflow components (not write components)
//   - Only injects if a tag_processor is found in the pipeline
//   - Skips injection if a downsampler already exists anywhere in the pipeline
//   - Injects an empty downsampler configuration: `downsampler: {}`
//   - The downsampler reads its configuration from message metadata set by the tag_processor
//
// Users control downsampler behavior through metadata fields in their tag_processor:
//   - msg.meta.ds_algorithm (deadband, swinging_door)
//   - msg.meta.ds_threshold (compression threshold)
//   - msg.meta.ds_min_time (minimum time between samples)
//   - msg.meta.ds_max_time (maximum time between samples)
//
// This approach provides zero-configuration data compression while allowing fine-grained
// control when needed through the tag_processor's conditions and metadata.
//
// By default, only duplicate values are removed.

package protocolconverterserviceconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

// Normalizer handles the normalization of ProtocolConverter configurations
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for ProtocolConverter
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig applies ProtocolConverter defaults to a structured config
func (n *Normalizer) NormalizeConfig(cfg ProtocolConverterServiceConfigSpec) ProtocolConverterServiceConfigSpec {

	// create a shallow copy
	normalized := cfg

	// We need to first normalize the underlying DFCServiceConfig
	dfcNormalizer := dataflowcomponentserviceconfig.NewNormalizer()
	normalized.Config.DataflowComponentReadServiceConfig = dfcNormalizer.NormalizeConfig(normalized.GetDFCReadServiceConfig())
	normalized.Config.DataflowComponentWriteServiceConfig = dfcNormalizer.NormalizeConfig(normalized.GetDFCWriteServiceConfig())

	// Inject default downsampler for READ components only
	// Write components process data leaving UNS, downsampling should happen before UNS entry
	normalized.Template.DataflowComponentReadServiceConfig = n.injectDefaultDownsampler(
		normalized.Template.DataflowComponentReadServiceConfig,
	)

	// Then we  need to normalize the underlying ConnectionServiceConfig
	connectionNormalizer := connectionserviceconfig.NewNormalizer()
	// If conversion fails, e.g., because the port is not a number, keep the original template config (graceful degradation during normalization)
	if connRuntime, err := connectionserviceconfig.ConvertTemplateToRuntime(normalized.GetConnectionServiceConfig()); err == nil {
		normalized.Config.ConnectionServiceConfig = connectionserviceconfig.ConvertRuntimeToTemplate(connectionNormalizer.NormalizeConfig(connRuntime))
	}

	// Then we need to normalize the variables
	variablesNormalizer := variables.NewNormalizer()
	normalized.Variables = variablesNormalizer.NormalizeConfig(normalized.Variables)

	// no need to normalize the location

	return normalized
}

// injectDefaultDownsampler automatically injects a downsampler processor after the first tag_processor
// in a dataflow component configuration. This provides automatic data compression for IoT time-series data.
//
// The injected downsampler uses an empty configuration ({}) which defaults to:
// - deadband algorithm with no threshold (removes exact duplicates only)
// - no max_time (no forced heartbeat)
// - passthrough late policy
//
// Users can override this behavior through metadata set by the tag_processor:
// - msg.meta.ds_algorithm (deadband, swinging_door)
// - msg.meta.ds_threshold (compression threshold)
// - msg.meta.ds_max_time (maximum time between samples)
// - etc.
//
// Injection logic:
// - Only injects if a tag_processor is found at the top level
// - Skips injection if a downsampler already exists anywhere in the pipeline
// - Injects after the first tag_processor encountered
// - Uses empty config: downsampler reads configuration from message metadata
//
// ## Architectural Decision: Why Normalization vs Generation?
//
// This downsampler injection happens during configuration NORMALIZATION rather than during
// runtime GENERATION for several important architectural reasons:
//
//  1. **Config Completeness Philosophy**: Normalization's job is to transform raw user config
//     into complete, ready-to-use configuration. Adding a downsampler is conceptually similar
//     to adding default buffers, processors arrays, or input/output configurations.
//
//  2. **Transparency & Debuggability**: After normalization, users can inspect the complete
//     configuration and see exactly what will run. The injected downsampler is visible in
//     the normalized config, not hidden runtime behavior.
//
//  3. **Consistency with Existing Patterns**: Other defaults in this codebase happen during
//     normalization (DFC normalization, connection normalization, etc.). This follows the
//     established pattern: normalization completes configs, generation executes them.
//
//  4. **Avoiding Runtime Complexity**: If injection happened during generation, we'd need
//     complex runtime logic that runs repeatedly on every service restart, adding overhead
//     and potential failure points.
//
//  5. **Avoiding Hidden Behavior**: Users can see the injected processor in their normalized
//     config, making debugging and understanding much easier than hidden runtime injection.
//
// 6. **Proper Separation of Concerns**:
//   - Normalization: "What should the config contain?"
//   - Generation: "How do we run this config?"
//     Injecting processors is about config content, not runtime execution.
func (n *Normalizer) injectDefaultDownsampler(dfc dataflowcomponentserviceconfig.DataflowComponentServiceConfig) dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	processors := getProcessorsFromPipeline(dfc.BenthosConfig.Pipeline)

	// Early exits for edge cases
	if len(processors) == 0 {
		return dfc // No processors to work with
	}

	// Check if downsampler already exists anywhere in the pipeline
	if hasDownsampler(processors) {
		return dfc // User has already configured downsampling
	}

	// Find first tag_processor position (only scan top-level processors)
	tagProcessorIndex := findFirstTagProcessorIndex(processors)
	if tagProcessorIndex == -1 {
		return dfc // No tag_processor found - downsampler needs UMH structure
	}

	// Create simple downsampler - configuration comes from tag_processor metadata
	defaultDownsampler := map[string]any{
		"downsampler": map[string]any{}, // Empty config - reads from metadata
	}

	// Insert downsampler after tag_processor
	newProcessors := insertProcessorAfter(processors, tagProcessorIndex, defaultDownsampler)

	// Update the configuration
	updatedDFC := dfc
	if updatedDFC.BenthosConfig.Pipeline == nil {
		updatedDFC.BenthosConfig.Pipeline = make(map[string]any)
	}
	updatedDFC.BenthosConfig.Pipeline["processors"] = newProcessors

	return updatedDFC
}

// getProcessorsFromPipeline safely extracts the processors array from a pipeline configuration
func getProcessorsFromPipeline(pipeline map[string]any) []interface{} {
	if pipeline == nil {
		return []interface{}{}
	}
	if procs, ok := pipeline["processors"]; ok {
		if procsArray, ok := procs.([]interface{}); ok {
			return procsArray
		}
	}
	return []interface{}{}
}

// hasDownsampler checks if any processor in the pipeline is a downsampler
func hasDownsampler(processors []interface{}) bool {
	for _, proc := range processors {
		if procMap, ok := proc.(map[string]interface{}); ok {
			// Check for exact "downsampler" key
			if _, hasDownsampler := procMap["downsampler"]; hasDownsampler {
				return true
			}
		}
	}
	return false
}

// findFirstTagProcessorIndex finds the index of the first tag_processor in the processors array
// Returns -1 if no tag_processor is found
func findFirstTagProcessorIndex(processors []interface{}) int {
	for i, proc := range processors {
		if isTagProcessor(proc) {
			return i
		}
	}
	return -1
}

// isTagProcessor checks if a processor configuration is a tag_processor
func isTagProcessor(proc interface{}) bool {
	if procMap, ok := proc.(map[string]interface{}); ok {
		// Check for exact "tag_processor" key
		if _, hasTagProcessor := procMap["tag_processor"]; hasTagProcessor {
			return true
		}
	}
	return false
}

// insertProcessorAfter inserts a new processor after the specified index in the processors array
func insertProcessorAfter(processors []interface{}, index int, newProcessor map[string]any) []interface{} {
	// Bounds checking to prevent panics
	if index < 0 || index >= len(processors) {
		// Return original slice unchanged for invalid index
		return processors
	}

	// Create new slice with capacity for one more element
	result := make([]interface{}, len(processors)+1)

	// Copy elements before and including the insertion point
	copy(result[:index+1], processors[:index+1])

	// Insert the new processor
	result[index+1] = newProcessor

	// Copy remaining elements
	copy(result[index+2:], processors[index+1:])

	return result
}
