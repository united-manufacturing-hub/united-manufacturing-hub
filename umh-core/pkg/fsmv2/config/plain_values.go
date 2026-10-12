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
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	userSpecType        = reflect.TypeOf(UserSpec{})
	yamlUnmarshalerType = reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()
	errPlaceholder      = errors.New("placeholders are only allowed in template fields")
)

// CheckPlainValues reports the first field of v, then of variables, whose
// value still contains a placeholder (a Go template action, {{ ... }}).
// Struct fields are visited in declaration order and map keys in sorted
// order, so the reported field is deterministic. A value outside a Template
// field is never rendered, so a placeholder there would reach the worker
// unresolved. Only a placeholder is rejected: benthos evaluates its own
// ${! ...} interpolation, and any other dollar text passes through
// unchanged. Each field is named by its YAML path, the variables map under
// the "variables" prefix.
func CheckPlainValues(v any, variables map[string]any) error {
	if err := checkPlainValue(reflect.ValueOf(v), ""); err != nil {
		return err
	}

	return checkPlainValue(reflect.ValueOf(variables), "variables")
}

// CheckConfigValues reports the first entry of values whose value still
// contains a placeholder (a Go template action, {{ ... }}). Map keys are
// visited in sorted order, so the reported field is deterministic. Each
// entry is named by its path under the "config" prefix, which is the
// prefix a template reads these values under ({{ .config.* }}). These
// values are substituted into templates but never rendered themselves, so
// a placeholder in one would reach a child unresolved.
func CheckConfigValues(values map[string]any) error {
	return checkPlainValue(reflect.ValueOf(values), "config")
}

func checkPlainValue(sv reflect.Value, path string) error {
	for sv.Kind() == reflect.Pointer || sv.Kind() == reflect.Interface {
		if sv.IsNil() {
			return nil
		}

		sv = sv.Elem()
	}

	if !sv.IsValid() {
		return nil
	}

	// Exempt leaves: Template fields render later; a UserSpec and a type
	// with its own UnmarshalYAML consume their own text. EntryFields removes
	// the same paths from the raw config map, so CheckPlainValues on the
	// typed config and EntryFields on the raw map judge the same values.
	if sv.Type() == templateType || sv.Type() == userSpecType || isYAMLUnmarshaler(sv.Type()) {
		return nil
	}

	switch sv.Kind() {
	case reflect.String:
		if strings.Contains(sv.String(), "{{") {
			return &ConfigError{Field: path, Err: errPlaceholder}
		}
	case reflect.Struct:
		for i := range sv.NumField() {
			sf := sv.Type().Field(i)
			if sf.PkgPath != "" {
				continue
			}

			if name, _, _ := strings.Cut(sf.Tag.Get("yaml"), ","); name == "-" {
				continue
			}

			if err := checkPlainValue(sv.Field(i), childPrefix(path, sf)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range sv.Len() {
			if err := checkPlainValue(sv.Index(i), joinPath(path, strconv.Itoa(i))); err != nil {
				return err
			}
		}
	case reflect.Map:
		keys := sv.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})

		for _, key := range keys {
			if err := checkPlainValue(sv.MapIndex(key), joinPath(path, fmt.Sprint(key.Interface()))); err != nil {
				return err
			}
		}
	}

	return nil
}

// isYAMLUnmarshaler reports whether t, or a pointer to it, decodes itself in
// yaml.v3 through an UnmarshalYAML method.
func isYAMLUnmarshaler(t reflect.Type) bool {
	return t.Implements(yamlUnmarshalerType) ||
		reflect.PointerTo(t).Implements(yamlUnmarshalerType)
}
