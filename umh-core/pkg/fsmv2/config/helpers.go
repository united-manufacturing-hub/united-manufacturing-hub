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

	"gopkg.in/yaml.v3"
)

// StateGetter is implemented by user spec types that embed BaseUserSpec.
// It allows DeriveLeafState to extract the desired state from parsed config.
type StateGetter interface {
	GetState() string
}

// ParseUserSpec provides type-safe parsing of UserSpec.Config into a typed struct.
// This eliminates boilerplate in DeriveDesiredState implementations.
//
// Usage:
//
//	func (w *MyWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
//	    parsed, err := config.ParseUserSpec[MyUserSpec](spec)
//	    if err != nil {
//	        return config.DesiredState{}, err
//	    }
//	    // Use parsed.Field1, parsed.Field2, etc.
//	    return config.DesiredState{State: parsed.GetState()}, nil
//	}
//
// For nil specs (used during initialization), returns zero value of T.
// For empty Config strings, returns zero value of T (allows defaults).
func ParseUserSpec[T any](spec interface{}) (T, error) {
	var zero T

	if spec == nil {
		return zero, nil
	}

	userSpec, ok := spec.(UserSpec)
	if !ok {
		return zero, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	var result T
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &result); err != nil {
			return zero, fmt.Errorf("failed to parse user spec config: %w", err)
		}
	}

	return result, nil
}

// DeriveLeafState is a one-liner helper for leaf workers (workers without children).
// It handles the common case of parsing UserSpec.Config and returning a DesiredState
// with no children.
//
// The type parameter T must have a pointer type *T that implements StateGetter
// (typically by embedding BaseUserSpec which has pointer receiver methods).
//
// Usage:
//
//	type MyUserSpec struct {
//	    config.BaseUserSpec `yaml:",inline"`
//	    CustomField string  `yaml:"customField"`
//	}
//
//	func (w *MyWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
//	    return config.DeriveLeafState[MyUserSpec](spec)
//	}
//
// This replaces ~15-25 lines of boilerplate with a single line.
func DeriveLeafState[T any, PT interface {
	*T
	StateGetter
}](spec interface{}) (DesiredState, error) {
	if spec == nil {
		return DesiredState{
			State:            DesiredStateRunning,
			OriginalUserSpec: nil,
		}, nil
	}

	parsed, err := ParseUserSpec[T](spec)
	if err != nil {
		return DesiredState{}, err
	}

	// Use pointer to call GetState since BaseUserSpec has pointer receiver
	ptr := PT(&parsed)
	return DesiredState{
		State:            ptr.GetState(),
		ChildrenSpecs:    nil,
		OriginalUserSpec: spec,
	}, nil
}
