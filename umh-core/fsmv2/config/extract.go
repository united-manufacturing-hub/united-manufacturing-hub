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
	"encoding/json"
	"fmt"
)

// Extract converts Variables.User map to a typed struct.
// Workers call this to get type-safe access to their configuration.
//
// Example:
//
//	type MyConfig struct {
//	    IP   string `json:"IP"`
//	    Port int    `json:"PORT"`
//	}
//
//	cfg, err := Extract[MyConfig](userSpec.Variables)
//	if err != nil {
//	    return err
//	}
//	fmt.Println(cfg.IP, cfg.Port)
func Extract[T any](vars VariableBundle) (T, error) {
	var result T

	jsonBytes, err := json.Marshal(vars.User)
	if err != nil {
		return result, fmt.Errorf("marshal variables: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return result, fmt.Errorf("unmarshal to %T: %w", result, err)
	}

	return result, nil
}

// MustExtract panics if extraction fails. Use in tests.
func MustExtract[T any](vars VariableBundle) T {
	result, err := Extract[T](vars)
	if err != nil {
		panic(err)
	}

	return result
}
