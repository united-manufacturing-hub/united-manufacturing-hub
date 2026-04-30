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

package fsmv2

import (
	"fmt"
	"sync"
)

var (
	initialStateRegistry   = make(map[string]State[any, any])
	initialStateRegistryMu sync.RWMutex
)

// RegisterInitialState registers the initial state for a worker type.
// Called from state package init() functions. Panics on duplicate registration.
// The registered state is shared across all worker instances of this type —
// it must be stateless (no mutable fields, per architecture test "Empty State Structs").
func RegisterInitialState(workerType string, state State[any, any]) {
	if workerType == "" {
		panic("RegisterInitialState: workerType must not be empty")
	}
	if state == nil {
		panic(fmt.Sprintf("RegisterInitialState(%q): state must not be nil", workerType))
	}
	initialStateRegistryMu.Lock()
	defer initialStateRegistryMu.Unlock()
	if _, exists := initialStateRegistry[workerType]; exists {
		panic(fmt.Sprintf("RegisterInitialState: duplicate registration for %q", workerType))
	}
	initialStateRegistry[workerType] = state
}

// LookupInitialState returns the registered initial state for a worker type.
// Returns nil if no state is registered.
func LookupInitialState(workerType string) State[any, any] {
	initialStateRegistryMu.RLock()
	defer initialStateRegistryMu.RUnlock()
	return initialStateRegistry[workerType]
}

// ResetInitialStateRegistry clears all registrations. For testing only.
func ResetInitialStateRegistry() {
	initialStateRegistryMu.Lock()
	defer initialStateRegistryMu.Unlock()
	initialStateRegistry = make(map[string]State[any, any])
}
