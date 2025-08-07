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

package config

import (
	"context"
	"fmt"
	"reflect"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"gopkg.in/yaml.v3"
)

// convertYamlToSpec processes protocol converter configs to resolve templateRef fields
// This translates between the "unrendered" config (with templateRef) and "rendered" config (with actual template content)
func convertYamlToSpec(config FullConfig, ctx context.Context) (FullConfig, error) {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return FullConfig{}, ctx.Err()
	default:
	}

	// Create a copy to avoid mutating the original
	processedConfig := config.Clone()

	// Build a map of available protocol converter templates for quick lookup
	protocolConverterTemplateMap := make(map[string]protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate)

	// Process protocol converter templates from the enforced structure
	for templateName, templateContent := range processedConfig.Templates.ProtocolConverter {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return FullConfig{}, ctx.Err()
		default:
		}

		// Convert the template content to the proper structure
		templateBytes, err := yaml.Marshal(templateContent)
		if err != nil {
			return FullConfig{}, fmt.Errorf("failed to marshal protocol converter template %s: %w", templateName, err)
		}

		var template protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate
		if err := yaml.Unmarshal(templateBytes, &template); err != nil {
			return FullConfig{}, fmt.Errorf("failed to unmarshal protocol converter template %s: %w", templateName, err)
		}

		protocolConverterTemplateMap[templateName] = template
	}

	// Build a map of available stream processor templates for quick lookup
	streamProcessorTemplateMap := make(map[string]streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate)

	// Process stream processor templates from the enforced structure
	for templateName, templateContent := range processedConfig.Templates.StreamProcessor {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return FullConfig{}, ctx.Err()
		default:
		}

		// Convert the template content to the proper structure
		templateBytes, err := yaml.Marshal(templateContent)
		if err != nil {
			return FullConfig{}, fmt.Errorf("failed to marshal stream processor template %s: %w", templateName, err)
		}

		var template streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate
		if err := yaml.Unmarshal(templateBytes, &template); err != nil {
			return FullConfig{}, fmt.Errorf("failed to unmarshal stream processor template %s: %w", templateName, err)
		}

		streamProcessorTemplateMap[templateName] = template
	}

	// Process each protocol converter to resolve templateRef
	for i, pc := range processedConfig.ProtocolConverter {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return FullConfig{}, ctx.Err()
		default:
		}

		// Only resolve templateRef if it's not empty/null and there's no inline config
		if pc.ProtocolConverterServiceConfig.TemplateRef != "" {
			// Resolve the template reference
			templateName := pc.ProtocolConverterServiceConfig.TemplateRef
			template, exists := protocolConverterTemplateMap[templateName]
			if !exists {
				return FullConfig{}, fmt.Errorf("protocol converter template reference %q not found for protocol converter %s", templateName, pc.Name)
			}

			// Create a new spec with the resolved template
			resolvedSpec := pc.ProtocolConverterServiceConfig
			resolvedSpec.Config = template

			// Update the config
			processedConfig.ProtocolConverter[i].ProtocolConverterServiceConfig = resolvedSpec
		}
		// If templateRef is empty/null, use the inline config as-is
	}

	// Process each stream processor to resolve templateRef
	for i, sp := range processedConfig.StreamProcessor {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return FullConfig{}, ctx.Err()
		default:
		}

		// Only resolve templateRef if it's not empty/null and there's no inline config
		if sp.StreamProcessorServiceConfig.TemplateRef != "" {
			// Resolve the template reference
			templateName := sp.StreamProcessorServiceConfig.TemplateRef
			template, exists := streamProcessorTemplateMap[templateName]
			if !exists {
				return FullConfig{}, fmt.Errorf("stream processor template reference %q not found for stream processor %s", templateName, sp.Name)
			}

			// Create a new spec with the resolved template
			resolvedSpec := sp.StreamProcessorServiceConfig
			resolvedSpec.Config = template

			// Update the config
			processedConfig.StreamProcessor[i].StreamProcessorServiceConfig = resolvedSpec
		}
		// If templateRef is empty/null, use the inline config as-is
	}

	// remove the templates from the config
	processedConfig.Templates = TemplatesConfig{}

	return processedConfig, nil
}

