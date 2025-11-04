// Copyright 2025 UMH Systems GmbH

// Package location provides ISA-95 location hierarchy computation for the FSM v2 system.
//
// ISA-95 Standard Hierarchy:
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
package location

import (
	"strings"
)

// LocationLevel represents a single level in the ISA-95 hierarchy.
// ISA-95 defines a 5-level hierarchy: Enterprise > Site > Area > Line > Cell.
type LocationLevel struct {
	Type  string `json:"type"  yaml:"type"`  // "enterprise", "site", "area", "line", or "cell"
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

// FillISA95Gaps ensures all 5 ISA-95 levels are present in the correct order.
// Missing levels are filled with empty strings.
// ISA-95 order: enterprise → site → area → line → cell.
func FillISA95Gaps(levels []LocationLevel) []LocationLevel {
	// Define ISA-95 hierarchy order
	isa95Order := []string{"enterprise", "site", "area", "line", "cell"}

	// Build map of existing levels
	levelMap := make(map[string]string)
	for _, level := range levels {
		levelMap[level.Type] = level.Value
	}

	// Build result with all 5 levels in ISA-95 order
	result := make([]LocationLevel, 5)

	for i, levelType := range isa95Order {
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
