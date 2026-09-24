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
// The map itself stays map[string]any because it crosses the supervisor and the
// worker factory, and neither of those knows what any given worker's
// dependencies are. The key carries the type instead, so the side that writes
// and the side that reads agree without either one writing a type assertion.
//
// Declare one as a package-level var next to the worker that reads it:
//
//	var FilesystemKey = config.NewDependencyKey[filesystem.Service]("helloworld.filesystem")
//
// One name has one type: the side that writes and the side that reads share
// the same key, and NewDependencyKey panics on a second type.
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

// SetDependency stores value under key. m must be non-nil, as for any map
// assignment.
func SetDependency[T any](m map[string]any, key DependencyKey[T], value T) {
	m[key.name] = value
}

// LookupDependency reads the value stored under key. The second return is false
// when the map holds nothing under that name, and also when it holds something
// of another type.
//
// A mismatched type reads as absent rather than panicking so that a worker
// whose dependency was wired up wrongly stays on its real implementation. The
// alternative is a panic in front of whoever is running the process.
func LookupDependency[T any](m map[string]any, key DependencyKey[T]) (T, bool) {
	// A name the map does not hold reads as a nil any, which fails this
	// assertion the same way a wrong type does. Both answers are the same to a
	// caller, so neither needs its own branch.
	value, ok := m[key.name].(T)

	return value, ok
}
