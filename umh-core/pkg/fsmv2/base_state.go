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
	"reflect"
	"strings"
)

// BaseState provides default implementations for common State interface methods.
// Embed this struct in your state types and use the helper methods to get automatic
// state name derivation from the type name.
//
// Since Go's method promotion doesn't preserve the embedding type's information
// when using reflection on the receiver, embedding types must call StateNameFromType()
// in their String() implementation, passing themselves as the argument.
//
// Usage:
//
//	type RunningState struct {
//	    fsmv2.BaseState
//	}
//
//	func (s RunningState) String() string {
//	    return s.StateNameFromType(s)
//	}
//
//	// String() returns "Running"
//	state := RunningState{}
//	fmt.Println(state.String()) // Output: Running
//
// For simpler usage, you can also use the package-level function:
//
//	func (s RunningState) String() string {
//	    return fsmv2.DeriveStateName(s)
//	}
type BaseState struct{}

// StateNameFromType derives the state name from the given value's type name.
// It strips the "State" suffix if present.
// e.g., RunningState -> "Running", TryingToStartState -> "TryingToStart"
//
// This method should be called from the embedding type's String() method,
// passing the embedding type instance as the argument.
func (b BaseState) StateNameFromType(state interface{}) string {
	return DeriveStateName(state)
}

// DeriveStateName derives a state name from the given value's type name.
// It strips the "State" suffix if present.
// e.g., RunningState -> "Running", TryingToStartState -> "TryingToStart"
//
// This is a package-level function that can be used directly without
// needing a BaseState instance.
func DeriveStateName(state interface{}) string {
	// Get the type of the state
	t := reflect.TypeOf(state)

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Get the type name
	name := t.Name()

	// Strip "State" suffix if present
	name = strings.TrimSuffix(name, "State")

	return name
}

// String returns "BaseState" for direct BaseState usage.
// Embedding types should override this method and call StateNameFromType(self).
func (b BaseState) String() string {
	return "BaseState"
}

// Reason returns the reason for the current state.
// Override this method in your state implementations to provide meaningful reasons.
// By default, returns an empty string.
func (b BaseState) Reason() string {
	return ""
}
