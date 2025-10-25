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

package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

// TriangularStoreAdapter wraps storage.TriangularStore to implement persistence.Store.
//
// DESIGN DECISION: Adapter pattern instead of modifying TriangularStore
// WHY: TriangularStore is designed for document-based storage (basic.Document),
// while FSM v2 needs typed interfaces (fsmv2.DesiredState, fsmv2.ObservedState).
// This adapter bridges the gap without requiring TriangularStore to know about FSM v2 types.
//
// TRADE-OFF: Additional abstraction layer, but preserves separation of concerns.
// TriangularStore remains generic, FSM v2 gets type-safe API.
//
// Type Conversion Strategy:
//   - ToDocument/FromDocument helpers handle struct ↔ basic.Document conversion
//   - Reflection used to deserialize documents back to typed states
//   - Type registry maps workerType to concrete state types
//
// INSPIRED BY: Adapter pattern, Linear ENG-3622 (TriangularStore for FSM persistence),
// Original design intent from prototype in commit 567a1f8
type TriangularStoreAdapter struct {
	triangular *storage.TriangularStore
	// Type registry for deserializing documents back to typed states
	// Maps workerType → reflect.Type
	desiredTypes  map[string]reflect.Type
	observedTypes map[string]reflect.Type
	identityTypes map[string]reflect.Type
}

// NewTriangularStoreAdapter creates adapter for given TriangularStore.
//
// Type Registration Pattern:
// The adapter needs to know which concrete types to deserialize documents into.
// Call RegisterTypes() after creation to register worker-specific types:
//
//	adapter := NewTriangularStoreAdapter(triangular)
//	adapter.RegisterTypes("container",
//	    reflect.TypeOf(ContainerDesiredState{}),
//	    reflect.TypeOf(ContainerObservedState{}),
//	    reflect.TypeOf(fsmv2.Identity{}))
//
// This is similar to how supervisor.TypeRegistry works.
func NewTriangularStoreAdapter(triangular *storage.TriangularStore) *TriangularStoreAdapter {
	return &TriangularStoreAdapter{
		triangular:    triangular,
		desiredTypes:  make(map[string]reflect.Type),
		observedTypes: make(map[string]reflect.Type),
		identityTypes: make(map[string]reflect.Type),
	}
}

// RegisterTypes registers concrete types for deserialization.
// Must be called before Load operations for each workerType.
func (a *TriangularStoreAdapter) RegisterTypes(
	workerType string,
	desiredType reflect.Type,
	observedType reflect.Type,
	identityType reflect.Type,
) {
	a.desiredTypes[workerType] = desiredType
	a.observedTypes[workerType] = observedType
	a.identityTypes[workerType] = identityType
}

// SaveIdentity persists the immutable identity of a worker.
func (a *TriangularStoreAdapter) SaveIdentity(ctx context.Context, workerType string, id string, data interface{}) error {
	doc, err := toDocument(data)
	if err != nil {
		return fmt.Errorf("failed to serialize identity: %w", err)
	}
	return a.triangular.SaveIdentity(ctx, workerType, id, doc)
}

// LoadIdentity retrieves the identity of a worker.
func (a *TriangularStoreAdapter) LoadIdentity(ctx context.Context, workerType string, id string) (interface{}, error) {
	doc, err := a.triangular.LoadIdentity(ctx, workerType, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	// Get registered type
	identityType, ok := a.identityTypes[workerType]
	if !ok {
		// Default to fsmv2.Identity if not registered
		identityType = reflect.TypeOf(fsmv2.Identity{})
	}

	identity, err := fromDocument(doc, identityType)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize identity: %w", err)
	}
	return identity, nil
}

// SaveDesired persists the desired state (user intent).
func (a *TriangularStoreAdapter) SaveDesired(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
	doc, err := toDocument(desired)
	if err != nil {
		return fmt.Errorf("failed to serialize desired state: %w", err)
	}
	return a.triangular.SaveDesired(ctx, workerType, id, doc)
}

// LoadDesired retrieves the desired state.
func (a *TriangularStoreAdapter) LoadDesired(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error) {
	doc, err := a.triangular.LoadDesired(ctx, workerType, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	// Get registered type
	desiredType, ok := a.desiredTypes[workerType]
	if !ok {
		return nil, fmt.Errorf("no desired type registered for workerType %q", workerType)
	}

	desired, err := fromDocument(doc, desiredType)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize desired state: %w", err)
	}

	// Type assertion to fsmv2.DesiredState
	desiredState, ok := desired.(fsmv2.DesiredState)
	if !ok {
		return nil, fmt.Errorf("deserialized type does not implement fsmv2.DesiredState")
	}

	return desiredState, nil
}

