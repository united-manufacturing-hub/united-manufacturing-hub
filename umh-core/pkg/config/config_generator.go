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
	"context"
	"errors"
	"fmt"
)

// ErrNoProtocolConverterFound is returned when no protocol converter is found in the configuration.
var ErrNoProtocolConverterFound = errors.New("no protocol converter found")

// GenerateConfig creates a test configuration with a specified number of processors.
// The specific config is not important, it is just a test configuration that is intended to be long
// as we only use it to test runtime of the parsing of the config.
func GenerateConfig(processors int, fcm *FileConfigManager) (FullConfig, error) {
	fullConfig, err := fcm.GetConfig(context.Background(), 0)
	if err != nil {
		return FullConfig{}, err
	}

	if len(fullConfig.ProtocolConverter) == 0 {
		return FullConfig{}, ErrNoProtocolConverterFound
	}

	pc := fullConfig.ProtocolConverter[0]
	pipeline := make(map[string]any)
	pipeline["processors"] = make(map[string]any)

	for i := range processors {
		if processors, ok := pipeline["processors"].(map[string]any); ok {
			processors[fmt.Sprintf("processor_%d", i)] = map[string]any{
				"name": "long_processor",
				"config": map[string]any{
					"long_processor": "long_processor",
				},
			}
		}
	}

	pc.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline = pipeline

	fullConfig.ProtocolConverter[0] = pc

	return fullConfig, nil
}
