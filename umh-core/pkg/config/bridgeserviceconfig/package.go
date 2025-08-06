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

package bridgeserviceconfig

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

var (
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// ConfigTemplate is the *blueprint* for deploying a
// Bridge. The struct **may still contain** Go
// `text/template` actions (e.g. `{{ .bridged_by }}`) so that callers can
// substitute values later on.
//
// This template form allows connection parameters (like port) to be templated
// using expressions like "{{ .PORT }}" which are resolved during rendering.
type ConfigTemplate struct { // ConnectionServiceConfig describes how the converter connects to the
	// underlying messaging infrastructure. Uses the template form to allow
	// templating of connection parameters like port numbers.
	// At render time, this gets converted to the runtime form with proper types.
	ConnectionConfig connectionserviceconfig.ConnectionServiceConfigTemplate `yaml:"connection,omitempty"`

	// DFCReadConfig is the blueprint for the *read* side
	// of the converter.  At render time we enforce that
	// `BenthosConfig.Output` is an UNS publisher because read‑DFCs **must not**
	// decide their own egress.
	DFCReadConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_read,omitempty"`

	// DFCWriteConfig is the blueprint for the *write* side
	// of the converter.  Symmetrically to the read‑DFC we override
	// `BenthosConfig.Input` so that it always consumes from UNS.
	DFCWriteConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_write,omitempty"`
}

// ConfigRuntime is the **fully rendered** form of a
// Bridge configuration.  All template actions have been executed,
// Variables injected and the UNS guard‑rails enforced.  This is the *only*
// structure the FSM should ever receive.
//
// The connection config uses the runtime form with proper types (uint16 port)
// to ensure type safety during FSM operations.
//
// Invariants:
//   - MUST NOT contain any `{{ ... }}` directives.
//   - `DataflowComponentReadServiceConfig.BenthosConfig.Output` **is** UNS.
//   - `DataflowComponentWriteServiceConfig.BenthosConfig.Input` **is** UNS.
//   - `ConnectionServiceConfig` has all template variables resolved with proper types.
type ConfigRuntime struct {
	// ConnectionConfig is the fully rendered connection configuration
	// with all template variables resolved and proper types enforced.
	ConnectionConfig connectionserviceconfig.ConnectionServiceConfigRuntime `yaml:"connection"`

	// DFCReadConfig and Dataflow	// remain unchanged as they don't need the template/runtime split yet.
	DFCReadConfig  dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_read"`
	DFCWriteConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_write"`
}

// ConfigSpec is the **user‑facing** wrapper that binds a
// reusable Template to concrete runtime Variables and optional placement
// metadata (`Location`). This is what can be found in the unmarshalled YAML file.
// Please note that the actual config.yaml file might contain additional yaml anchors
// that will land up in the rendered form here into the ConfigSpec
// as well.
//
// Field semantics:
//   - Template  – the blueprint to render; **required**.
//   - Variables – a bag of key/value pairs that are exposed to the Template's
//     `text/template` actions.  Optional: an empty bundle leaves
//     placeholders unresolved.
//   - Location  – used to further specify the exact location of the converter in addition to the agent.location
//     (which takes precedence). Will be added to the variables and then accessible via `{{ .location }}`.
//
// Spec → (render) → Runtime → FSM.
type ConfigSpec struct {
	Variables   variables.VariableBundle `yaml:"variables,omitempty"`
	Location    map[string]string        `yaml:"location,omitempty"`
	TemplateRef string                   `yaml:"templateRef,omitempty"`
	Config      ConfigTemplate           `yaml:"config,omitempty"`
}

// Equal checks if two Configs are equal
func (c ConfigSpec) Equal(other ConfigSpec) bool {
	return defaultComparator.ConfigsEqual(c, other)
}

// NormalizeConfig is a package-level function for easy config normalization
func NormalizeConfig(cfg ConfigSpec) ConfigSpec {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed ConfigSpec) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed ConfigSpec) string {
	return defaultComparator.ConfigDiff(desired, observed)
}

// convertRuntimeToTemplate converts a runtime configuration back to template format.
// This helper exists because our comparison and diff logic operates on template types,
// but FSMs work with runtime types. When we need to compare or diff runtime configs,
// we must first convert them back to template format to reuse existing logic.
//
// The conversion process:
// - Runtime connection config (uint16 port) → Template connection config (string port)
// - DFC configs remain unchanged (they're already template-compatible)
//
// This avoids duplicating the conversion logic across ConfigsEqualRuntime and ConfigDiffRuntime.
func convertRuntimeToTemplate(runtime ConfigRuntime) ConfigTemplate {
	connectionTemplate := connectionserviceconfig.ConvertRuntimeToTemplate(runtime.ConnectionConfig)

	return ConfigTemplate{
		ConnectionConfig: connectionTemplate,
		DFCReadConfig:    runtime.DFCReadConfig,
		DFCWriteConfig:   runtime.DFCWriteConfig,
	}
}

// ConfigsEqualRuntime is a package-level function for comparing runtime configurations
func ConfigsEqualRuntime(desired, observed ConfigRuntime) bool {
	// Convert runtime configs back to template format for comparison
	// This is necessary because our comparison logic operates on template types
	// Runtime types (uint16 port) → Template types (string port)
	desiredTemplate := convertRuntimeToTemplate(desired)
	observedTemplate := convertRuntimeToTemplate(observed)

	// Convert runtime configs to spec configs for comparison
	// This allows us to reuse the existing comparison logic that operates on specs
	// The comparison will handle deep equality checking of all nested fields
	desiredSpec := ConfigSpec{Config: desiredTemplate}
	observedSpec := ConfigSpec{Config: observedTemplate}
	return defaultComparator.ConfigsEqual(desiredSpec, observedSpec)
}

// ConfigDiffRuntime is a package-level function for generating diffs between runtime configurations
func ConfigDiffRuntime(desired, observed ConfigRuntime) string {
	// Convert runtime configs back to template format for diffing
	// This allows us to reuse the existing diff generation logic
	desiredTemplate := convertRuntimeToTemplate(desired)
	observedTemplate := convertRuntimeToTemplate(observed)

	// Convert to spec configs for diffing
	desiredSpec := ConfigSpec{Config: desiredTemplate}
	observedSpec := ConfigSpec{Config: observedTemplate}
	return defaultComparator.ConfigDiff(desiredSpec, observedSpec)
}

// SpecToRuntime converts a ConfigSpec to a ConfigRuntime
// This function performs structural conversion from template to runtime types.
// It assumes the spec template contains no unresolved template variables.
// For full template rendering with variable substitution, use the runtime_config package instead.
func SpecToRuntime(spec ConfigSpec) (ConfigRuntime, error) {
	// Convert template connection config to runtime format using existing helper
	// This handles the string-to-uint16 port conversion properly
	connRuntime, err := connectionserviceconfig.ConvertTemplateToRuntime(spec.Config.ConnectionConfig)
	if err != nil {
		return ConfigRuntime{}, fmt.Errorf("invalid connection configuration: %w", err)
	}

	return ConfigRuntime{
		ConnectionConfig: connRuntime,
		DFCReadConfig:    spec.Config.DFCReadConfig,
		DFCWriteConfig:   spec.Config.DFCWriteConfig,
	}, nil
}
