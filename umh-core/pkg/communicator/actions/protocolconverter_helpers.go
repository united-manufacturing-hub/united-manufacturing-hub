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

package actions

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// buildUserScope constructs the template rendering scope from a ProtocolConverter's
// TemplateInfo. Returns an empty map when templateInfo is nil.
func buildUserScope(templateInfo *models.ProtocolConverterTemplateInfo) map[string]any {
	if templateInfo == nil {
		return map[string]any{}
	}
	scope := make(map[string]any, len(templateInfo.Variables))
	for _, v := range templateInfo.Variables {
		scope[v.Label] = v.Value
	}
	return scope
}

// validateWriteDFCConfig validates a raw write DFC config input and its desired state.
func validateWriteDFCConfig(cfg *dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput, state string) error {
	if cfg != nil && cfg.HasOutput() {
		if strings.TrimSpace(cfg.InputTopics) == "" {
			return errors.New("write DFC requires at least one input topic (input_topics)")
		}
	}
	if state != "" {
		if err := ValidateDataFlowComponentState(state); err != nil {
			return fmt.Errorf("invalid write DFC state: %w", err)
		}
	}
	return nil
}


// validateReadProtocolConverterDFC validates a read ProtocolConverterDFC's state and configuration.
// Returns nil if dfc is nil (nothing to validate).
func validateReadProtocolConverterDFC(dfc *models.ProtocolConverterDFC) error {
	if dfc == nil {
		return nil
	}
	if dfc.State != "" {
		if err := ValidateDataFlowComponentState(dfc.State); err != nil {
			return fmt.Errorf("invalid read DFC state: %w", err)
		}
	}
	payload := dfcToPayload(dfc)
	if err := ValidateCustomDataFlowComponentPayload(payload, true, false); err != nil {
		return fmt.Errorf("invalid read DFC configuration: %w", err)
	}
	return nil
}

// newIPPortConnectionTemplate returns the standard {{ .IP }}/{{ .PORT }} Nmap connection template
// used by all protocol converters.
func newIPPortConnectionTemplate() connectionserviceconfig.ConnectionServiceConfigTemplate {
	return connectionserviceconfig.ConnectionServiceConfigTemplate{
		NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
			Target: "{{ .IP }}",
			Port:   "{{ .PORT }}",
		},
	}
}

// buildReadDFCServiceConfig validates a CDFCPayload and converts it into a
// DataflowComponentServiceConfig ready to be stored in a ProtocolConverterServiceConfigTemplate.
func buildReadDFCServiceConfig(payload models.CDFCPayload, name string) (dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error) {
	if err := ValidateCustomDataFlowComponentPayload(payload, true, false); err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("invalid read DFC configuration: %w", err)
	}
	benthos, err := CreateBenthosConfigFromCDFCPayload(payload, name)
	if err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("failed to create read DFC benthos config: %w", err)
	}
	return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{BenthosConfig: benthos}, nil
}

// convertIntMapToStringMap converts map[int]string to map[string]string.
func convertIntMapToStringMap(intMap map[int]string) map[string]string {
	if intMap == nil {
		return nil
	}
	result := make(map[string]string, len(intMap))
	for k, v := range intMap {
		result[strconv.Itoa(k)] = v
	}
	return result
}

// dfcToPayload converts a ProtocolConverterDFC to the internal CDFCPayload representation.
func dfcToPayload(dfc *models.ProtocolConverterDFC) models.CDFCPayload {
	return models.CDFCPayload{
		Inputs:   models.DfcDataConfig{Data: dfc.Inputs.Data, Type: dfc.Inputs.Type},
		Pipeline: convertPipelineToMap(dfc.Pipeline),
		Outputs:  models.DfcDataConfig{Data: dfc.Outputs.Data, Type: dfc.Outputs.Type},
		Inject:   extractInjectFromRawYAML(dfc.RawYAML),
	}
}

// convertPipelineToMap converts CommonDataFlowComponentPipelineConfig to map[string]DfcDataConfig.
func convertPipelineToMap(pipeline models.CommonDataFlowComponentPipelineConfig) map[string]models.DfcDataConfig {
	result := make(map[string]models.DfcDataConfig)
	for key, processor := range pipeline.Processors {
		result[key] = models.DfcDataConfig{
			Data: processor.Data,
			Type: processor.Type,
		}
	}
	return result
}

// extractInjectFromRawYAML extracts the inject data from the RawYAML field to support
// CacheResources, RateLimitResources, and Buffer in protocol converter DFCs.
func extractInjectFromRawYAML(rawYAML *models.CommonDataFlowComponentRawYamlConfig) string {
	if rawYAML == nil {
		return ""
	}
	return rawYAML.Data
}

// observedBenthosToServiceConfig converts an observed BenthosServiceConfig to a
// DataflowComponentServiceConfig for comparison with the desired config.
func observedBenthosToServiceConfig(obs benthosserviceconfig.BenthosServiceConfig) dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input:              obs.Input,
			Pipeline:           obs.Pipeline,
			Output:             obs.Output,
			CacheResources:     obs.CacheResources,
			RateLimitResources: obs.RateLimitResources,
			Buffer:             obs.Buffer,
		},
	}
}

// dfcTypeFromPresence returns a DFCType based on boolean flags for read/write presence.
func dfcTypeFromPresence(read, write bool) DFCType {
	switch {
	case read && write:
		return DFCTypeBoth
	case read:
		return DFCTypeRead
	case write:
		return DFCTypeWrite
	default:
		return DFCTypeEmpty
	}
}
