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

import (
	"fmt"
	"sync"
)

// Package-level typed deps registry consumed by the register.Worker factory
// closure during worker construction. Parent wiring (cmd/main.go or a parent
// worker constructor) calls SetGlobalDeps[TDeps](workerType, deps) before
// factory.NewWorkerByType(workerType, ...) runs; the closure then calls
// GlobalDeps[TDeps](workerType) and forwards the value to the user-defined
// constructor.

var (
	depsRegistryMu sync.RWMutex
	depsRegistry   = map[string]any{}
)

// SetGlobalDeps stores deps under workerType and replaces any earlier value.
// Every worker in the process can read it until ClearGlobalDeps or ResetRegistry
// removes it. Safe for concurrent use.
func SetGlobalDeps[TDeps any](workerType string, deps TDeps) {
	depsRegistryMu.Lock()
	defer depsRegistryMu.Unlock()

	depsRegistry[workerType] = deps
}

// GlobalDeps returns the value stored under workerType. When nothing is stored,
// it returns the zero value of TDeps, so a pointer TDeps gets nil.
//
// It panics when the stored value's type is not TDeps.
func GlobalDeps[TDeps any](workerType string) TDeps {
	depsRegistryMu.RLock()
	defer depsRegistryMu.RUnlock()

	if d, ok := depsRegistry[workerType]; ok {
		typed, ok := d.(TDeps)
		if !ok {
			var want TDeps
			panic(fmt.Sprintf("register.GlobalDeps(%q): stored deps have type %T, requested %T - parent wiring published an incompatible value", workerType, d, want))
		}

		return typed
	}

	var zero TDeps

	return zero
}

// ClearGlobalDeps removes the value stored under workerType.
func ClearGlobalDeps(workerType string) {
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
