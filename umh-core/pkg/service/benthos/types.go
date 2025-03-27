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

package benthos

import (
	"strings"
)

// PossiblyArray handles input/output/buffer sections that should be empty arrays when empty
type PossiblyArray map[string]interface{}

func (m PossiblyArray) MarshalYAML() (interface{}, error) {
	if len(m) == 0 {
		return []interface{}{}, nil
	}
	return map[string]interface{}(m), nil
}

// PossiblyResourceArray handles cache/rate_limit resources sections that should be empty arrays when empty
type PossiblyResourceArray []map[string]interface{}

func (a PossiblyResourceArray) MarshalYAML() (interface{}, error) {
	if len(a) == 0 {
		return []interface{}{}, nil
	}
	return []map[string]interface{}(a), nil
}

// ProcessorList handles processor arrays, ensuring processors: [] is always present
type ProcessorList []Processor

func (p ProcessorList) MarshalYAML() (interface{}, error) {
	if len(p) == 0 {
		return []interface{}{}, nil
	}

	// Convert Processor objects to proper YAML structure
	result := make([]interface{}, len(p))
	for i, proc := range p {
		m, err := proc.MarshalYAML()
		if err != nil {
			return nil, err
		}
		result[i] = m
	}

	return result, nil
}

// Processor represents a single Benthos processor configuration
type Processor struct {
	Type   string
	Config interface{}
}

// MarshalYAML handles marshaling processor configurations
func (p Processor) MarshalYAML() (interface{}, error) {
	// Handle special case for empty config
	if p.Config == nil {
		return map[string]interface{}{p.Type: map[string]interface{}{}}, nil
	}

	// Handle different config types
	switch v := p.Config.(type) {
	case string:
		// For string values like bloblang mappings
		if strings.HasPrefix(v, "|") || strings.HasPrefix(v, ">") {
			// Already has YAML block indicator
			return map[string]interface{}{p.Type: v}, nil
		}
		// For regular string values that are multi-line, add YAML block style
		if strings.Contains(v, "\n") {
			// Ensure proper YAML block scalar formatting with pipe character
			return map[string]interface{}{p.Type: "|\n" + v}, nil
		}
		return map[string]interface{}{p.Type: v}, nil

	case map[string]interface{}:
		if len(v) == 0 {
			return map[string]interface{}{p.Type: map[string]interface{}{}}, nil
		}

		// Special handling for branch processors
		if p.Type == "branch" {
			// Extract nested processors if they exist
			if nestedProcs, ok := v["processors"]; ok {
				branchProcessors := ProcessorList{}

				// Handle different types of nested processors
				switch procs := nestedProcs.(type) {
				case []interface{}:
					extractProcessorsFromInterface(procs, &branchProcessors)
				case []map[string]interface{}:
					for _, pMap := range procs {
						for pType, pConfig := range pMap {
							branchProcessors = append(branchProcessors, Processor{
								Type:   pType,
								Config: pConfig,
							})
						}
					}
				case []map[string]map[string]interface{}:
					// This is a special case format sometimes used in tests
					for _, pMap := range procs {
						for pType, pConfig := range pMap {
							branchProcessors = append(branchProcessors, Processor{
								Type:   pType,
								Config: pConfig,
							})
						}
					}
				}

				// Create a copy of v without the processors field
				vCopy := make(map[string]interface{})
				for k, val := range v {
					if k != "processors" {
						vCopy[k] = val
					}
				}

				// Add the processors back as a properly marshaled list
				vCopy["processors"] = branchProcessors

				return map[string]interface{}{p.Type: vCopy}, nil
			}
		}

		// Special handling for mapping processor with "value" field containing multi-line strings
		if p.Type == "mapping" && v["value"] != nil {
			if valueStr, ok := v["value"].(string); ok && strings.Contains(valueStr, "\n") {
				vCopy := make(map[string]interface{})
				for k, val := range v {
					if k == "value" {
						vCopy[k] = "|\n" + valueStr
					} else {
						vCopy[k] = val
					}
				}
				return map[string]interface{}{p.Type: vCopy}, nil
			}
		}

		return map[string]interface{}{p.Type: v}, nil

	default:
		return map[string]interface{}{p.Type: v}, nil
	}
}

// Pipeline represents a Benthos pipeline section
type Pipeline struct {
	Processors ProcessorList `yaml:"processors"`
}

// PossiblyEmptyObject handles buffer sections that should be empty objects when empty
type PossiblyEmptyObject map[string]interface{}

func (m PossiblyEmptyObject) MarshalYAML() (interface{}, error) {
	if len(m) == 0 {
		return map[string]interface{}{}, nil
	}
	return map[string]interface{}(m), nil
}

// BenthosConfig represents the complete Benthos configuration
type BenthosConfig struct {
	Input              PossiblyArray         `yaml:"input"`
	Pipeline           Pipeline              `yaml:"pipeline"`
	Output             PossiblyArray         `yaml:"output"`
	CacheResources     PossiblyResourceArray `yaml:"cache_resources"`
	RateLimitResources PossiblyResourceArray `yaml:"rate_limit_resources"`
	Buffer             PossiblyEmptyObject   `yaml:"buffer"`
	HTTP               HTTPConfig            `yaml:"http"`
	Logger             LoggerConfig          `yaml:"logger"`
}

// HTTPConfig represents the HTTP server configuration
type HTTPConfig struct {
	Address string `yaml:"address"`
}

// LoggerConfig represents the logger configuration
type LoggerConfig struct {
	Level string `yaml:"level"`
}

// extractProcessorsFromInterface extracts processors from an interface slice and adds them to the given ProcessorList
func extractProcessorsFromInterface(procs []interface{}, processorList *ProcessorList) {
	for _, proc := range procs {
		if procMap, ok := proc.(map[string]interface{}); ok {
			for pType, pConfig := range procMap {
				*processorList = append(*processorList, Processor{
					Type:   pType,
					Config: pConfig,
				})
			}
		}
	}
}
