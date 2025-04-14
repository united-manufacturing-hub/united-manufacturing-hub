package env

/*
Copyright 2023 UMH Systems GmbH

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// GetAsString returns the value of the environment variable as a string. If the environment variable is not set and not required, the fallback value is returned.
func GetAsString(key string, required bool, fallback string) (string, error) {
	value, set := os.LookupEnv(key)

	// Check if the environment variable is set
	if !set {
		// If not required, return the fallback value
		if !required {
			return fallback, nil
		}
		// If required, return an error
		return "", fmt.Errorf("environment variable %s is required but not set", key)
	}

	return value, nil
}

// GetAsInt returns the value of the environment variable as an int. If the environment variable is not set and not required, the fallback value is returned.
func GetAsInt(key string, required bool, fallback int) (int, error) {
	value, set := os.LookupEnv(key)

	// Check if the environment variable is set
	if !set {
		// If not required, return the fallback value
		if !required {
			return fallback, nil
		}
		// If required, return an error
		return 0, fmt.Errorf("environment variable %s is required but not set", key)
	}

	i, err := strconv.Atoi(value)
	if err != nil {
		return fallback, fmt.Errorf("environment variable %s is not an integer. using fallback value", key)
	}

	return i, nil
}

// GetAsUint64 returns the value of the environment variable as an uint64. If the environment variable is not set and not required, the fallback value is returned.
func GetAsUint64(key string, required bool, fallback uint64) (uint64, error) {
	value, set := os.LookupEnv(key)

	// Check if the environment variable is set
	if !set {
		// If not required, return the fallback value
		if !required {
			return fallback, nil
		}
		// If required, return an error
		return 0, fmt.Errorf("environment variable %s is required but not set", key)
	}

	i, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fallback, fmt.Errorf("environment variable %s is not an integer. using fallback value", key)
	}

	return i, nil
}

// GetAsFloat64 returns the value of the environment variable as a float64. If the environment variable is not set and not required, the fallback value is returned.
func GetAsFloat64(key string, required bool, fallback float64) (float64, error) {
	value, set := os.LookupEnv(key)

	// Check if the environment variable is set
	if !set {
		// If not required, return the fallback value
		if !required {
			return fallback, nil
		}
		// If required, return an error
		return 0, fmt.Errorf("environment variable %s is required but not set", key)
	}

	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback, fmt.Errorf("environment variable %s is not a float. using fallback value", key)
	}

	return f, nil
}

// GetAsBool returns the value of the environment variable as a bool. If the environment variable is not set and not required, the fallback value is returned.
func GetAsBool(key string, required bool, fallback bool) (bool, error) {
	value, set := os.LookupEnv(key)

	// Check if the environment variable is set
	if !set {
		// If not required, return the fallback value
		if !required {
			return fallback, nil
		}
		// If required, return an error
		return false, fmt.Errorf("environment variable %s is required but not set", key)
	}

	b, err := strconv.ParseBool(value)
	if err != nil {
		return fallback, fmt.Errorf("environment variable %s is not a boolean. using fallback value", key)
	}

	return b, nil
}

// GetAsType retrieves the value of an environment variable by the given key,
// unmarshals it to the type of unmarshalTo, and sets the value of unmarshalTo to
// the unmarshaled value. If the environment variable is not set and required
// is false, the value of fallback is used instead. If the environment variable
// is not set and required is true, an error is returned. If unmarshaling fails,
// an error is returned.
//
// key: the name of the environment variable
//
// unmarshalTo: a pointer to the value to unmarshal the environment variable to
//
// required: whether the environment variable is required to be set
//
// fallback: a pointer to the value to use if the environment variable is not set
//
// Returns: an error if there is an error getting the environment variable value
// or unmarshaling it to the target value, or nil if successful.
func GetAsType[T any](key string, unmarshalTo *T, required bool, fallback T) error {
	value, set := os.LookupEnv(key)

	// Check if the value is null or empty
	if !set {
		// If not required, return the fallback value
		if !required {
			var ptr *T = &fallback
			*unmarshalTo = *ptr
			return nil
		}
		// If required, panic with an error message
		return fmt.Errorf("environment variable %s is required but not set", key)
	}

	// Unmarshal the environment variable value to the target value
	err := json.Unmarshal([]byte(value), &unmarshalTo)
	if err != nil {
		// If unmarshaling fails, return an error message
		var ptr *T = &fallback
		*unmarshalTo = *ptr
		return fmt.Errorf("failed to unmarshal environment variable %s: %w", key, err)
	}

	return nil
}
