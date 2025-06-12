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
	"bytes"
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"gopkg.in/yaml.v3"
)

// parseConfig unmarshals *data* (a YAML document) into a FullConfig.
//
// The YAML decoder is configured with KnownFields(true) by default so that any
// unknown or misspelled keys cause an immediate error, preventing silent
// misconfiguration. Setting allowUnknownFields to true allows YAML anchors and other
// custom fields to pass validation.
//
// YAML Anchor Handling for Protocol Converters:
// This function intelligently processes YAML anchors/aliases in protocol converter templates
// to prevent anchor expansion while preserving the structure for parsing. When a protocol
// converter uses a template reference via YAML alias (e.g., template: *opcua_http), this
// function:
//
//   1. Detects the alias reference in the protocol converter's template field
//   2. Extracts the anchor name (e.g., "opcua_http" from "*opcua_http")
//   3. Maps the protocol converter name to the anchor name in the returned anchorMap
//   4. Replaces the alias node with an empty template structure to prevent parsing errors
//   5. Preserves the templates section unchanged so anchors remain available
//
// Protocol Converter Anchor Mapping:
//   - Templated protocol converters (using YAML aliases): Will have an entry in anchorMap
//     mapping the converter name to the referenced anchor name
//   - Inline protocol converters (no YAML aliases): Will NOT appear in anchorMap, their
//     template content is parsed normally into the FullConfig structure
//
// Example behavior:
//   Input:  template: *opcua_http  (YAML alias)
//   Result: - template field becomes empty in FullConfig
//           - anchorMap["converter-name"] = "opcua_http"
//
//   Input:  template: { connection: {...} }  (inline template)
//   Result: - template field contains the inline content in FullConfig
//           - No entry added to anchorMap
//
// This approach allows the system to distinguish between templated configurations
// (which require special handling and cannot be edited via UI) and inline configurations
// (which can be modified through standard config management operations).

func parseConfig(data []byte, allowUnknownFields bool) (FullConfig, map[string]string, error) {
	var cfg FullConfig
	anchorMap := make(map[string]string)

	// First, parse with yaml.Node to detect anchors and aliases
	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return FullConfig{}, nil, fmt.Errorf("failed to parse YAML structure: %w", err)
	}

	// Extract anchor mappings and modify the node tree
	if err := extractAndModifyAnchors(&rootNode, anchorMap); err != nil {
		return FullConfig{}, nil, fmt.Errorf("failed to extract anchor mappings: %w", err)
	}

	// Marshal the modified node tree back to bytes
	modifiedData, err := yaml.Marshal(&rootNode)
	if err != nil {
		return FullConfig{}, nil, fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	// Now decode the modified YAML into FullConfig
	dec := yaml.NewDecoder(bytes.NewReader(modifiedData))
	dec.KnownFields(!allowUnknownFields) // Only reject unknown keys if allowUnknownFields is false
	if err := dec.Decode(&cfg); err != nil {
		return FullConfig{}, nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return cfg, anchorMap, nil
}

// extractAndModifyAnchors walks through the YAML node tree to find protocol converter
// template aliases and extracts the anchor names while replacing alias nodes with resolved template content
func extractAndModifyAnchors(node *yaml.Node, anchorMap map[string]string) error {
	if node == nil {
		return nil
	}

	// Get the root document node for template resolution
	var rootNode *yaml.Node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		rootNode = node
		return extractAndModifyAnchorsWithRoot(node.Content[0], anchorMap, rootNode)
	}

	return extractAndModifyAnchorsWithRoot(node, anchorMap, nil)
}

// extractAndModifyAnchorsWithRoot walks through the YAML node tree with access to the root for template resolution
func extractAndModifyAnchorsWithRoot(node *yaml.Node, anchorMap map[string]string, rootNode *yaml.Node) error {
	if node == nil {
		return nil
	}

	// Handle mapping nodes
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Look for protocolConverter section
			if keyNode.Value == "protocolConverter" && valueNode.Kind == yaml.SequenceNode {
				if err := processProtocolConverterSequenceWithRoot(valueNode, anchorMap, rootNode); err != nil {
					return err
				}
			}

			// Recursively process child nodes
			if err := extractAndModifyAnchorsWithRoot(valueNode, anchorMap, rootNode); err != nil {
				return err
			}
		}
	}

	// Handle sequence nodes
	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if err := extractAndModifyAnchorsWithRoot(child, anchorMap, rootNode); err != nil {
				return err
			}
		}
	}

	return nil
}

