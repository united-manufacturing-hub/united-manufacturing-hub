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
	"sync"
)

// dependencyKeyTypes maps each declared key name to the type it was first
// declared with.
var dependencyKeyTypes sync.Map

// DependencyKey names one entry in a dependency map and records the type stored
// under it.
//
// The map is map[string]any because the supervisor and the worker factory pass
// it on without knowing any worker's dependency types. The key carries the type,
// so neither the writer nor the reader writes a type assertion.
//
// Declare one as a package-level var next to the worker that reads it:
//
//	var FilesystemKey = config.NewDependencyKey[filesystem.Service]("helloworld.filesystem")
type DependencyKey[T any] struct {
	name string
}

// NewDependencyKey returns a key that stores a T under name.
//
// Prefix the name with the worker type, as in "helloworld.filesystem", so two
// workers cannot collide in a map they share.
//
// Declaring a name again with a different type panics, because a value stored
// under one of the two keys would read as absent through the other.
func NewDependencyKey[T any](name string) DependencyKey[T] {
	t := reflect.TypeFor[T]()
	if prev, loaded := dependencyKeyTypes.LoadOrStore(name, t); loaded && prev != t {
		panic(fmt.Sprintf("config.NewDependencyKey(%q): already declared with type %s, now %s", name, prev, t))
	}

	return DependencyKey[T]{name: name}
}

// SetDependency stores value under key. A nil value panics: a nil pointer or map
// would read as present through LookupDependency, and the worker would call it
// instead of falling back to its real implementation.
func SetDependency[T any](m map[string]any, key DependencyKey[T], value T) {
	if isNil(value) {
		panic(fmt.Sprintf("config.SetDependency(%q): value is nil", key.name))
	}

	m[key.name] = value
}

// isNil reports whether value is nil. Unlike value == nil, it also catches a nil
// pointer, map or other nilable value held in an interface.
func isNil(value any) bool {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return true
	}

	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// LookupDependency reads the value stored under key. The second return is false
// when the map holds nothing under that name, holds a value of another type, or
// holds a nil value, so a worker whose dependency was wired up wrongly stays on
// its real implementation.
func LookupDependency[T any](m map[string]any, key DependencyKey[T]) (T, bool) {
	// A missing name reads as a nil any, which fails this assertion too.
	value, ok := m[key.name].(T)

	return value, ok && !isNil(value)
}
