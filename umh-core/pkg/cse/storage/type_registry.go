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

package storage

import (
	"fmt"
	"reflect"
	"sync"
)

// TypeRegistry stores reflect.Type metadata for worker types.
type TypeRegistry struct {
	mu       sync.RWMutex
	observed map[string]reflect.Type
	desired  map[string]reflect.Type
}

// NewTypeRegistry creates a new type registry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		observed: make(map[string]reflect.Type),
		desired:  make(map[string]reflect.Type),
	}
}

// RegisterWorkerType stores type metadata for a worker.
func (r *TypeRegistry) RegisterWorkerType(workerType string, observedType, desiredType reflect.Type) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.observed[workerType]; exists {
		return fmt.Errorf("worker type %q already registered", workerType)
	}

	r.observed[workerType] = observedType
	r.desired[workerType] = desiredType

	return nil
}

// GetObservedType retrieves observed type for a worker.
func (r *TypeRegistry) GetObservedType(workerType string) reflect.Type {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.observed[workerType]
}

// GetDesiredType retrieves desired type for a worker.
func (r *TypeRegistry) GetDesiredType(workerType string) reflect.Type {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.desired[workerType]
}
