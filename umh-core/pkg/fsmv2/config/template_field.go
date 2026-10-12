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
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"
)

// Template is a config field whose text is a Go template. It marshals and
// unmarshals as a plain string in YAML and JSON.
type Template string

// ConfigError reports that a config field failed to load or render.
// Field is the field's YAML path.
type ConfigError struct {
	Field string
	Err   error
}

func (e *ConfigError) Error() string {
	if e.Err == nil {
		return e.Field
	}
	if e.Field == "" {
		return e.Err.Error()
	}
	return e.Field + ": " + e.Err.Error()
}

func (e *ConfigError) Unwrap() error { return e.Err }

// Render executes the template against names in strict mode (a missing key
// fails). Every error is a *ConfigError naming the field, with Go's original
// error text kept.
func (t Template) Render(field string, names map[string]any) (string, error) {
	parsed, err := template.New("config").
		Option("missingkey=error").
		Funcs(template.FuncMap{"printable": printable}).
		Parse(string(t))
	if err != nil {
		return "", &ConfigError{Field: field, Err: err}
	}

	addPrintChecks(parsed)

	var buf bytes.Buffer
	if err := parsed.Execute(&buf, normalize(names)); err != nil {
		return "", &ConfigError{Field: field, Err: err}
	}

	return buf.String(), nil
}

// printable guards what a print action emits. Every value it rejects has no
// meaningful string form, so it fails instead of letting the template print
// Go's default representation.
func printable(v any) (any, error) {
	if v == nil {
		return nil, errors.New("template printed nil")
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return nil, errors.New("template printed a map or list; print a single value instead")
	default:
		return v, nil
	}
}

// addPrintChecks appends printable to every print action in tmpl's parse
// tree, so each printed value passes through it. The bodies of templates
// that define and block create are associated trees, so they are walked too.
// The pipelines of if, range and with judge truth and are left alone, and so
// are declarations and assignments, which print nothing. printable sees
// only the final value of a pipeline, so a print or printf earlier in the
// pipeline can stringify a value first.
func addPrintChecks(tmpl *template.Template) {
	addPrintChecksToNode(tmpl.Tree.Root)
	for _, assoc := range tmpl.Templates() {
		if assoc.Tree != tmpl.Tree {
			addPrintChecksToNode(assoc.Tree.Root)
		}
	}
}

func addPrintChecksToNode(node parse.Node) {
	switch n := node.(type) {
	case *parse.ListNode:
		for _, child := range n.Nodes {
			addPrintChecksToNode(child)
		}
	case *parse.ActionNode:
		// A declaration ({{ $c := .conn }}) and an assignment ({{ $c = .conn }})
		// print nothing, so neither is checked.
		if !n.Pipe.IsAssign && len(n.Pipe.Decl) == 0 {
			cmd := &parse.CommandNode{
				NodeType: parse.NodeCommand,
				Args:     []parse.Node{parse.NewIdentifier("printable")},
			}
			n.Pipe.Cmds = append(n.Pipe.Cmds, cmd)
		}
	case *parse.IfNode:
		addPrintChecksToBranch(n.List, n.ElseList)
	case *parse.RangeNode:
		addPrintChecksToBranch(n.List, n.ElseList)
	case *parse.WithNode:
		addPrintChecksToBranch(n.List, n.ElseList)
	}
}

func addPrintChecksToBranch(list, elseList *parse.ListNode) {
	if list != nil {
		addPrintChecksToNode(list)
	}
	if elseList != nil {
		addPrintChecksToNode(elseList)
	}
}

// normalize rebuilds v for template execution: a whole float64 within int64
// range becomes an int64 (a JSON round trip turns 1000000 into 1e+06
// otherwise; outside the range the conversion is undefined and even
// platform-dependent, so the float renders its own form), and the two map
// types yaml.v3 and JSON decoding produce become keyed by string (yaml.v3
// decodes `1: hall` with an int key, which index cannot reach with a
// string). The input is never modified; all containers are rebuilt.
func normalize(v any) any {
	switch x := v.(type) {
	case float64:
		if x == math.Trunc(x) && !math.IsInf(x, 0) && x >= math.MinInt64 && x < 9223372036854775808 {
			return int64(x)
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalize(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprint(k)] = normalize(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalize(val)
		}
		return out
	default:
		return v
	}
}

var templateType = reflect.TypeOf(Template(""))

// RenderTemplates renders every Template field of the struct v points to and
// writes the results back. Each field renders into a pending record first,
// so either every Template field is updated or none is. Each field is named
// by its YAML path, so a Template behind an inline-embedded struct reports
// the inner YAML name. A Template the walk cannot write back, such as one
// inside a map where values are not settable, is an error, never a silent
// skip.
func RenderTemplates(v any, names map[string]any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("RenderTemplates requires a non-nil pointer to a struct, got %T", v)
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("RenderTemplates requires a pointer to a struct, got pointer to %s", rv.Kind())
	}

	type pendingRender struct {
		field    reflect.Value
		path     string
		rendered string
	}
	var pending []pendingRender

	var walk func(sv reflect.Value, prefix string) error
	walk = func(sv reflect.Value, prefix string) error {
		if sv.Kind() == reflect.Pointer {
			if sv.IsNil() {
				return nil
			}
			return walk(sv.Elem(), prefix)
		}
		if sv.Kind() == reflect.Interface {
			if sv.IsNil() {
				return nil
			}
			return walk(sv.Elem(), prefix)
		}
		if sv.Type() == templateType {
			if !sv.CanSet() {
				return &ConfigError{
					Field: prefix,
					Err:   errors.New("template field cannot be written back (map values and copies are not settable)"),
				}
			}
			pending = append(pending, pendingRender{field: sv, path: prefix})
			return nil
		}
		switch sv.Kind() {
		case reflect.Struct:
			for i := range sv.NumField() {
				sf := sv.Type().Field(i)
				if sf.PkgPath != "" {
					continue
				}
				if err := walk(sv.Field(i), childPrefix(prefix, sf)); err != nil {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			for i := range sv.Len() {
				if err := walk(sv.Index(i), joinPath(prefix, strconv.Itoa(i))); err != nil {
					return err
				}
			}
		case reflect.Map:
			keys := sv.MapKeys()
			sort.Slice(keys, func(i, j int) bool {
				return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
			})
			for _, key := range keys {
				if err := walk(sv.MapIndex(key), joinPath(prefix, fmt.Sprint(key.Interface()))); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(rv, ""); err != nil {
		return err
	}

	for i := range pending {
		rendered, err := pending[i].field.Interface().(Template).Render(pending[i].path, names)
		if err != nil {
			return err
		}
		pending[i].rendered = rendered
	}
	for i := range pending {
		pending[i].field.SetString(pending[i].rendered)
	}

	return nil
}

// childPrefix extends prefix for a nested struct: an inline-embedded struct
// merges into its parent, so its YAML path adds nothing.
func childPrefix(prefix string, sf reflect.StructField) string {
	if isInlineField(sf) {
		return prefix
	}
	return joinPath(prefix, yamlFieldName(sf))
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// yamlFieldName returns the name yaml.v3 uses for the field: the yaml tag's
// name, else the lower-cased Go field name. A field tagged yaml:"-" never
// appears in YAML; RenderTemplates still renders it under its Go name.
func yamlFieldName(sf reflect.StructField) string {
	if name, _, _ := strings.Cut(sf.Tag.Get("yaml"), ","); name != "" && name != "-" {
		return name
	}
	return strings.ToLower(sf.Name)
}
