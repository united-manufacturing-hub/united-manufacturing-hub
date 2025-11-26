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

package validator

import (
	"fmt"
	"strings"
)

// FormatViolations formats violations into a readable string.
func FormatViolations(title string, violations []Violation) string {
	return FormatViolationsWithPattern(title, violations, "")
}

// FormatViolationsWithPattern formats violations with pattern info from the registry.
func FormatViolationsWithPattern(title string, violations []Violation, patternType string) string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString("╔════════════════════════════════════════════════════════════════════════════╗\n")

	// Get pattern info if available
	pattern, hasPattern := PatternRegistry[patternType]
	if !hasPattern && len(violations) > 0 {
		pattern, hasPattern = PatternRegistry[violations[0].Type]
	}

	if hasPattern {
		sb.WriteString(fmt.Sprintf("║  PATTERN: %-66s║\n", pattern.Name))
		sb.WriteString("╠════════════════════════════════════════════════════════════════════════════╣\n")
		sb.WriteString("║  WHY THIS MATTERS:                                                         ║\n")
		for _, line := range WrapText(pattern.Why, 74) {
			sb.WriteString(fmt.Sprintf("║  %-74s║\n", line))
		}
		sb.WriteString("║                                                                            ║\n")
		sb.WriteString("║  CORRECT EXAMPLE:                                                          ║\n")
		for _, line := range strings.Split(pattern.CorrectCode, "\n") {
			if len(line) > 74 {
				line = line[:71] + "..."
			}
			sb.WriteString(fmt.Sprintf("║  %-74s║\n", line))
		}
		sb.WriteString("║                                                                            ║\n")
	} else {
		sb.WriteString(fmt.Sprintf("║  %-74s║\n", title))
		sb.WriteString("╠════════════════════════════════════════════════════════════════════════════╣\n")
	}

	sb.WriteString("║  VIOLATIONS FOUND:                                                         ║\n")
	for i, v := range violations {
		line := fmt.Sprintf("%d. %s", i+1, v)
		for _, wrapped := range WrapText(line, 74) {
			sb.WriteString(fmt.Sprintf("║  %-74s║\n", wrapped))
		}
	}
	sb.WriteString("║                                                                            ║\n")

	if hasPattern && pattern.ReferenceFile != "" {
		sb.WriteString(fmt.Sprintf("║  REFERENCE: See %-58s║\n", pattern.ReferenceFile))
	}

	sb.WriteString("╚════════════════════════════════════════════════════════════════════════════╝\n")
	sb.WriteString(fmt.Sprintf("\n📊 Total violations found: %d\n\n", len(violations)))

	return sb.String()
}

// WrapText wraps text to fit within maxWidth characters.
func WrapText(text string, maxWidth int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		currentLine := words[0]
		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) <= maxWidth {
				currentLine += " " + word
			} else {
				lines = append(lines, currentLine)
				currentLine = word
			}
		}
		lines = append(lines, currentLine)
	}
	return lines
}
