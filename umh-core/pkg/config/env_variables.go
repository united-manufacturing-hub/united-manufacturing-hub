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
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// benthos expands these references in every bridge config; the patterns and
// rules are copied so a value means the same in umh-core as in a bridge.
// https://github.com/redpanda-data/benthos/blob/v4.78.0/internal/config/env_vars.go
var (
	envVariableReferencePattern        = regexp.MustCompile(`\${[0-9A-Za-z_.]+(:((\${[^}]+})|[^}])*)?(?:\|[0-9A-Za-z]+)?}`)
	escapedEnvVariableReferencePattern = regexp.MustCompile(`\${({[0-9A-Za-z_.]+(:((\${[^}]+})|[^}])*)?})}`)
)

// missingEnvVariablesError lists the referenced environment variables that are
// not set and have no default.
type missingEnvVariablesError struct {
	Names []string
}

func (e *missingEnvVariablesError) Error() string {
	return "required environment variables were not set: " + strings.Join(e.Names, ", ")
}

// ExpandEnvVariables replaces benthos-style environment variable references in
// value: ${VAR}, ${VAR:default}, ${VAR|base64decode}, and the escape ${{VAR}},
// which becomes the literal text ${VAR}.
func ExpandEnvVariables(value string) (string, error) {
	var (
		missingNames    []string
		transformErrors []error
	)

	expanded := envVariableReferencePattern.ReplaceAllStringFunc(value, func(reference string) string {
		resolved, err := resolveEnvVariableReference(reference)

		var missing *missingEnvVariablesError
		if errors.As(err, &missing) {
			missingNames = append(missingNames, missing.Names...)

			return ""
		}

		if err != nil {
			transformErrors = append(transformErrors, err)

			return ""
		}

		return resolved
	})

	if len(transformErrors) > 0 {
		return "", errors.Join(transformErrors...)
	}

	if len(missingNames) > 0 {
		return "", &missingEnvVariablesError{Names: missingNames}
	}

	return escapedEnvVariableReferencePattern.ReplaceAllString(expanded, "$$$1"), nil
}

// resolveEnvVariableReference resolves one reference matched by
// envVariableReferencePattern, such as ${VAR:default|base64decode}.
func resolveEnvVariableReference(reference string) (string, error) {
	body := reference[len("${") : len(reference)-len("}")]

	name, defaultValue, hasDefault := strings.Cut(body, ":")

	var transform string
	if hasDefault {
		defaultValue, transform = cutTransform(defaultValue)
	} else {
		name, transform = cutTransform(name)
	}

	value, isSet := os.LookupEnv(name)
	if hasDefault && value == "" {
		value = defaultValue
	}

	if !hasDefault && !isSet {
		return "", &missingEnvVariablesError{Names: []string{name}}
	}

	return applyEnvVariableTransform(value, transform)
}

// cutTransform splits a trailing "|transform" off a reference part.
func cutTransform(part string) (string, string) {
	pipeIndex := strings.LastIndex(part, "|")
	if pipeIndex == -1 {
		return part, ""
	}

	return part[:pipeIndex], part[pipeIndex+1:]
}

func applyEnvVariableTransform(value string, transform string) (string, error) {
	switch transform {
	case "":
		return value, nil
	case "base64decode":
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64-encoded env var: %w", err)
		}

		return string(decoded), nil
	default:
		return "", fmt.Errorf("unknown env var decode function: %s", transform)
	}
}
