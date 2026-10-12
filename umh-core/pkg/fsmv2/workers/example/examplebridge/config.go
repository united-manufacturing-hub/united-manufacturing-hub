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

package examplebridge

import (
	"fmt"
	"sort"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExamplebridgeConfig is the typed configuration of the example bridge worker.
type ExamplebridgeConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	// Name is the bridge's name.
	Name string `json:"name" yaml:"name"`
	// Connection is the bridge's connection. Host and Port become the IP and
	// PORT template names.
	Connection Connection `json:"connection" yaml:"connection"`
	// Location is the bridge's own location levels, keyed by the level
	// index as written in YAML. The bridge merges these levels with the
	// agent's to build the location names.
	Location map[string]string `json:"location,omitempty" yaml:"location"`
	// Variables is the bridge's own variables section. It becomes the
	// "variables" template name and the source of one shortcut per variable.
	Variables map[string]any `json:"variables" yaml:"variables"`
	// Read is a flow template. The bridge keeps it unrendered: a flow renders
	// it against the names the bridge builds, never the bridge itself.
	Read config.Template `json:"read,omitempty" yaml:"read"`
	// Write is a flow template, like Read.
	Write config.Template `json:"write,omitempty" yaml:"write"`
	// Names are the template names DeriveDesiredState builds.
	Names map[string]any `json:"names,omitempty" yaml:"-"`
}

// Connection is the bridge's connection.
type Connection struct {
	// Host becomes the IP template name.
	Host string `json:"host" yaml:"host"`
	// Port becomes the PORT template name.
	Port int `json:"port" yaml:"port"`
}

// Template returns the flow template named by flow ("read" or "write").
func (c ExamplebridgeConfig) Template(flow string) config.Template {
	switch flow {
	case "read":
		return c.Read
	case "write":
		return c.Write
	default:
		return ""
	}
}

// ReadyMade returns the bridge's ready-made names: the agent's location levels
// from cfgValues merged with the bridge's own Location levels as the location
// name, and their joined path as location_path. Both sides pass the level key
// check of LocationFromAny, so a key that is not a canonical level index is a
// *ConfigError on the location field. A bridge with no location on either side
// yields the empty path.
func (c ExamplebridgeConfig) ReadyMade(cfgValues map[string]any) (map[string]any, error) {
	agentLocation, err := agentLocationFrom(cfgValues)
	if err != nil {
		return nil, err
	}

	own, err := config.LocationFromAny(c.Location)
	if err != nil {
		return nil, err
	}

	merged, locationPath := config.MergeAgentLocation(agentLocation, own)

	mergedLocation := make(map[string]any, len(merged))
	for level, value := range merged {
		mergedLocation[level] = value
	}

	return map[string]any{
		"location":      mergedLocation,
		"location_path": locationPath,
	}, nil
}

// Shortcuts returns the bridge's shortcut names: IP and PORT from the
// connection, one name per variable, and the historian shortcut from
// cfgValues. Two shortcuts that claim one name are a *ConfigError naming both
// sources.
func (c ExamplebridgeConfig) Shortcuts(cfgValues map[string]any) (map[string]any, error) {
	connShortcuts := map[string]any{}
	if c.Connection.Host != "" {
		connShortcuts["IP"] = c.Connection.Host
	}
	if c.Connection.Port != 0 {
		connShortcuts["PORT"] = c.Connection.Port
	}

	shortcutSources := []struct {
		label  string
		values map[string]any
	}{
		{"the connection shortcut", connShortcuts},
		{"a variable", config.VariableShortcuts(c.Variables)},
		{"the historian shortcut", config.HistorianShortcut(cfgValues)},
	}

	shortcuts := map[string]any{}
	shortcutSource := map[string]string{}

	for _, source := range shortcutSources {
		for _, name := range sortedKeys(source.values) {
			previous, exists := shortcutSource[name]
			if exists {
				return nil, &config.ConfigError{
					Field: name,
					Err:   fmt.Errorf("names: %q is both %s and %s", name, previous, source.label),
				}
			}

			shortcutSource[name] = source.label
			shortcuts[name] = source.values[name]
		}
	}

	return shortcuts, nil
}

// agentLocationFrom reads the agent's location out of the config values a
// parent hands down. A parent that hands down no agent yields the empty
// location; an agent that is not a mapping is a *ConfigError on the agent
// field.
func agentLocationFrom(cfgValues map[string]any) (map[string]string, error) {
	raw, exists := cfgValues["agent"]
	if !exists {
		return map[string]string{}, nil
	}

	agent, ok := raw.(map[string]any)
	if !ok {
		return nil, &config.ConfigError{
			Field: "agent",
			Err:   fmt.Errorf("agent must be a mapping, got %T", raw),
		}
	}

	location, err := config.LocationFromAny(agent["location"])
	if err != nil {
		return nil, fmt.Errorf("failed to read the agent location: %w", err)
	}

	return location, nil
}

// sortedKeys returns the keys of m in sorted order, so a name two sources
// claim always produces the same error.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))

	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// ExamplebridgeStatus is the observed status of the example bridge worker.
type ExamplebridgeStatus struct{}
