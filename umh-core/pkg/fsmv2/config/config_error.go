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

// ConfigError reports that a config field failed to load or render.
// Field is the field's YAML path.
type ConfigError struct {
	Field string
	Err   error
}

func (e *ConfigError) Error() string {
	if e.Err == nil {
		return e.Field
	}
	if e.Field == "" {
		return e.Err.Error()
	}
	return e.Field + ": " + e.Err.Error()
}

func (e *ConfigError) Unwrap() error { return e.Err }
