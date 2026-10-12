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
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// benthos-umh expands these references in every bridge config through the
// redpanda-data/benthos library it is built on (v4.80.0 in benthos-umh v0.16.0);
// the patterns and rules are copied so a value means the same in umh-core.
// https://github.com/redpanda-data/benthos/blob/v4.80.0/internal/config/env_vars.go
var (
	referencePattern        = regexp.MustCompile(`\${[0-9A-Za-z_.]+(:((\${[^}]+})|[^}])*)?(?:\|[0-9A-Za-z]+)?}`)
	escapedReferencePattern = regexp.MustCompile(`\${({[0-9A-Za-z_.]+(:((\${[^}]+})|[^}])*)?})}`)
)

type variableReference struct {
	name         string
	defaultValue string
	hasDefault   bool
	transform    string
}

// Expand replaces benthos-style environment variable references in value:
// ${VAR}, ${VAR:default}, ${VAR|base64decode}, and the escape ${{VAR}}, which
// becomes the literal text ${VAR}.
func Expand(value string) (string, error) {
	var expanded strings.Builder

	var missingNames []string

	previousEnd := 0

	for _, match := range referencePattern.FindAllStringIndex(value, -1) {
		start, end := match[0], match[1]
		expanded.WriteString(value[previousEnd:start])
		previousEnd = end

		reference := parseReference(value[start:end])

		resolved, found := lookup(reference)
		if !found {
			missingNames = append(missingNames, reference.name)

			continue
		}

		transformed, err := applyTransform(resolved, reference.transform)
		if err != nil {
			return "", err
		}

		expanded.WriteString(transformed)
	}

	expanded.WriteString(value[previousEnd:])

	if len(missingNames) > 0 {
		return "", fmt.Errorf("required environment variables were not set: %s", strings.Join(missingNames, ", "))
	}

	return escapedReferencePattern.ReplaceAllString(expanded.String(), "$$$1"), nil
}

func parseReference(text string) variableReference {
	body := text[len("${") : len(text)-len("}")]

	beforeColon, afterColon, hasDefault := strings.Cut(body, ":")
	if !hasDefault {
		name, transform := cutTransform(beforeColon)

		return variableReference{name: name, transform: transform}
	}

	defaultValue, transform := cutTransform(afterColon)

	return variableReference{name: beforeColon, defaultValue: defaultValue, hasDefault: true, transform: transform}
}

func cutTransform(part string) (rest, transform string) {
	pipeIndex := strings.LastIndex(part, "|")
	if pipeIndex == -1 {
		return part, ""
	}

	return part[:pipeIndex], part[pipeIndex+1:]
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

func applyTransform(value, transform string) (string, error) {
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
