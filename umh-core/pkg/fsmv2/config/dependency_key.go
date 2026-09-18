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
type DependencyKey[T any] struct {
	name string
}

// NewDependencyKey returns a key that stores a T under name.
//
// Prefix the name with the worker type, as in "helloworld.filesystem", so two
// workers cannot collide in a map they share.
func NewDependencyKey[T any](name string) DependencyKey[T] {
	return DependencyKey[T]{name: name}
}

// PutDependency stores value under key.
func PutDependency[T any](m map[string]any, key DependencyKey[T], value T) {
	m[key.name] = value
}

// GetDependency reads the value stored under key.
func GetDependency[T any](m map[string]any, key DependencyKey[T]) (T, bool) {
	return m[key.name].(T), true
}
