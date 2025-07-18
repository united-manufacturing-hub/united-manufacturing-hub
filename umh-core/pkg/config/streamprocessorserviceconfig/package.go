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

package streamprocessorserviceconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

var (
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// ModelRef represents a reference to a data model
type ModelRef struct {
	Name    string `yaml:"name"`    // name of the referenced data model
	Version string `yaml:"version"` // version of the referenced data model
}

// SourceMapping represents the mapping of source aliases to UNS topics
type SourceMapping map[string]string

// StreamProcessorServiceConfigTemplate is the *blueprint* for deploying a
// Stream Processor. The struct **may still contain** Go
// `text/template` actions (e.g. `{{ .location_path }}`) so that callers can
// substitute values later on.
//
// This template form allows stream processor parameters to be templated
// using expressions like "{{ .abc }}" which are resolved during rendering.
type StreamProcessorServiceConfigTemplate struct {
	// Model defines which data model this stream processor implements
	Model ModelRef `yaml:"model"`

	// Sources defines the mapping of source aliases to UNS topics with template support
	Sources SourceMapping `yaml:"sources,omitempty"`

	// Mapping defines how source values are transformed into model fields

	Mapping map[string]interface{} `yaml:"mapping,omitempty"`
}

// StreamProcessorServiceConfigRuntime is the **fully rendered** form of a
// StreamProcessor configuration. All template actions have been executed,
// Variables injected and the UNS guard‑rails enforced. This is the *only*
// structure the FSM should ever receive.
//
// Invariants:
//   - MUST NOT contain any `{{ ... }}` directives.
//   - Model reference must point to a valid data model.
//   - All source mappings must be resolved to valid UNS topics.
type StreamProcessorServiceConfigRuntime struct {
	// Model defines which data model this stream processor implements
	Model ModelRef `yaml:"model"`

	// Sources defines the resolved mapping of source aliases to UNS topics
	Sources SourceMapping `yaml:"sources"`

	// Mapping defines how source values are transformed into model fields
	// here we don't differentiate between static and dynamic mappings
	// the difference is tackled in the benthos plugin
	// "dynamic" mappings are mappings that are triggered by incoming messages in the specified source topic
	// "static" mappings are mappings that are triggered by the stream processor itself (e.g. a static value)
	Mapping map[string]interface{} `yaml:"mapping"`
}

// StreamProcessorServiceConfigSpec is the **user‑facing** wrapper that binds a
// reusable Template to concrete runtime Variables and optional placement
// metadata (`Location`). This is what can be found in the unmarshalled YAML file.
//
// Field semantics:
//   - Config  – the blueprint to render; **required**.
//   - Variables – a bag of key/value pairs that are exposed to the Template's
//     `text/template` actions.  Optional: an empty bundle leaves
//     placeholders unresolved.
//   - Location  – used to specify the exact location of the stream processor in addition to the agent.location
//     (which takes precedence). Will be added to the variables and then accessible via `{{ .location }}`.
//
// Spec → (render) → Runtime → FSM.
type StreamProcessorServiceConfigSpec struct {
	Config      StreamProcessorServiceConfigTemplate `yaml:"config,omitempty"`
	Variables   variables.VariableBundle             `yaml:"variables,omitempty"`
	Location    map[string]string                    `yaml:"location,omitempty"`
	TemplateRef string                               `yaml:"templateRef,omitempty"`
}

// Equal checks if two StreamProcessorServiceConfigs are equal
func (c StreamProcessorServiceConfigSpec) Equal(other StreamProcessorServiceConfigSpec) bool {
	return defaultComparator.ConfigsEqual(c, other)
}

// NormalizeStreamProcessorConfig is a package-level function for easy config normalization
func NormalizeStreamProcessorConfig(cfg StreamProcessorServiceConfigSpec) StreamProcessorServiceConfigSpec {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed StreamProcessorServiceConfigSpec) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed StreamProcessorServiceConfigSpec) string {
	return defaultComparator.ConfigDiff(desired, observed)
}

// convertRuntimeToTemplate converts a runtime configuration back to template format.
// This helper exists because our comparison and diff logic operates on template types,
// but FSMs work with runtime types. When we need to compare or diff runtime configs,
// we must first convert them back to template format to reuse existing logic.
func convertRuntimeToTemplate(runtime StreamProcessorServiceConfigRuntime) StreamProcessorServiceConfigTemplate {
	return StreamProcessorServiceConfigTemplate(runtime)
}

// ConfigsEqualRuntime is a package-level function for comparing runtime configurations
func ConfigsEqualRuntime(desired, observed StreamProcessorServiceConfigRuntime) bool {
	// Convert runtime configs back to template format for comparison
	streamProcessorDesiredTemplate := convertRuntimeToTemplate(desired)
	streamProcessorObservedTemplate := convertRuntimeToTemplate(observed)

	// Convert runtime configs to spec configs for comparison
	desiredSpec := StreamProcessorServiceConfigSpec{Config: streamProcessorDesiredTemplate}
	observedSpec := StreamProcessorServiceConfigSpec{Config: streamProcessorObservedTemplate}
	return defaultComparator.ConfigsEqual(desiredSpec, observedSpec)
}

// ConfigDiffRuntime is a package-level function for generating diffs between runtime configurations
func ConfigDiffRuntime(desired, observed StreamProcessorServiceConfigRuntime) string {
	// Convert runtime configs back to template format for diffing
	streamProcessorDesiredTemplate := convertRuntimeToTemplate(desired)
	streamProcessorObservedTemplate := convertRuntimeToTemplate(observed)

	// Convert to spec configs for diffing
	desiredSpec := StreamProcessorServiceConfigSpec{Config: streamProcessorDesiredTemplate}
	observedSpec := StreamProcessorServiceConfigSpec{Config: streamProcessorObservedTemplate}
	return defaultComparator.ConfigDiff(desiredSpec, observedSpec)
}

// SpecToRuntime converts a StreamProcessorServiceConfigSpec to a StreamProcessorServiceConfigRuntime
// This function performs structural conversion from template to runtime types.
// It assumes the spec template contains no unresolved template variables.
// For full template rendering with variable substitution, use the runtime_config package instead.
func SpecToRuntime(spec StreamProcessorServiceConfigSpec) StreamProcessorServiceConfigRuntime {
	return StreamProcessorServiceConfigRuntime{
		Model:   spec.Config.Model,
		Sources: spec.Config.Sources,
		Mapping: spec.Config.Mapping,
	}
}
