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

// Package config provides core configuration types for FSMv2, including child specifications, variables, templates, and location hierarchies.
//
// The package supports flexible location hierarchies, with ISA-95 as the default standard:
//
//	Enterprise → Site → Area → Line → Cell
//
// Location computation is used to construct topic paths in the Unified Namespace (UNS).
// For example, a parent location [enterprise: ACME, site: Factory-1] merged with
// a child location [line: Line-A, cell: Cell-5] results in the path:
//
//	ACME.Factory-1.Line-A.Cell-5
//
// Empty levels are filled but skipped in the path computation to avoid double dots.
package config

import (
	"strings"
)

// LocationLevel represents a single level in a location hierarchy.
// Supports flexible hierarchies with ISA-95 as the default: Enterprise > Site > Area > Line > Cell.
type LocationLevel struct {
	Type  string `json:"type"  yaml:"type"`  // Level type (e.g., "enterprise", "site", "area", "line", "cell")
	Value string `json:"value" yaml:"value"` // The actual value for this level (e.g., "ACME", "Factory-1")
}

// MergeLocations combines parent and child location levels.
// Parent locations come first, followed by child locations.
func MergeLocations(parent, child []LocationLevel) []LocationLevel {
	result := make([]LocationLevel, 0, len(parent)+len(child))
	result = append(result, parent...)
	result = append(result, child...)

	return result
}

// NormalizeHierarchyLevels ensures all 5 hierarchy levels are present in the correct order.
// Missing levels are filled with empty strings.
// Default order (following ISA-95): enterprise → site → area → line → cell.
func NormalizeHierarchyLevels(levels []LocationLevel) []LocationLevel {
	// Define standard hierarchy order (ISA-95 compatible)
	hierarchyOrder := []string{"enterprise", "site", "area", "line", "cell"}

	levelMap := make(map[string]string)
	for _, level := range levels {
		levelMap[level.Type] = level.Value
	}

	// Build result with all 5 levels in standard order
	result := make([]LocationLevel, 5)

	for i, levelType := range hierarchyOrder {
		value := levelMap[levelType]
		result[i] = LocationLevel{
			Type:  levelType,
			Value: value,
		}
	}

	return result
}

// ComputeLocationPath joins non-empty location values with dots.
// Empty values are skipped to avoid double dots.
// Returns empty string if all levels are empty.
func ComputeLocationPath(levels []LocationLevel) string {
	var parts []string

	for _, level := range levels {
		if level.Value != "" {
			parts = append(parts, level.Value)
		}
	}

	return strings.Join(parts, ".")
}