// Our in-memory representation (**Spec FullConfig**) keeps every
// *instance* fully materialised and **does not** carry the `templates:` map
// that appears in the YAML-on-disk file.
//
//   - Stand-alone  (TemplateRef == "") →   keep Config inline.
//   - Root        (TemplateRef == Name) → copy Config into
//     clone.Templates.ProtocolConverter[Name] and clear the
//     instance.Config so we do not duplicate YAML.
//   - Child       (TemplateRef != "" && TemplateRef != Name) →
//     only keep the metadata; Config is cleared.
//     At the end we verify the referenced root exists.
//
// Invariants enforced here:
//  1. No two roots may define different Config under the same name.
//  2. Every child must point to an existing root.
//  3. Function must not mutate the *input*; it always works on a deep copy.
//
// Data-flow
// ─────────
//
//	YAML (templates + refs) ──› parseYAMLToSpec() ──› Spec (instances only)
//
//	Spec (instances) ──› convertSpecToYaml() ──› YAML (templates + refs)
//
// In short: **Spec is expanded, YAML is compressed – with an escape hatch for
// stand-alone converters.**
func convertSpecToYaml(spec FullConfig, ctx context.Context) (FullConfig, error) {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return FullConfig{}, ctx.Err()
	default:
	}

	//------------------------------------
	// 1) start with a deep copy
	//------------------------------------
	clone := spec.Clone()

	//------------------------------------
	// 2) helper structures
	//------------------------------------

	// Protocol Converter template maps
	// tplMap collects every **root** protocol-converter we encounter.
	// A "root" is the first, fully-detailed instance whose TemplateRef
	// equals its own Name.  We stash those complete Config blocks here
	// so that, after the loop, we can write them once into
	//   clone.Templates.protocolConverter[<root-name>]
	protocolConverterTplMap := make(map[string]protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate) // roots collected here

	// pendingRefs
	// -----------
	// As we meet **child** instances (TemplateRef points to some other name) we
	// jot down the referenced template name in pendingRefs.
	// After the loop finishes we make sure every entry in pendingRefs also exists
	// in tplMap.  If something is missing we've discovered an "orphan" child that
	// points to a template which is not present – in that case we return an error
	// instead of writing an inconsistent YAML file.
	protocolConverterPendingRefs := make(map[string]struct{}) // child refs to be validated

	// Stream Processor template maps
	streamProcessorTplMap := make(map[string]streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate) // roots collected here
	streamProcessorPendingRefs := make(map[string]struct{})                                                     // child refs to be validated

	//------------------------------------
	// 3) walk every PC once
	//------------------------------------
	for i, pc := range clone.ProtocolConverter {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return FullConfig{}, ctx.Err()
		default:
		}

		tr := pc.ProtocolConverterServiceConfig.TemplateRef

		// ─────────────────────────────
		// 3a) Stand-alone (no template)
		// ─────────────────────────────
		if tr == "" {
			// keep Config as-is → nothing else to do
			continue
		}

		// ─────────────────────────────
		// 3b) Root  (golden instance)
		// ─────────────────────────────
		if tr == pc.Name {
			if prev, dup := protocolConverterTplMap[tr]; dup {
				// second root with same name ⇒ must be byte-identical
				if !reflect.DeepEqual(prev, pc.ProtocolConverterServiceConfig.Config) {
					return FullConfig{}, fmt.Errorf(
						"duplicate protocol converter root %q with different Config blocks", tr)
				}
			} else {
				protocolConverterTplMap[tr] = pc.ProtocolConverterServiceConfig.Config
			}
		} else {
			// ─────────────────────────
			// 3c) Child (inherits root)
			// ─────────────────────────
			// pendingRefs is a _set_ of template-names that were referenced by CHILD
			// instances.  We use
			//
			//     map[string]struct{}
			//
			// instead of
			//
			//     map[string]bool
			//
			// because the empty struct occupies **zero bytes**.
			// We only care whether a key exists, not about any value it might hold, so
			// storing an empty struct is the most memory-efficient and idiomatic way to
			// represent a "set" in Go.  At the end of the loop we simply iterate over the
			// keys to verify that every referenced template has a corresponding root.
			protocolConverterPendingRefs[tr] = struct{}{}
		}

		// Strip Config from every templated instance (root or child) ─ the full
		// definition will live once in the templates section, so we avoid
		// duplicating it inside each instance.
		pc.ProtocolConverterServiceConfig.Config =
			protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{}
		// remove the location and location_path from the user variables
		delete(pc.ProtocolConverterServiceConfig.Variables.User, "location")
		delete(pc.ProtocolConverterServiceConfig.Variables.User, "location_path")
		clone.ProtocolConverter[i] = pc
	}

	//------------------------------------
	// 4) walk every SP once
	//------------------------------------
	for i, sp := range clone.StreamProcessor {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return FullConfig{}, ctx.Err()
		default:
		}

		tr := sp.StreamProcessorServiceConfig.TemplateRef

		// ─────────────────────────────
		// 4a) Stand-alone (no template)
		// ─────────────────────────────
		if tr == "" {
			// keep Config as-is → nothing else to do
			continue
		}

		// ─────────────────────────────
		// 4b) Root  (golden instance)
		// ─────────────────────────────
		if tr == sp.Name {
			if prev, dup := streamProcessorTplMap[tr]; dup {
				// second root with same name ⇒ must be byte-identical
				if !reflect.DeepEqual(prev, sp.StreamProcessorServiceConfig.Config) {
					return FullConfig{}, fmt.Errorf(
						"duplicate stream processor root %q with different Config blocks", tr)
				}
			} else {
				streamProcessorTplMap[tr] = sp.StreamProcessorServiceConfig.Config
			}
		} else {
			// ─────────────────────────
			// 4c) Child (inherits root)
			// ─────────────────────────
			streamProcessorPendingRefs[tr] = struct{}{}
		}

		// Strip Config from every templated instance (root or child) ─ the full
		// definition will live once in the templates section, so we avoid
		// duplicating it inside each instance.
		sp.StreamProcessorServiceConfig.Config =
			streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{}
		// remove the location and location_path from the user variables
		delete(sp.StreamProcessorServiceConfig.Variables.User, "location")
		delete(sp.StreamProcessorServiceConfig.Variables.User, "location_path")
		clone.StreamProcessor[i] = sp
	}

	//------------------------------------
	// 5) orphan-ref validation and cleanup (children → root)
	//------------------------------------
	// If a reference is missing it means a child points to a
	// non-existent template. For protocol converters, this is an error.
	// For stream processors, we convert them to inline configs to handle
	// the external template pattern.
	for ref := range protocolConverterPendingRefs {
		if _, ok := protocolConverterTplMap[ref]; !ok {
			return FullConfig{}, fmt.Errorf(
				"protocol converter references unknown template %q", ref)
		}
	}

	// For stream processors with orphaned references, convert to inline configs
	orphanedStreamProcessorRefs := make(map[string]struct{})
	for ref := range streamProcessorPendingRefs {
		if _, ok := streamProcessorTplMap[ref]; !ok {
			orphanedStreamProcessorRefs[ref] = struct{}{}
		}
	}

	// Convert orphaned stream processor references to inline configs
	if len(orphanedStreamProcessorRefs) > 0 {
		for i, sp := range clone.StreamProcessor {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return FullConfig{}, ctx.Err()
			default:
			}

			if sp.StreamProcessorServiceConfig.TemplateRef != "" {
				if _, isOrphaned := orphanedStreamProcessorRefs[sp.StreamProcessorServiceConfig.TemplateRef]; isOrphaned {
					// Clear the template reference since it points to an external template
					// The config should already be expanded inline from convertYamlToSpec
					sp.StreamProcessorServiceConfig.TemplateRef = ""
					clone.StreamProcessor[i] = sp
				}
			}
		}
	}

	//------------------------------------
	// 6) attach template maps (only if we have roots)
	//------------------------------------
	if len(protocolConverterTplMap) > 0 {
		if clone.Templates.ProtocolConverter == nil {
			clone.Templates.ProtocolConverter = make(map[string]interface{})
		}
		for name, tpl := range protocolConverterTplMap {
			clone.Templates.ProtocolConverter[name] = tpl
		}
	}

	if len(streamProcessorTplMap) > 0 {
		if clone.Templates.StreamProcessor == nil {
			clone.Templates.StreamProcessor = make(map[string]interface{})
		}
		for name, tpl := range streamProcessorTplMap {
			clone.Templates.StreamProcessor[name] = tpl
		}
	}

	//------------------------------------
	// 7) done –  clone now has YAML layout
	//------------------------------------
	return clone, nil
}
