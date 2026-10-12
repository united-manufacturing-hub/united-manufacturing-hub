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

package env

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// benthos resolves these two forms the same way in bridge flows; every other
// benthos form (transforms, the ${{VAR}} escape) is rejected so the two never
// disagree on a value both read.
// https://github.com/redpanda-data/benthos/blob/v4.80.0/internal/config/env_vars.go
var (
	referencePattern          = regexp.MustCompile(`\$\{[^}]*\}`)
	supportedReferencePattern = regexp.MustCompile(`^\$\{[0-9A-Za-z_.]+(:[^}|{$]*)?\}$`)
)

type variableReference struct {
	name         string
	defaultValue string
	hasDefault   bool
}

// ResolveVariables replaces ${VAR} and ${VAR:default} references in value with
// environment variables. It fails on any other reference form and on a variable
// that is not set and has no default.
func ResolveVariables(value string) (string, error) {
	var resolvedValue strings.Builder

	var missingNames []string

	previousEnd := 0

	for _, match := range referencePattern.FindAllStringIndex(value, -1) {
		start, end := match[0], match[1]
		resolvedValue.WriteString(value[previousEnd:start])
		previousEnd = end

		reference, err := parseReference(value[start:end])
		if err != nil {
			return "", err
		}

		variableValue, found := lookup(reference)
		if !found {
			missingNames = append(missingNames, reference.name)

			continue
		}

		resolvedValue.WriteString(variableValue)
	}

	resolvedValue.WriteString(value[previousEnd:])

	if len(missingNames) > 0 {
		return "", fmt.Errorf("required environment variables were not set: %s", strings.Join(missingNames, ", "))
	}

	return resolvedValue.String(), nil
}

// ValidateReferences fails when value contains a reference other than ${VAR} or
// ${VAR:default}. It does not read the environment.
func ValidateReferences(value string) error {
	for _, reference := range referencePattern.FindAllString(value, -1) {
		if _, err := parseReference(reference); err != nil {
			return err
		}
	}

	return nil
}

func parseReference(text string) (variableReference, error) {
	if !supportedReferencePattern.MatchString(text) {
		return variableReference{}, fmt.Errorf("environment variable reference %s is not supported: use ${VAR} or ${VAR:default}", text)
	}

	body := text[len("${") : len(text)-len("}")]
	name, defaultValue, hasDefault := strings.Cut(body, ":")

	return variableReference{name: name, defaultValue: defaultValue, hasDefault: hasDefault}, nil
}

func lookup(reference variableReference) (string, bool) {
	value, isSet := os.LookupEnv(reference.name)
	if !reference.hasDefault {
		return value, isSet
	}

	if value == "" {
		return reference.defaultValue, true
	}

	return value, true
}
