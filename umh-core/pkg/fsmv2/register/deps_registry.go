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

package register

import "sync"

// Package-level typed deps registry consumed by the register.Worker factory
// closure during worker construction. Parent wiring (e.g. cmd/main.go or a
// parent worker constructor) calls SetDeps[TDeps](workerType, deps) before
// factory.NewWorkerByType(workerType, ...) runs; the closure then calls
// GetDeps[TDeps](workerType) and forwards the value to the user-defined
// constructor.
//
// Workers that never call SetDeps (transport/push/pull/persistence — all of
// today's PR2 workers rely on per-package singletons like ChildDeps/Store)
// receive the Go-native zero value of TDeps, preserving the pre-C8 factory
// closure behavior byte-for-byte. This is the core invariant preserving
// backward compatibility for the migrated workers.

var (
	depsRegistryMu sync.RWMutex
	depsRegistry   = map[string]any{}
)

// SetDeps publishes typed deps for workerType. Parent wiring calls this
// before the register.Worker factory closure runs for that type. Thread-safe.
// Overwrites any prior value for the same key.
func SetDeps[TDeps any](workerType string, deps TDeps) {
	depsRegistryMu.Lock()
	defer depsRegistryMu.Unlock()

	depsRegistry[workerType] = deps
}

// GetDeps retrieves typed deps for workerType. Returns the Go-native zero
// value of TDeps when no SetDeps call has happened for the key — intentionally
// `var zero TDeps` rather than reflect.Zero so pointer-TDeps callers see a
// predictable Go-native nil (comparable via `== nil`). This preserves the
// pre-C8 factory closure's zero-value forwarding contract for workers that
// never publish deps.
func GetDeps[TDeps any](workerType string) TDeps {
	depsRegistryMu.RLock()
	defer depsRegistryMu.RUnlock()

	if d, ok := depsRegistry[workerType]; ok {
		return d.(TDeps)
	}

	var zero TDeps

	return zero
}

// ClearDeps removes a single workerType from the registry. Test cleanup hook.
func ClearDeps(workerType string) {
	depsRegistryMu.Lock()
	defer depsRegistryMu.Unlock()

	delete(depsRegistry, workerType)
}

// ResetRegistry clears the entire registry. Test cleanup hook for test files
// that exercise multiple worker types.
func ResetRegistry() {
	depsRegistryMu.Lock()
	defer depsRegistryMu.Unlock()

	depsRegistry = map[string]any{}
}
