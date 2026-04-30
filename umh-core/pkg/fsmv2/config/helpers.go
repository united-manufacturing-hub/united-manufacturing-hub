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

// ParseUserSpec provides type-safe parsing of UserSpec.Config into a typed struct.
//
// Usage:
//
//	func (w *MyWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
//	    parsed, err := config.ParseUserSpec[MyUserSpec](spec)
//	    if err != nil {
//	        return nil, err
//	    }
//	    // Use parsed.Field1, parsed.Field2, etc.
//	    return &fsmv2.WrappedDesiredState[MyConfig]{Config: parsed}, nil
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

