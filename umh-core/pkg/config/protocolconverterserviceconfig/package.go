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
	"fmt"
	"reflect"

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
//
// This template form allows connection parameters (like port) to be templated
// using expressions like "{{ .PORT }}" which are resolved during rendering.
type ProtocolConverterServiceConfigTemplate struct {

	// DataflowComponentWriteServiceConfig is the blueprint for the *write* side
	// of the converter. Source.Topics is a string that may contain Go template actions
	// rendered at deploy time.
	DataflowComponentWriteServiceConfig dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput `yaml:"dataflowcomponent_write,omitempty"`

	// ConnectionServiceConfig describes how the converter connects to the
	// underlying messaging infrastructure. Uses the template form to allow
	// templating of connection parameters like port numbers.
	// At render time, this gets converted to the runtime form with proper types.
	ConnectionServiceConfig connectionserviceconfig.ConnectionServiceConfigTemplate `yaml:"connection,omitempty"`

	// DataflowComponentReadServiceConfig is the blueprint for the *read* side
	// of the converter.  At render time we enforce that
	// `BenthosConfig.Output` is an UNS publisher because read‑DFCs **must not**
	// decide their own egress.
	DataflowComponentReadServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_read,omitempty"`
}

// ProtocolConverterServiceConfigRuntime is the **fully rendered** form of a
// ProtocolConverter configuration.  All template actions have been executed,
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
type ProtocolConverterServiceConfigRuntime struct {

	// ConnectionServiceConfig is the fully rendered connection configuration
	// with all template variables resolved and proper types enforced.
	ConnectionServiceConfig connectionserviceconfig.ConnectionServiceConfigRuntime `yaml:"connection"`

	// DataflowComponentReadServiceConfig and DataflowComponentWriteServiceConfig
	// hold the rendered DFC runtime configs. debug_level is configured at the
	// instance level (ProtocolConverterConfig.DebugLevel) and propagated here at runtime.
	DataflowComponentReadServiceConfig  dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_read"`
	DataflowComponentWriteServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_write"`
}

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
// Note: debug_level is configured at the instance level (ProtocolConverterConfig.DebugLevel),
// not inside dataflowcomponent_read/write (that field is yaml:"-" and ignored during unmarshal).
//
// Spec → (render) → Runtime → FSM.
type ProtocolConverterServiceConfigSpec struct {
	Variables   variables.VariableBundle `yaml:"variables,omitempty"`
	Location    map[string]string        `yaml:"location,omitempty"`
	TemplateRef string                   `yaml:"templateRef,omitempty"`
	// ReadDFCDesiredState overrides the desired state for the read DFC ("active" or "stopped").
	// When empty, the overall protocol converter desired state is used.
	ReadDFCDesiredState string `yaml:"readDFCDesiredState,omitempty"`
	// WriteDFCDesiredState overrides the desired state for the write DFC ("active" or "stopped").
	// When empty, the overall protocol converter desired state is used.
	WriteDFCDesiredState string                                 `yaml:"writeDFCDesiredState,omitempty"`
	Config               ProtocolConverterServiceConfigTemplate `yaml:"config,omitempty"`
}

// Equal checks if two ProtocolConverterServiceConfigs are equal.
func (c ProtocolConverterServiceConfigSpec) Equal(other ProtocolConverterServiceConfigSpec) bool {
	return defaultComparator.ConfigsEqual(c, other)
}

