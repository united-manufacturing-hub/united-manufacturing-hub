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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

var (
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

type ConfigTemplate struct {
	DFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent,omitempty"`
}

// RuntimeConfig is the **fully rendered** form of a
type RuntimeConfig struct {
	DFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent"`
}

type ConfigSpec struct {
	Config      ConfigTemplate           `yaml:"config,omitempty"`
	TemplateRef string                   `yaml:"templateRef,omitempty"`
	Location    map[int]string           `yaml:"location,omitempty"`
	Variables   variables.VariableBundle `yaml:"variables,omitempty"`
}

// Equal checks if two ConfigSpecs are equal
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
func convertRuntimeToTemplate(runtime RuntimeConfig) ConfigTemplate {
	// placeholder

	return ConfigTemplate{
		DFCConfig: runtime.DFCConfig,
	}
}

// ConfigsEqualRuntime is a package-level function for comparing runtime configurations
func ConfigsEqualRuntime(desired, observed RuntimeConfig) bool {
	// Convert runtime configs back to template format for comparison
	desiredTemplate := convertRuntimeToTemplate(desired)
	observedTemplate := convertRuntimeToTemplate(observed)

	// Convert runtime configs to spec configs for comparison
	desiredSpec := ConfigSpec{Config: desiredTemplate}
	observedSpec := ConfigSpec{Config: observedTemplate}
	return defaultComparator.ConfigsEqual(desiredSpec, observedSpec)
}

// ConfigDiffRuntime is a package-level function for generating diffs between runtime configurations
func ConfigDiffRuntime(desired, observed RuntimeConfig) string {
	desiredTemplate := convertRuntimeToTemplate(desired)
	observedTemplate := convertRuntimeToTemplate(observed)

	// Convert to spec configs for diffing
	desiredSpec := ConfigSpec{Config: desiredTemplate}
	observedSpec := ConfigSpec{Config: observedTemplate}
	return defaultComparator.ConfigDiff(desiredSpec, observedSpec)
}

// SpecToRuntime converts a ConfigSpec to a RuntimeConfig
func SpecToRuntime(spec ConfigSpec) (RuntimeConfig, error) {
	// Convert template connection config to runtime format using existing helper
	// This handles the string-to-uint16 port conversion properly

	return RuntimeConfig{
		DFCConfig: spec.Config.DFCConfig,
	}, nil
}
