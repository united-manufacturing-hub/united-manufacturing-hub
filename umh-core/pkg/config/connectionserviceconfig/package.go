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

package connectionserviceconfig

import (
	"strconv"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
)

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// ConnectionServiceConfigRuntime represents the fully rendered configuration for a Connection service.
// This is the runtime form with all template variables resolved and proper types enforced.
// Only TCP probes are supported at the moment.
//
// This type is used by:
// - Connection FSM instances (after template rendering)
// - Connection service implementations
// - All existing code that expects typed configuration
//
// The port field is uint16 to ensure type safety at runtime.
type ConnectionServiceConfigRuntime struct {
	NmapServiceConfig nmapserviceconfig.NmapServiceConfig `yaml:"nmap"`
}

// ConnectionServiceConfigTemplate represents the template form of connection configuration
// that may contain Go text/template actions (e.g. {{ .PORT }}).
// This is used by protocol converters to allow templating of connection parameters.
//
// This type is used by:
// - Protocol converter templates (before rendering)
// - YAML configuration files that need templating
//
// The port field is string to allow template expressions like "{{ .PORT }}".
type ConnectionServiceConfigTemplate struct {
	NmapTemplate *NmapConfigTemplate `yaml:"nmap,omitempty"`
}

// NmapConfigTemplate is the template form of nmap configuration with string fields
// to support templating. All fields that need templating are strings.
type NmapConfigTemplate struct {
	Target string `yaml:"target"`
	Port   string `yaml:"port"` // string to allow templating like "{{ .PORT }}"
}

// NmapConfigRuntime is the runtime form of nmap configuration with proper types.
// This ensures type safety after template rendering.
type NmapConfigRuntime struct {
	Target string `yaml:"target"`
	Port   uint16 `yaml:"port"` // uint16 for type safety at runtime
}

// ConnectionServiceConfig is a backward compatibility alias.
// All existing code continues to work unchanged by using the runtime type.
// This maintains API compatibility while enabling the new template/runtime pattern.
type ConnectionServiceConfig = ConnectionServiceConfigRuntime

// Equal checks if two ConnectionServiceConfigs are equal
// This method works on the runtime configuration for backward compatibility.
func (c ConnectionServiceConfigRuntime) Equal(other ConnectionServiceConfigRuntime) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderConnectionYAML is a package-level function for easy YAML generation
// This works on runtime configuration with proper types.
func RenderConnectionYAML(nmap nmapserviceconfig.NmapServiceConfig) (string, error) {
	// Create a config object from the individual components
	cfg := ConnectionServiceConfigRuntime{
		NmapServiceConfig: nmap,
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeConnectionConfig is a package-level function for easy config normalization
// This works on runtime configuration with proper types.
func NormalizeConnectionConfig(cfg ConnectionServiceConfigRuntime) ConnectionServiceConfigRuntime {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
// This works on runtime configuration with proper types.
func ConfigsEqual(desired, observed ConnectionServiceConfigRuntime) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
// This works on runtime configuration with proper types.
func ConfigDiff(desired, observed ConnectionServiceConfigRuntime) string {
	return defaultComparator.ConfigDiff(desired, observed)
}

// ConvertRuntimeToTemplate converts a runtime configuration to a template configuration
// This is a helper function, so that when we comapre configs, we can simply use the template comparison and don't need
// to create runtime normalizer, comparator, etc.
func ConvertRuntimeToTemplate(cfg ConnectionServiceConfigRuntime) ConnectionServiceConfigTemplate {
	return ConnectionServiceConfigTemplate{
		NmapTemplate: &NmapConfigTemplate{
			Target: cfg.NmapServiceConfig.Target,
			Port:   strconv.Itoa(int(cfg.NmapServiceConfig.Port)),
		},
	}
}

// ConvertTemplateToRuntime converts a template configuration to a runtime configuration
// This is a helper function used for template-to-runtime conversion across the codebase.
//
// Conversion Process:
// 1. Parse the string port from template (e.g., "443" or rendered "{{ .PORT }}") to uint16
// 2. Build runtime config with proper Go types for type safety
// 3. Handle conversion errors by returning empty config (caller should check for zero values)
//
// Template vs Runtime Types:
// - Template: Uses string port for YAML templating compatibility
// - Runtime: Uses uint16 port for type safety during FSM operations
//
// This function is used by:
// - Protocol converter runtime rendering (after template variable substitution)
// - Spec-to-runtime conversions for structural type conversion
// - Any code that needs to convert from template form to runtime form
//
// NOTE: this does NOT perform template rendering. It only converts the template form to runtime form.
func ConvertTemplateToRuntime(cfg ConnectionServiceConfigTemplate) ConnectionServiceConfigRuntime {
	// Handle nil NmapTemplate (e.g., from empty/uninitialized configs)
	if cfg.NmapTemplate == nil {
		return ConnectionServiceConfigRuntime{} // Return empty config for nil template
	}

	// Parse string port to uint16 for runtime type safety
	// Template uses string to allow expressions like "{{ .PORT }}", runtime needs uint16
	port, err := strconv.ParseUint(cfg.NmapTemplate.Port, 10, 16)
	if err != nil {
		return ConnectionServiceConfigRuntime{} // Return empty config on conversion error
	}

	// Build runtime config with proper types - this is the final form used by FSMs
	return ConnectionServiceConfigRuntime{
		NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
			Target: cfg.NmapTemplate.Target,
			Port:   uint16(port), // Convert from string template to uint16 runtime type
		},
	}
}
