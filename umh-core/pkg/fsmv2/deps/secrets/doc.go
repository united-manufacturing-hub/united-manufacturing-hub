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

// Package secrets holds a store for secret entries, keyed by the hierarchy
// path of the worker that owns them.
//
// Usage: a worker that stores or reads a secret writes three steps. First,
// in its constructor, it looks up the store's handle in its dependency map:
//
//	handle, ok := config.LookupDependency(dependencies, secrets.StoreKey)
//
// The lookup returns the handle, or false when the dependency map holds
// nothing under the key. Second, it opens its scope with
// its *deps.BaseDependencies, which carry its hierarchy path:
//
//	scope, err := secrets.ScopeOf(handle, bd)
//
// ScopeOf returns the worker's own scope, or an error and no usable scope
// when the handle has no store behind it or the path is empty or malformed.
// Third, it calls Get, Put or Delete on that scope. Each call takes an entry
// name and no path: Put stores a Value under the name, Get returns the
// stored Value or ErrNotFound, Delete removes the entry. Each of them
// returns ErrInvalidName when the name is not valid.
//
// Use the worker's own scope for entries this worker owns. Use the scope
// that Ancestor(workerType) returns for an entry a worker above it owns:
//
//	transport, ok := scope.Ancestor("transport")
//
// Ancestor returns the scope of the nearest ancestor whose type part equals
// workerType, and true, or false when no ancestor has that type. An
// ancestor scope can read, write and delete that ancestor's entries.
//
// For example, the transport worker could keep its login token in the store,
// and its push child could read it:
//
//	// transport worker
//	scope.Put("jwt", secrets.NewValue(token))
//
//	// push worker, a child of the transport worker
//	transport, ok := scope.Ancestor("transport")
//	jwt, err := transport.Get("jwt")
//
// The model in one place: one store per process. cmd/main.go creates the
// store and puts its handle in the root dependencies under StoreKey, so
// every worker receives it like any other dependency. A worker opens its
// scope with ScopeOf in its constructor. Its own scope holds the entries of
// its own hierarchy path, and Ancestor(type) returns the scope of the
// nearest ancestor of that type. A worker that passes its own BaseDependencies
// reaches only its own entries and its ancestors' entries. ScopeOf does
// not check who calls it, so code that passes another worker's BaseDependencies,
// or builds its own path source, gets that path's scope. A Value never
// prints the string it holds: it prints [secret] and returns an error to
// JSON and text encoders.
//
// A worker constructor that opens its scope:
//
//	func newTransportWorker(dependencies map[string]any, bd *deps.BaseDependencies) (*transportWorker, error) {
//		handle, ok := config.LookupDependency(dependencies, secrets.StoreKey)
//		if !ok {
//			return nil, errors.New("no secrets store in the dependencies")
//		}
//		scope, err := secrets.ScopeOf(handle, bd)
//		if err != nil {
//			return nil, err
//		}
//		return &transportWorker{scope: scope}, nil
//	}
//
// A worker's hierarchy path is segments name(type) joined by "/". A root
// application worker sits at <ID>(application), and each child adds a
// segment <name>-001(<type>), so a transport child of the communicator sits
// at
// application-fsmv2(application)/communicator-001(communicator)/transport-001(transport).
// Ancestor reads only the type part of each segment, the text inside the
// last pair of parentheses.
package secrets
