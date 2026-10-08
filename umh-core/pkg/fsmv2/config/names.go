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
	"fmt"
	"sort"
)

// Entry holds the rows BuildNames assembles a template's names from.
type Entry struct {
	// Fields is the entry's own config, with template fields and the
	// variables key already removed.
	Fields map[string]any
	// Variables is the entry's variables map.
	Variables map[string]any
	// Config is the config values a parent hands down.
	Config map[string]any
	// ReadyMade holds values a worker computes in code.
	ReadyMade map[string]any
	// Shortcuts holds extra top-level names a worker declares.
	Shortcuts map[string]any
}

// BuildNames builds the names a template is executed against. Every field,
// ready-made value and shortcut becomes one top-level name, and the
// variables and config sections sit under their own names, empty maps when
// the entry carries none. A ready-made value replaces a field of the same
// name; every other collision between two rows is a *ConfigError naming
// both sources. The result passes through normalize, so json.Marshal always
// accepts it, and the entry is never modified.
func BuildNames(e Entry) (map[string]any, error) {
	// A field or a ready-made value can only collide with another row
	// through one of the two section names, so checking those keys
	// directly keeps the reported error deterministic.
	for _, section := range [...]string{"config", "variables"} {
		if _, ok := e.Fields[section]; ok {
			return nil, namesCollision(section, "a field", "the "+section+" section")
		}
	}

	names := make(map[string]any)
	for k, v := range e.Fields {
		names[k] = normalize(v)
	}
	names["variables"] = normalize(emptyIfNil(e.Variables))
	names["config"] = normalize(emptyIfNil(e.Config))

	for _, section := range [...]string{"config", "variables"} {
		if _, ok := e.ReadyMade[section]; ok {
			return nil, namesCollision(section, "a ready-made value", "the "+section+" section")
		}
	}
	for k, v := range e.ReadyMade {
		names[k] = normalize(v)
	}

	// The shortcut keys are sorted so two colliding shortcuts always
	// produce the same error.
	shortcutKeys := make([]string, 0, len(e.Shortcuts))
	for k := range e.Shortcuts {
		shortcutKeys = append(shortcutKeys, k)
	}
	sort.Strings(shortcutKeys)
	for _, k := range shortcutKeys {
		if _, ok := e.Fields[k]; ok {
			return nil, namesCollision(k, "a shortcut", "a field")
		}
		if _, ok := e.ReadyMade[k]; ok {
			return nil, namesCollision(k, "a shortcut", "a ready-made value")
		}
		if k == "config" || k == "variables" {
			return nil, namesCollision(k, "a shortcut", "the "+k+" section")
		}
		names[k] = normalize(e.Shortcuts[k])
	}

	return names, nil
}

// namesCollision reports a key that two rows claim.
func namesCollision(key, a, b string) error {
	return &ConfigError{
		Field: key,
		Err:   fmt.Errorf("names: %q is both %s and %s", key, a, b),
	}
}

// emptyIfNil returns an empty map in place of a nil one.
func emptyIfNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// VariableShortcuts returns one top-level name per variable, for templates
// that reach a variable without the variables prefix. The result is a
// shallow copy; the input is not modified.
func VariableShortcuts(variables map[string]any) map[string]any {
	names := make(map[string]any, len(variables))
	for k, v := range variables {
		names[k] = v
	}
	return names
}

// HistorianShortcut passes cfgValues["historian"] through as a top-level
// name when present, for templates written as {{ .historian }}. The result
// is empty when the config values carry no historian.
func HistorianShortcut(cfgValues map[string]any) map[string]any {
	names := make(map[string]any)
	if historian, ok := cfgValues["historian"]; ok {
		names["historian"] = historian
	}
	return names
}
