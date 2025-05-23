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

package protocolconverterserviceconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

var (
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// ProtocolConverterServiceConfigTemplate is the *blueprint* for deploying a
// Protocol-Converter. The struct **may still contain** Go
// `text/template` actions (e.g. `{{ .bridged_by }}`) so that callers can
// substitute values later on.
type ProtocolConverterServiceConfigTemplate struct {

	// ConnectionServiceConfig describes how the converter connects to the
	// underlying messaging infrastructure.  **Copied verbatim** into the final
	// Runtime config – no guard‑rails are applied here.
	ConnectionServiceConfig connectionserviceconfig.ConnectionServiceConfig `yaml:"connection"`

	// DataflowComponentReadServiceConfig is the blueprint for the *read* side
	// of the converter.  At render time we enforce that
	// `BenthosConfig.Output` is an UNS publisher because read‑DFCs **must not**
	// decide their own egress.
	DataflowComponentReadServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_read"`

	// DataflowComponentWriteServiceConfig is the blueprint for the *write* side
	// of the converter.  Symmetrically to the read‑DFC we override
	// `BenthosConfig.Input` so that it always consumes from UNS.
	DataflowComponentWriteServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_write"`
}

// ProtocolConverterServiceConfigRuntime is the **fully rendered** form of a
// ProtocolConverter configuration.  All template actions have been executed,
// Variables injected and the UNS guard‑rails enforced.  This is the *only*
// structure the FSM should ever receive.
//
// It is defined as a type‑alias of `ProtocolConverterServiceConfigTemplate` to
// make clear that the *shape* is identical – only the state differs.
//
// Invariants:
//   - MUST NOT contain any `{{ ... }}` directives.
//   - `DataflowComponentReadServiceConfig.BenthosConfig.Output` **is** UNS.
//   - `DataflowComponentWriteServiceConfig.BenthosConfig.Input` **is** UNS.
type ProtocolConverterServiceConfigRuntime ProtocolConverterServiceConfigTemplate

// ProtocolConverterServiceConfigSpec is the **user‑facing** wrapper that binds a
// reusable Template to concrete runtime Variables and optional placement
// metadata (`Location`). This is what can be found in the unmarshalled YAML file.
// Please note that the actual config.yaml file might contain additional yaml anchors
// that will land up in the rendered form here into the ProtocolConverterServiceConfigSpec
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
type ProtocolConverterServiceConfigSpec struct {
	Template  ProtocolConverterServiceConfigTemplate `yaml:"template"`
	Variables variables.VariableBundle               `yaml:"variables,omitempty"`
	Location  map[string]string                      `yaml:"location,omitempty"`
}

// Equal checks if two ProtocolConverterServiceConfigs are equal
func (c ProtocolConverterServiceConfigSpec) Equal(other ProtocolConverterServiceConfigSpec) bool {
	return defaultComparator.ConfigsEqual(c, other)
}

// NormalizeProtocolConverterConfig is a package-level function for easy config normalization
func NormalizeProtocolConverterConfig(cfg ProtocolConverterServiceConfigSpec) ProtocolConverterServiceConfigSpec {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed ProtocolConverterServiceConfigSpec) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed ProtocolConverterServiceConfigSpec) string {
	return defaultComparator.ConfigDiff(desired, observed)
}

// ConfigsEqualRuntime is a package-level function for comparing runtime configurations
func ConfigsEqualRuntime(desired, observed ProtocolConverterServiceConfigRuntime) bool {
	// Convert runtime configs to spec configs for comparison
	// This allows us to reuse the existing comparison logic
	desiredSpec := ProtocolConverterServiceConfigSpec{Template: ProtocolConverterServiceConfigTemplate(desired)}
	observedSpec := ProtocolConverterServiceConfigSpec{Template: ProtocolConverterServiceConfigTemplate(observed)}
	return defaultComparator.ConfigsEqual(desiredSpec, observedSpec)
}

// ConfigDiffRuntime is a package-level function for generating diffs between runtime configurations
func ConfigDiffRuntime(desired, observed ProtocolConverterServiceConfigRuntime) string {
	// Convert runtime configs to spec configs for diffing
	// This allows us to reuse the existing diff generation logic
	desiredSpec := ProtocolConverterServiceConfigSpec{Template: ProtocolConverterServiceConfigTemplate(desired)}
	observedSpec := ProtocolConverterServiceConfigSpec{Template: ProtocolConverterServiceConfigTemplate(observed)}
	return defaultComparator.ConfigDiff(desiredSpec, observedSpec)
}

// SpecToRuntime converts a ProtocolConverterServiceConfigSpec to a ProtocolConverterServiceConfigRuntime
// This is commonly used when we need to convert from the config structure to the runtime structure
func SpecToRuntime(spec ProtocolConverterServiceConfigSpec) ProtocolConverterServiceConfigRuntime {
	return ProtocolConverterServiceConfigRuntime(spec.Template)
}