// SaveObserved persists the observed state (system reality).
func (a *TriangularStoreAdapter) SaveObserved(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error {
	doc, err := toDocument(observed)
	if err != nil {
		return fmt.Errorf("failed to serialize observed state: %w", err)
	}
	return a.triangular.SaveObserved(ctx, workerType, id, doc)
}

// LoadObserved retrieves the observed state.
func (a *TriangularStoreAdapter) LoadObserved(ctx context.Context, workerType string, id string) (fsmv2.ObservedState, error) {
	doc, err := a.triangular.LoadObserved(ctx, workerType, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	// Get registered type
	observedType, ok := a.observedTypes[workerType]
	if !ok {
		return nil, fmt.Errorf("no observed type registered for workerType %q", workerType)
	}

	observed, err := fromDocument(doc, observedType)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize observed state: %w", err)
	}

	// Type assertion to fsmv2.ObservedState
	observedState, ok := observed.(fsmv2.ObservedState)
	if !ok {
		return nil, fmt.Errorf("deserialized type does not implement fsmv2.ObservedState")
	}

	return observedState, nil
}

// LoadSnapshot retrieves a complete snapshot by joining identity, desired, and observed.
func (a *TriangularStoreAdapter) LoadSnapshot(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
	// Load all three parts using TriangularStore
	snapshot, err := a.triangular.LoadSnapshot(ctx, workerType, id)
	if err != nil {
		return nil, err
	}

	// Convert documents to typed states
	identity, err := a.deserializeIdentity(workerType, snapshot.Identity)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize identity: %w", err)
	}

	desired, err := a.deserializeDesired(workerType, snapshot.Desired)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize desired: %w", err)
	}

	observed, err := a.deserializeObserved(workerType, snapshot.Observed)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize observed: %w", err)
	}

	// Construct fsmv2.Snapshot
	return &fsmv2.Snapshot{
		Identity: identity,
		Observed: observed,
		Desired:  desired,
	}, nil
}

// deserializeIdentity converts basic.Document to fsmv2.Identity
func (a *TriangularStoreAdapter) deserializeIdentity(workerType string, doc basic.Document) (fsmv2.Identity, error) {
	identityType, ok := a.identityTypes[workerType]
	if !ok {
		// Default to fsmv2.Identity
		identityType = reflect.TypeOf(fsmv2.Identity{})
	}

	result, err := fromDocument(doc, identityType)
	if err != nil {
		return fsmv2.Identity{}, err
	}

	// If it's already fsmv2.Identity, return it
	if identity, ok := result.(fsmv2.Identity); ok {
		return identity, nil
	}

	// Otherwise, extract fields manually
	// This handles cases where a custom identity type is registered
	return fsmv2.Identity{
		ID:         doc["id"].(string),
		Name:       doc["name"].(string),
		WorkerType: workerType,
	}, nil
}

// deserializeDesired converts basic.Document to fsmv2.DesiredState
func (a *TriangularStoreAdapter) deserializeDesired(workerType string, doc basic.Document) (fsmv2.DesiredState, error) {
	desiredType, ok := a.desiredTypes[workerType]
	if !ok {
		return nil, fmt.Errorf("no desired type registered for workerType %q", workerType)
	}

	result, err := fromDocument(doc, desiredType)
	if err != nil {
		return nil, err
	}

	desired, ok := result.(fsmv2.DesiredState)
	if !ok {
		return nil, fmt.Errorf("deserialized type does not implement fsmv2.DesiredState")
	}

	return desired, nil
}

// deserializeObserved converts basic.Document to fsmv2.ObservedState
func (a *TriangularStoreAdapter) deserializeObserved(workerType string, doc basic.Document) (fsmv2.ObservedState, error) {
	observedType, ok := a.observedTypes[workerType]
	if !ok {
		return nil, fmt.Errorf("no observed type registered for workerType %q", workerType)
	}

	result, err := fromDocument(doc, observedType)
	if err != nil {
		return nil, err
	}

	observed, ok := result.(fsmv2.ObservedState)
	if !ok {
		return nil, fmt.Errorf("deserialized type does not implement fsmv2.ObservedState")
	}

	return observed, nil
}

// GetLastSyncID returns the current global sync version.
func (a *TriangularStoreAdapter) GetLastSyncID(ctx context.Context) (int64, error) {
	return a.triangular.GetLastSyncID(ctx)
}

// IncrementSyncID atomically increments and returns the new global sync version.
func (a *TriangularStoreAdapter) IncrementSyncID(ctx context.Context) (int64, error) {
	return a.triangular.IncrementSyncID(ctx)
}

// Close closes the store and releases resources.
func (a *TriangularStoreAdapter) Close() error {
	return a.triangular.Close()
}

// toDocument converts a Go struct to a basic.Document using JSON marshaling.
//
// DESIGN DECISION: Use JSON directly instead of mapstructure
// WHY: BasicStore marshals Documents to JSON anyway, so mapstructure is
// an unnecessary intermediate step that complicates time.Time handling.
// Direct JSON marshaling handles all Go types correctly including time.Time.
func toDocument(v interface{}) (basic.Document, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	var doc basic.Document
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to document: %w", err)
	}

	return doc, nil
}

// fromDocument converts a basic.Document to a typed Go struct using JSON marshaling.
//
// DESIGN DECISION: Use JSON directly instead of mapstructure
// WHY: Consistent with toDocument(), handles all types correctly,
// eliminates reflection complexity and custom hooks.
//
// Handles both value types and pointer types:
// - If targetType is a value type (e.g., TestDesiredState), creates pointer,
//   decodes, and returns dereferenced value.
// - If targetType is a pointer type (e.g., *ContainerDesiredState), creates
//   instance and returns pointer directly.
func fromDocument(doc basic.Document, targetType reflect.Type) (interface{}, error) {
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal document to JSON: %w", err)
	}

	var result interface{}
	if targetType.Kind() == reflect.Ptr {
		result = reflect.New(targetType.Elem()).Interface()
	} else {
		result = reflect.New(targetType).Interface()
	}

	if err := json.Unmarshal(jsonBytes, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to %s: %w", targetType.Name(), err)
	}

	if targetType.Kind() == reflect.Ptr {
		return result, nil
	}
	return reflect.ValueOf(result).Elem().Interface(), nil
}
