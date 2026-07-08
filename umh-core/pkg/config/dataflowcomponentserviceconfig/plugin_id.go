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

package dataflowcomponentserviceconfig

// BenthosPluginID returns the benthos plugin name from a single-section input
// map, descending one level through the "input" wrapper. Non-plugin metadata
// keys (label, processors) are skipped so that labeled inputs such as
// {"label":"x","mqtt":{...}} resolve to "mqtt".
func BenthosPluginID(m map[string]any) string {
	pluginKey, ok := benthosPluginKey(m)
	if !ok {
		return ""
	}

	if pluginKey != "input" {
		return pluginKey
	}

	inner, _ := m["input"].(map[string]any)

	innerKey, ok := benthosPluginKey(inner)
	if !ok {
		return ""
	}

	return innerKey
}

// benthosPluginKey returns the sole plugin key in a benthos input map after
// skipping non-plugin metadata keys. It reports false when the map holds zero
// or more than one plugin key.
func benthosPluginKey(m map[string]any) (string, bool) {
	var pluginKeys []string

	for k := range m {
		switch k {
		case "label", "processors":
			continue
		}

		pluginKeys = append(pluginKeys, k)
	}

	if len(pluginKeys) != 1 {
		return "", false
	}

	return pluginKeys[0], true
}
