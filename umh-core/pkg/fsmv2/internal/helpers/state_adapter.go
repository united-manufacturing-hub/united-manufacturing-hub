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

package helpers

import (
	"fmt"
	"reflect"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// TypedSnapshot holds typed observed and desired state for use in state transitions.
// This provides type-safe access to snapshot fields without manual type assertions.
//
// Usage pattern:
//
//	func (s *MyState) Next(snapAny any) (State[any, any], Signal, Action[any]) {
//	    // Single line conversion instead of 5 lines of type assertions
//	    snap := ConvertSnapshot[MyObserved, *MyDesired](snapAny)
//
//	    // Direct typed access - no casting needed
//	    if snap.Desired.IsShutdownRequested() {
//	        return &StoppedState{}, SignalNone, nil
//	    }
//
//	    // Access observed state directly
//	    if snap.Observed.ConnectionStatus == "disconnected" {
//	        return &DisconnectedState{}, SignalNone, nil
//	    }
//
//	    return s, SignalNone, nil
//	}
type TypedSnapshot[O any, D any] struct {
	Observed O
	Desired  D
	Identity fsmv2.Identity
}

// ConvertSnapshot converts an any-typed snapshot to a typed snapshot.
// This function provides a single-line conversion that replaces the boilerplate
// pattern of manual type assertions.
//
// Panics with descriptive message if:
//   - The input is not a Snapshot
//   - Observed is nil or wrong type
//   - Desired is nil or wrong type
//
// The panic behavior is intentional because type mismatches indicate programming
// errors that should fail fast during development/testing, not be silently handled.
//
// Before (5+ lines of boilerplate):
//
//	func (s *MyState) Next(snapAny any) (State[any, any], Signal, Action[any]) {
//	    rawSnap := snapAny.(fsmv2.Snapshot)
//	    snap := MySnapshot{
//	        Identity: rawSnap.Identity,
//	        Observed: rawSnap.Observed.(MyObserved),
//	        Desired:  *rawSnap.Desired.(*MyDesired),
//	    }
//	    // ... state logic
//	}
//
// After (1 line):
//
//	func (s *MyState) Next(snapAny any) (State[any, any], Signal, Action[any]) {
//	    snap := ConvertSnapshot[MyObserved, *MyDesired](snapAny)
//	    // ... state logic with direct typed access
//	}
func ConvertSnapshot[O any, D any](snapAny any) TypedSnapshot[O, D] {
	// Convert to Snapshot type
	raw, ok := snapAny.(fsmv2.Snapshot)
	if !ok {
		actualType := reflect.TypeOf(snapAny)
		panic(fmt.Sprintf("ConvertSnapshot: expected Snapshot, got %v", actualType))
	}

	// Convert Observed with descriptive error
	var observed O
	if raw.Observed == nil {
		var zero O

		expectedType := reflect.TypeOf(zero)
		panic(fmt.Sprintf("ConvertSnapshot: Observed is nil, expected %v", expectedType))
	}

	observed, ok = raw.Observed.(O)
	if !ok {
		// Handle case where O is value type but raw.Observed is pointer type
		// Try to dereference if raw.Observed is a pointer to O
		actualVal := reflect.ValueOf(raw.Observed)
		if actualVal.Kind() == reflect.Ptr && !actualVal.IsNil() {
			elem := actualVal.Elem()
			if elem.Type() == reflect.TypeOf(observed) {
				observed = elem.Interface().(O)
				ok = true
			}
		}

		if !ok {
			actualType := reflect.TypeOf(raw.Observed)

			var zero O

			expectedType := reflect.TypeOf(zero)
			panic(fmt.Sprintf("ConvertSnapshot: Observed type mismatch - expected %v, got %v", expectedType, actualType))
		}
	}

	// Convert Desired with descriptive error
	var desired D
	if raw.Desired == nil {
		var zero D

		expectedType := reflect.TypeOf(zero)
		panic(fmt.Sprintf("ConvertSnapshot: Desired is nil, expected %v", expectedType))
	}

	desired, ok = raw.Desired.(D)
	if !ok {
		actualType := reflect.TypeOf(raw.Desired)

		var zero D

		expectedType := reflect.TypeOf(zero)
		panic(fmt.Sprintf("ConvertSnapshot: Desired type mismatch - expected %v, got %v", expectedType, actualType))
	}

	return TypedSnapshot[O, D]{
		Identity: raw.Identity,
		Observed: observed,
		Desired:  desired,
	}
}