// NormalizeProtocolConverterConfig is a package-level function for easy config normalization.
func NormalizeProtocolConverterConfig(cfg ProtocolConverterServiceConfigSpec) ProtocolConverterServiceConfigSpec {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison.
func ConfigsEqual(desired, observed ProtocolConverterServiceConfigSpec) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation.
func ConfigDiff(desired, observed ProtocolConverterServiceConfigSpec) string {
	return defaultComparator.ConfigDiff(desired, observed)
}

// ConfigsEqualRuntimeWithDFCState compares two fully-rendered runtime configurations.
// Connection configs are converted back to template form first (to normalise the
// uint16 port → string port difference); DFC configs are compared directly.
//
// A DFC whose desired state is stopped is skipped: a stopped DFC renders an empty
// benthos config on disk by design, so comparing its (empty) observed config
// against the full desired config would report permanent false divergence and
// trigger an endless re-apply loop.
func ConfigsEqualRuntimeWithDFCState(desired, observed ProtocolConverterServiceConfigRuntime, readStopped, writeStopped bool) bool {
	desiredConnT := connectionserviceconfig.ConvertRuntimeToTemplate(desired.ConnectionServiceConfig)
	observedConnT := connectionserviceconfig.ConvertRuntimeToTemplate(observed.ConnectionServiceConfig)

	comparatorDFC := dataflowcomponentserviceconfig.NewComparator()

	readEqual := readStopped ||
		comparatorDFC.ConfigsEqual(desired.DataflowComponentReadServiceConfig, observed.DataflowComponentReadServiceConfig)
	writeEqual := writeStopped ||
		comparatorDFC.ConfigsEqual(desired.DataflowComponentWriteServiceConfig, observed.DataflowComponentWriteServiceConfig)

	return reflect.DeepEqual(desiredConnT, observedConnT) && readEqual && writeEqual
}

// ConfigDiffRuntimeWithDFCState returns a human-readable diff between two runtime
// configurations, skipping a DFC whose desired state is stopped (see
// ConfigsEqualRuntimeWithDFCState for why).
func ConfigDiffRuntimeWithDFCState(desired, observed ProtocolConverterServiceConfigRuntime, readStopped, writeStopped bool) string {
	desiredConnT := connectionserviceconfig.ConvertRuntimeToTemplate(desired.ConnectionServiceConfig)
	observedConnT := connectionserviceconfig.ConvertRuntimeToTemplate(observed.ConnectionServiceConfig)

	diff := ""
	if !reflect.DeepEqual(desiredConnT, observedConnT) {
		diff += fmt.Sprintf("Connection: nmap %+v vs %+v\n",
			desired.ConnectionServiceConfig.NmapServiceConfig,
			observed.ConnectionServiceConfig.NmapServiceConfig)
	}

	// Gate each DFC section on actual inequality: the underlying DFC ConfigDiff
	// returns "No significant differences" (never "") for equal configs, which
	// would otherwise emit a noisy ReadDFC/WriteDFC line every tick.
	comparatorDFC := dataflowcomponentserviceconfig.NewComparator()
	if !readStopped && !comparatorDFC.ConfigsEqual(desired.DataflowComponentReadServiceConfig, observed.DataflowComponentReadServiceConfig) {
		diff += "ReadDFC: " + comparatorDFC.ConfigDiff(desired.DataflowComponentReadServiceConfig, observed.DataflowComponentReadServiceConfig) + "\n"
	}

	if !writeStopped && !comparatorDFC.ConfigsEqual(desired.DataflowComponentWriteServiceConfig, observed.DataflowComponentWriteServiceConfig) {
		diff += "WriteDFC: " + comparatorDFC.ConfigDiff(desired.DataflowComponentWriteServiceConfig, observed.DataflowComponentWriteServiceConfig) + "\n"
	}

	return diff
}

// SpecToRuntime converts a ProtocolConverterServiceConfigSpec to a ProtocolConverterServiceConfigRuntime
// This function performs structural conversion from template to runtime types.
// It assumes the spec template contains no unresolved template variables.
// For full template rendering with variable substitution, use the runtime_config package instead.
func SpecToRuntime(spec ProtocolConverterServiceConfigSpec) (ProtocolConverterServiceConfigRuntime, error) {
	connRuntime, err := connectionserviceconfig.ConvertTemplateToRuntime(spec.Config.ConnectionServiceConfig)
	if err != nil {
		return ProtocolConverterServiceConfigRuntime{}, fmt.Errorf("invalid connection configuration: %w", err)
	}

	return ProtocolConverterServiceConfigRuntime{
		ConnectionServiceConfig:            connRuntime,
		DataflowComponentReadServiceConfig: spec.Config.DataflowComponentReadServiceConfig,
		// bridgedBy is intentionally empty: SpecToRuntime does structural conversion only,
		// not full template rendering (Source.Topics is split as-is). Use BuildRuntimeConfig for deploys.
		DataflowComponentWriteServiceConfig: spec.Config.DataflowComponentWriteServiceConfig.ToDataflowComponentServiceConfig(""),
	}, nil
}
