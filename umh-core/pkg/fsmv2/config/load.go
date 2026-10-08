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
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadUserSpec decodes spec.Config into T and rejects a placeholder outside a
// Template field. It does not render; a worker that wants its Template fields
// rendered calls RenderTemplates after it, with the names from BuildNames. A
// nil spec or an empty config yields the zero value of T. A spec that is not a
// UserSpec is a plain error, because it is a programming mistake. Every other
// error is a *ConfigError; a decode failure keeps yaml's own text.
func LoadUserSpec[T any](spec any) (T, error) {
	var zero T

	if spec == nil {
		return zero, nil
	}

	userSpec, ok := spec.(UserSpec)
	if !ok {
		return zero, fmt.Errorf("invalid spec type: expected config.UserSpec, got %T", spec)
	}

	var result T
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &result); err != nil {
			return zero, &ConfigError{Err: err}
		}
	}

	if err := CheckPlainValues(&result, userSpec.Variables.User); err != nil {
		return zero, err
	}

	return result, nil
}

// EntryFields decodes configYAML into the fields a template's names are built
// from: the config's own values as a map, with the variables section, the
// key of every Template field and every field the typed plain-value walk
// exempts removed, because BuildNames supplies those names itself and the
// exempt fields decode their own text. cfg is the struct the same YAML
// decodes into; its Template fields are found by reflection, honouring an
// inline embed. The decoded map is normalized, so a Template key behind a
// map yaml decoded with a non-string key is removed too. A value left in
// the map that still contains a placeholder is a *ConfigError naming its
// path: yaml.Unmarshal into the typed struct drops keys no field declares,
// so CheckPlainValues in LoadUserSpec never runs on them.
func EntryFields(configYAML string, cfg any) (map[string]any, error) {
	fields := map[string]any{}
	if configYAML != "" {
		if err := yaml.Unmarshal([]byte(configYAML), &fields); err != nil {
			return nil, &ConfigError{Err: err}
		}

		fields = normalize(fields).(map[string]any)
	}

	delete(fields, "variables")
	for _, path := range templatePaths(reflect.ValueOf(cfg)) {
		deleteField(fields, path)
	}

	for _, path := range exemptPaths(reflect.ValueOf(cfg)) {
		deleteField(fields, path)
	}

	if err := checkPlainValue(reflect.ValueOf(fields), ""); err != nil {
		return nil, err
	}

	return fields, nil
}

// templatePaths returns the YAML path of every Template field of the value v
// points to, as segments in the same form the walkers use: an
// inline-embedded struct adds no segment.
func templatePaths(v reflect.Value) [][]string {
	return leafPaths(v, func(t reflect.Type) bool {
		return t == templateType
	})
}

// exemptPaths returns the YAML path of every field of the value v points to
// whose type the plain-value walk skips: a UserSpec, or a type that decodes
// itself in yaml.v3 and may legitimately carry placeholder text its own
// decoding consumes.
func exemptPaths(v reflect.Value) [][]string {
	return leafPaths(v, func(t reflect.Type) bool {
		return t == userSpecType || isYAMLUnmarshaler(t)
	})
}

// leafPaths returns the YAML path of every leaf-matching field of the value v
// points to, as segments in the same form the walkers use: an
// inline-embedded struct adds no segment.
func leafPaths(v reflect.Value, leaf func(reflect.Type) bool) [][]string {
	var paths [][]string

	var walk func(sv reflect.Value, prefix []string)
	walk = func(sv reflect.Value, prefix []string) {
		for sv.Kind() == reflect.Pointer || sv.Kind() == reflect.Interface {
			if sv.IsNil() {
				return
			}

			sv = sv.Elem()
		}

		if !sv.IsValid() {
			return
		}

		if leaf(sv.Type()) {
			if len(prefix) > 0 {
				paths = append(paths, prefix)
			}

			return
		}

		switch sv.Kind() {
		case reflect.Struct:
			for i := range sv.NumField() {
				sf := sv.Type().Field(i)
				if sf.PkgPath != "" {
					continue
				}

				child := prefix
				if !isInlineField(sf) {
					child = appendSegment(prefix, yamlFieldName(sf))
				}
				walk(sv.Field(i), child)
			}
		case reflect.Slice, reflect.Array:
			for i := range sv.Len() {
				walk(sv.Index(i), appendSegment(prefix, strconv.Itoa(i)))
			}
		case reflect.Map:
			for _, key := range sv.MapKeys() {
				walk(sv.MapIndex(key), appendSegment(prefix, fmt.Sprint(key.Interface())))
			}
		}
	}

	walk(v, nil)
	return paths
}

// appendSegment extends path by one segment without sharing the parent's
// backing array, so sibling branches cannot overwrite each other.
func appendSegment(path []string, segment string) []string {
	child := make([]string, len(path), len(path)+1)
	copy(child, path)

	return append(child, segment)
}

// isInlineField reports whether the struct field merges into its parent in
// YAML.
func isInlineField(sf reflect.StructField) bool {
	return strings.Contains(sf.Tag.Get("yaml"), ",inline")
}

// deleteField removes the value at path from a decoded config map. A path the
// config does not carry, or one that runs through a value that is not a
// container, deletes nothing. A slice element cannot be deleted from the
// middle, so its slot is emptied; the plain-value walk skips the invalid
// value left there.
func deleteField(fields map[string]any, path []string) {
	current := any(fields)
	for _, segment := range path[:len(path)-1] {
		current = mapSegment(current, segment)
		if current == nil {
			return
		}
	}

	last := path[len(path)-1]

	if m, ok := current.(map[string]any); ok {
		delete(m, last)

		return
	}

	if s, ok := current.([]any); ok {
		if index, err := strconv.Atoi(last); err == nil && index >= 0 && index < len(s) {
			s[index] = nil
		}
	}
}

// mapSegment descends one path segment into a decoded config map. The tree
// EntryFields walks is normalized first, so every container is a
// map[string]any or a []any.
func mapSegment(v any, segment string) any {
	switch container := v.(type) {
	case map[string]any:
		return container[segment]
	case []any:
		index, err := strconv.Atoi(segment)
		if err != nil || index < 0 || index >= len(container) {
			return nil
		}

		return container[index]
	default:
		return nil
	}
}
