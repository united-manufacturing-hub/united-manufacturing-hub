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

package permission_validator

import (
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var UMH_PEN = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59193}

// OIDs for our custom extensions
// 1: V1 of our extensions
// 1.1: Role extension
// 1.2: Location extension
var (
	// OID for Role extension
	oidExtensionRole asn1.ObjectIdentifier = append(UMH_PEN, 1, 1)

	// OID for Location extension
	oidExtensionLocation asn1.ObjectIdentifier = append(UMH_PEN, 1, 2)
)

// Role represents the role of a certificate holder
type Role string

const (
	// RoleAdmin represents an administrator role
	RoleAdmin Role = "Admin"

	// RoleViewer represents a viewer role
	RoleViewer Role = "Viewer"

	// RoleEditor represents an editor role
	RoleEditor Role = "Editor"
)

// LocationType represents the type of location in the hierarchy
type LocationType string

const (
	// LocationTypeEnterprise represents the enterprise level
	LocationTypeEnterprise LocationType = "Enterprise"

	// LocationTypeSite represents a site level
	LocationTypeSite LocationType = "Site"

	// LocationTypeArea represents an area level
	LocationTypeArea LocationType = "Area"

	// LocationTypeProductionLine represents a production line level
	LocationTypeProductionLine LocationType = "ProductionLine"

	// LocationTypeWorkCell represents a work cell level
	LocationTypeWorkCell LocationType = "WorkCell"
)

// Location represents a specific location with a type and value
type Location struct {
	Type  LocationType
	Value string
}

// NewLocation creates a new Location with the given type and value
func NewLocation(locType LocationType, value string) Location {
	return Location{
		Type:  locType,
		Value: value,
	}
}

// NewWildcardLocation creates a new Location with the given type and a wildcard value
func NewWildcardLocation(locType LocationType) Location {
	return Location{
		Type:  locType,
		Value: "*",
	}
}

// IsWildcard returns true if the location is a wildcard (any value is allowed)
func (l Location) IsWildcard() bool {
	return l.Value == "*"
}

func (l Location) filterString(s string) string {
	// The frontend often uses "All sites", "All areas", etc.
	wildcardStrings := []string{
		"All enterprises",
		"All sites",
		"All areas",
		"All production lines",
		"All work cells",
	}

	if slices.Contains(wildcardStrings, s) {
		return "*"
	}
	if s == "" {
		return "*"
	}

	return s
}

// String returns a string representation of the location
func (l Location) String() string {
	return fmt.Sprintf("%s:%s", l.Type, l.filterString(l.Value))
}

// LocationHierarchy represents a complete location hierarchy
type LocationHierarchy struct {
	Enterprise     Location
	Site           Location
	Area           Location
	ProductionLine Location
	WorkCell       Location
}

// ValidateLocationHierarchy validates that the location hierarchy is correct
// All levels must be set, but can be wildcards (including Enterprise)
// If a specific level has a non-wildcard value, all higher levels must also have non-wildcard values
func ValidateLocationHierarchy(hierarchy LocationHierarchy) error {
	// Enterprise must be set
	if hierarchy.Enterprise.Type != LocationTypeEnterprise {
		return errors.New("enterprise location must be of type Enterprise")
	}

	// Site must be set
	if hierarchy.Site.Type != LocationTypeSite {
		return errors.New("site location must be of type Site")
	}

	// Area must be set
	if hierarchy.Area.Type != LocationTypeArea {
		return errors.New("area location must be of type Area")
	}

	// ProductionLine must be set
	if hierarchy.ProductionLine.Type != LocationTypeProductionLine {
		return errors.New("production line location must be of type ProductionLine")
	}

	// WorkCell must be set
	if hierarchy.WorkCell.Type != LocationTypeWorkCell {
		return errors.New("work cell location must be of type WorkCell")
	}
	return nil
}

// GetRoleFromCertificate extracts the role from a certificate
func GetRoleFromCertificate(cert *x509.Certificate) (Role, error) {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionRole) {
			var roleStr string
			_, err := asn1.Unmarshal(ext.Value, &roleStr)
			if err != nil {
				return "", errors.Join(err, errors.New("failed to unmarshal role extension"))
			}

			role := Role(roleStr)
			if role != RoleAdmin && role != RoleViewer && role != RoleEditor {
				return "", fmt.Errorf("invalid role in certificate: %s", role)
			}

			return role, nil
		}
	}

	return "", errors.New("role extension not found in certificate")
}

// GetLocationHierarchiesFromCertificate extracts the location hierarchies from a certificate
func GetLocationHierarchiesFromCertificate(cert *x509.Certificate) ([]LocationHierarchy, error) {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionLocation) {
			var locStrings []string
			_, err := asn1.Unmarshal(ext.Value, &locStrings)
			if err != nil {
				return nil, errors.Join(err, errors.New("failed to unmarshal location extension"))
			}

			// Each hierarchy consists of 5 location strings
			if len(locStrings)%5 != 0 {
				return nil, fmt.Errorf("invalid number of location strings: %d (must be a multiple of 5)", len(locStrings))
			}

			hierarchyCount := len(locStrings) / 5
			hierarchies := make([]LocationHierarchy, hierarchyCount)

			for h := 0; h < hierarchyCount; h++ {
				// Parse each location string for this hierarchy
				locations := make([]Location, 5)
				for i := 0; i < 5; i++ {
					locStr := locStrings[h*5+i]
					parts := strings.SplitN(locStr, ":", 2)
					if len(parts) != 2 {
						return nil, fmt.Errorf("invalid location string format: %s", locStr)
					}

					locType := LocationType(parts[0])
					locValue := parts[1]

					locations[i] = Location{
						Type:  locType,
						Value: locValue,
					}
				}

				hierarchies[h] = LocationHierarchy{
					Enterprise:     locations[0],
					Site:           locations[1],
					Area:           locations[2],
					ProductionLine: locations[3],
					WorkCell:       locations[4],
				}

				// Validate location hierarchy
				if err := ValidateLocationHierarchy(hierarchies[h]); err != nil {
					return nil, errors.Join(err, errors.New("invalid location hierarchy in certificate"))
				}
			}

			return hierarchies, nil
		}
	}

	return nil, errors.New("location extension not found in certificate")
}