// processProtocolConverterSequenceWithRoot processes the protocol converter sequence to find
// template aliases and extract anchor names with access to the root for template resolution
func processProtocolConverterSequenceWithRoot(sequenceNode *yaml.Node, anchorMap map[string]string, rootNode *yaml.Node) error {
	for _, converterNode := range sequenceNode.Content {
		if converterNode.Kind != yaml.MappingNode {
			continue
		}

		var protocolConverterName string

		// Find the protocol converter name and process its service config
		for i := 0; i < len(converterNode.Content); i += 2 {
			keyNode := converterNode.Content[i]
			valueNode := converterNode.Content[i+1]

			if keyNode.Value == "name" {
				protocolConverterName = valueNode.Value
			} else if keyNode.Value == "protocolConverterServiceConfig" && valueNode.Kind == yaml.MappingNode {
				if err := processServiceConfigWithRoot(valueNode, protocolConverterName, anchorMap, rootNode); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// processServiceConfigWithRoot processes the protocol converter service config to find template aliases
// with access to the root for template resolution
func processServiceConfigWithRoot(serviceConfigNode *yaml.Node, protocolConverterName string, anchorMap map[string]string, rootNode *yaml.Node) error {
	for i := 0; i < len(serviceConfigNode.Content); i += 2 {
		keyNode := serviceConfigNode.Content[i]
		valueNode := serviceConfigNode.Content[i+1]

		if keyNode.Value == "template" {
			// Check if this is an alias node
			if valueNode.Kind == yaml.AliasNode {
				// Extract the anchor name (remove the * prefix if present)
				anchorName := valueNode.Value
				if len(anchorName) > 0 && anchorName[0] == '*' {
					anchorName = anchorName[1:]
				}

				// Store the mapping
				if protocolConverterName != "" {
					anchorMap[protocolConverterName] = anchorName
				}

				// Find and resolve the template content from the templates section
				resolvedTemplate, err := resolveTemplateFromRoot(rootNode, anchorName)
				if err != nil {
					// If we can't resolve the template, use an empty template as fallback
					resolvedTemplate = &yaml.Node{
						Kind:    yaml.MappingNode,
						Content: []*yaml.Node{},
					}
				}

				// Replace the alias node with the resolved template content
				serviceConfigNode.Content[i+1] = resolvedTemplate
			}
		}
	}
	return nil
}

// resolveTemplateFromRoot finds and returns the template content from the templates section
func resolveTemplateFromRoot(rootNode *yaml.Node, anchorName string) (*yaml.Node, error) {
	if rootNode == nil {
		return nil, fmt.Errorf("root node is nil")
	}

	// Navigate to the document content
	var documentNode *yaml.Node
	if rootNode.Kind == yaml.DocumentNode && len(rootNode.Content) > 0 {
		documentNode = rootNode.Content[0]
	} else {
		documentNode = rootNode
	}

	// Find the templates section
	if documentNode.Kind == yaml.MappingNode {
		for i := 0; i < len(documentNode.Content); i += 2 {
			keyNode := documentNode.Content[i]
			valueNode := documentNode.Content[i+1]

			if keyNode.Value == "templates" && valueNode.Kind == yaml.SequenceNode {
				// Search through the templates for the anchor
				for _, templateItem := range valueNode.Content {
					if templateItem.Kind == yaml.MappingNode {
						for j := 0; j < len(templateItem.Content); j += 2 {
							templateKeyNode := templateItem.Content[j]
							templateValueNode := templateItem.Content[j+1]

							if templateKeyNode.Value == anchorName {
								// Found the template, return a deep copy of the content
								return deepCopyYAMLNode(templateValueNode), nil
							}
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("template with anchor %q not found", anchorName)
}

// deepCopyYAMLNode creates a deep copy of a YAML node
func deepCopyYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}

	copy := &yaml.Node{
		Kind:   node.Kind,
		Style:  node.Style,
		Tag:    node.Tag,
		Value:  node.Value,
		Anchor: "", // Don't copy anchor to avoid conflicts
		Alias:  node.Alias,
	}

	if node.Content != nil {
		copy.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			copy.Content[i] = deepCopyYAMLNode(child)
		}
	}

	return copy
}

// getConfigWithAnchors returns the current configuration along with the anchor mapping
// This is used internally by atomic functions that need to know which protocol converters use anchors
func (m *FileConfigManager) getConfigWithAnchors(ctx context.Context) (FullConfig, map[string]string, error) {
	// We need to re-read and parse the file to get the anchor map
	// since GetConfig() doesn't return the anchor information

	err := m.mutexReadOrWrite.RLock(ctx)
	if err != nil {
		return FullConfig{}, nil, fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexReadOrWrite.RUnlock()

	// Read the file
	data, err := m.fsService.ReadFile(ctx, m.configPath)
	if err != nil {
		return FullConfig{}, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config, anchorMap, err := parseConfig(data, true) // Use allowUnknownFields=true for anchor handling
	if err != nil {
		return FullConfig{}, nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, anchorMap, nil
}

// generateTemplateAnchorName creates a valid YAML anchor name from a protocol converter name
func generateTemplateAnchorName(pcName string) string {
	// Replace non-alphanumeric characters with underscores and add template suffix
	// YAML anchors must contain only alphanumeric characters
	result := ""
	for _, r := range pcName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result += string(r)
		} else {
			result += "_"
		}
	}
	return result
}

// templateExists checks if a template with the given anchor name already exists
func templateExists(templates []map[string]interface{}, anchorName string) bool {
	for _, template := range templates {
		if _, exists := template[anchorName]; exists {
			return true
		}
	}
	return false
}

// createTemplateContent converts a template config to a map for YAML anchoring
func createTemplateContent(template protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate) map[string]interface{} {
	// Convert the template struct to a map that can be used in YAML templates section
	templateMap := make(map[string]interface{})

	// Add connection config using the proper struct - let YAML marshaler handle the tags
	if template.ConnectionServiceConfig.NmapTemplate != nil {
		templateMap["connection"] = template.ConnectionServiceConfig
	}

	// Add dataflow component configs (currently empty as per deploy action)
	// These will be populated later via edit actions
	if !isEmptyDataflowComponentConfig(template.DataflowComponentReadServiceConfig) {
		templateMap["dataflowcomponent_read"] = template.DataflowComponentReadServiceConfig
	}

	if !isEmptyDataflowComponentConfig(template.DataflowComponentWriteServiceConfig) {
		templateMap["dataflowcomponent_write"] = template.DataflowComponentWriteServiceConfig
	}

	return templateMap
}

// isEmptyDataflowComponentConfig checks if a dataflow component config is empty
func isEmptyDataflowComponentConfig(config dataflowcomponentserviceconfig.DataflowComponentServiceConfig) bool {
	// For now, assume it's empty if BenthosConfig is nil or empty
	return config.BenthosConfig.Input == nil && config.BenthosConfig.Output == nil
}
