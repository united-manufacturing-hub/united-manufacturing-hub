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
	"fmt"
	"regexp"
	"strings"
)

// ComponentType represents the type of UMH component for bridged_by header generation
//
// To add a new component type:
//  1. Add a new constant to this enum (e.g., ComponentTypeMyComponent ComponentType = "my-component")
//  2. Update your component's runtime config to call GenerateBridgedBy with the new constant
//  3. Add tests for the new component type in runtime_utils_test.go
type ComponentType string

const (
	// ComponentTypeProtocolConverter represents a protocol converter component
	ComponentTypeProtocolConverter ComponentType = "protocol-converter"
	// ComponentTypeStreamProcessor represents a stream processor component
	ComponentTypeStreamProcessor ComponentType = "stream-processor"
)

// String returns the string representation of the ComponentType
func (ct ComponentType) String() string {
	return string(ct)
}

// At the top of the file, alongside your imports:
var (
    reNonAlnum  = regexp.MustCompile(`[^a-zA-Z0-9]`)
    reMultiDash = regexp.MustCompile(`-{2,}`)
)


// GenerateBridgedBy creates a sanitized bridged_by header for UMH components.
// It takes a component type, node name, and component name and returns a properly
// formatted and sanitized identifier.
//
// Parameters:
//   - componentType: The type of component (use ComponentType constants)
//   - nodeName: The name of the node/agent (will default to "unknown" if empty)
//   - componentName: The name of the specific component instance
//
// Returns:
//   - A sanitized string in the format "{componentType}_{nodeName}_{componentName}"
//
// The sanitization process:
//  1. Sanitizes each component individually by replacing non-alphanumeric characters with dashes
//  2. Joins the sanitized components with underscores
//  3. Collapses multiple consecutive dashes into single dashes
//  4. Trims leading and trailing dashes
//
// Examples:
//
//	GenerateBridgedBy(ComponentTypeProtocolConverter, "test-node", "temp-sensor")
//	// → "protocol-converter_test-node_temp-sensor"
//
//	GenerateBridgedBy(ComponentTypeStreamProcessor, "test@node#1", "pump.sp@2")
//	// → "stream-processor_test-node-1_pump-sp-2"
//
//	GenerateBridgedBy(ComponentTypeProtocolConverter, "", "sensor")
//	// → "protocol-converter_unknown_sensor"
func GenerateBridgedBy(componentType ComponentType, nodeName, componentName string) string {
	if nodeName == "" {
		nodeName = "unknown"
	}

	componentTypeStr := strings.Trim(
		reNonAlnum.ReplaceAllString(componentType.String(), "-"),
		"-",
	)
	nodeNameStr := strings.Trim(
		reNonAlnum.ReplaceAllString(nodeName, "-"),
		"-",
	)
	componentNameStr := strings.Trim(
		reNonAlnum.ReplaceAllString(componentName, "-"),
		"-",
	)

	bridgeName := fmt.Sprintf("%s_%s_%s", componentTypeStr, nodeNameStr, componentNameStr)

	// Collapse multiple consecutive dashes into single dashes
	bridgeName = reMultiDash.ReplaceAllString(bridgeName, "-")

	// Trim leading and trailing dashes
	return strings.Trim(bridgeName, "-")
}
