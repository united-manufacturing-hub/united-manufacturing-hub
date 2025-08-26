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
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var UMH_PEN = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59193}

// OIDs for our custom extensions
// 1: V1 of our extensions (backwards compatibility)
// 1.1: Role extension (v1: global role, v2: per-location role scopes)
// 1.2: Location extension (deprecated in favor of DNS SANs + Name Constraints)
var (
	// OID for Role extension (supports both v1 and v2 payloads)
	oidExtensionRole asn1.ObjectIdentifier = append(UMH_PEN, 1, 1)

	// OID for Location extension (deprecated - keeping for backwards compatibility)
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

// NEW: LocationDNS represents a location as a DNS name with arbitrary depth
type LocationDNS string

// Examples:
// - "cell3.lineA.area2.cologne.umh.internal"  (most specific: cell level)
// - "area2.cologne.umh.internal"              (area level)
// - "cologne.umh.internal"                    (site level)
// - "office.cologne.umh.internal"             (administrative locations)
// - "*.area2.cologne.umh.internal"            (wildcard: all cells in area2)
// - "*.cologne.umh.internal"                  (wildcard: all areas in cologne)

// NEW: RoleScope represents a role that applies to a specific DNS subtree
type RoleScope struct {
	DNSSuffix LocationDNS `json:"dns_suffix"` // e.g. ".cologne.umh.internal"
	Role      Role        `json:"role"`       // admin/editor/viewer
}

// NEW: V2 role extension payload structure
type roleScopesV2 struct {
	Version int         `json:"v"`                 // must be 2
	Scopes  []RoleScope `json:"scopes"`            // per-location role mappings
	Default Role        `json:"default,omitempty"` // fallback role if no suffix matches
}

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

// NewLocationHierarchy creates a new LocationHierarchy with the given locations
func NewLocationHierarchy(enterprise, site, area, productionLine, workCell Location) LocationHierarchy {
	return LocationHierarchy{
		Enterprise:     enterprise,
		Site:           site,
		Area:           area,
		ProductionLine: productionLine,
		WorkCell:       workCell,
	}
}

// RoleExtension represents the role extension data
type RoleExtension struct {
	Role Role
}

// LocationExtension represents the location extension data
type LocationExtension struct {
	Hierarchies []LocationHierarchy
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

// AddRoleExtension adds a role extension to the certificate template
func AddRoleExtension(template *x509.Certificate, role Role) error {
	// Validate role
	if role != RoleAdmin && role != RoleViewer && role != RoleEditor {
		return fmt.Errorf("invalid role: %s", role)
	}

	// Marshal the role to ASN.1 DER encoding
	value, err := asn1.Marshal(string(role))
	if err != nil {
		return errors.Join(err, errors.New("failed to marshal role extension"))
	}

	// Add the extension to the template
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       oidExtensionRole,
		Critical: false, // Non-critical so clients that don't understand it can ignore it
		Value:    value,
	})

	return nil
}

// AddLocationExtension adds a location extension to the certificate template
func AddLocationExtension(template *x509.Certificate, hierarchies []LocationHierarchy) error {
	if len(hierarchies) == 0 {
		return errors.New("at least one location hierarchy must be specified")
	}

	// Validate each location hierarchy
	for _, hierarchy := range hierarchies {
		if err := ValidateLocationHierarchy(hierarchy); err != nil {
			return err
		}
	}

	// Convert hierarchies to a serializable format
	// Format: [hierarchy1_enterprise, hierarchy1_site, hierarchy1_area, hierarchy1_productionline, hierarchy1_workcell, hierarchy2_enterprise, ...]
	locStrings := make([]string, 0, len(hierarchies)*5)
	for _, hierarchy := range hierarchies {
		locStrings = append(locStrings,
			hierarchy.Enterprise.String(),
			hierarchy.Site.String(),
			hierarchy.Area.String(),
			hierarchy.ProductionLine.String(),
			hierarchy.WorkCell.String(),
		)
	}

	err := validateHierarchies(hierarchies)
	if err != nil {
		return err
	}

	// Marshal the locations to ASN.1 DER encoding
	value, err := asn1.Marshal(locStrings)
	if err != nil {
		return errors.Join(err, errors.New("failed to marshal location extension"))
	}

	// Add the extension to the template
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       oidExtensionLocation,
		Critical: false, // Non-critical so clients that don't understand it can ignore it
		Value:    value,
	})

	return nil
}

// NEW: AddRoleScopesV2Extension adds a v2 role extension with per-location role mappings
func AddRoleScopesV2Extension(template *x509.Certificate, scopes []RoleScope) error {
	if len(scopes) == 0 {
		return errors.New("role scopes cannot be empty")
	}

	// Validate all scopes
	for _, scope := range scopes {
		if scope.Role != RoleAdmin && scope.Role != RoleEditor && scope.Role != RoleViewer {
			return fmt.Errorf("invalid role in scope: %s", scope.Role)
		}
		if scope.DNSSuffix == "" {
			return errors.New("DNS suffix cannot be empty")
		}
		if !strings.HasPrefix(string(scope.DNSSuffix), ".") {
			return fmt.Errorf("DNS suffix must start with '.': %s", scope.DNSSuffix)
		}
	}

	payload := roleScopesV2{
		Version: 2,
		Scopes:  scopes,
	}

	// Marshal to JSON for simplicity and debuggability
	value, err := json.Marshal(payload)
	if err != nil {
		return errors.Join(err, errors.New("failed to marshal role scopes v2"))
	}

	// Add the extension to the template
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       oidExtensionRole,
		Critical: false, // Non-critical for compatibility with standard X.509 validation
		Value:    value,
	})

	return nil
}

// NEW: ParseRoleInfo extracts role information from a certificate (backwards compatible)
func ParseRoleInfo(cert *x509.Certificate) (v2 *roleScopesV2, v1 Role, hasV2 bool, hasV1 bool, err error) {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionRole) {
			// Try v2 JSON first
			var rs roleScopesV2
			if json.Unmarshal(ext.Value, &rs) == nil && rs.Version == 2 {
				return &rs, "", true, false, nil
			}

			// Fallback to v1 ASN.1 format (existing format)
			var roleStr string
			if _, asnErr := asn1.Unmarshal(ext.Value, &roleStr); asnErr == nil {
				role := Role(roleStr)
				if role == RoleAdmin || role == RoleEditor || role == RoleViewer {
					return nil, role, false, true, nil
				}
			}

			// If we get here, the extension exists but we can't parse it
			return nil, "", false, false, fmt.Errorf("unknown role extension payload format")
		}
	}
	return nil, "", false, false, errors.New("role extension not found")
}

// NEW: EffectiveRoleFor determines the effective role for a given DNS name using v2 scopes
func EffectiveRoleFor(dnsName string, v2 *roleScopesV2) Role {
	if v2 == nil {
		return ""
	}

	// Find the most specific (longest) matching DNS suffix
	bestLen := -1
	var bestRole Role
	lowerName := strings.ToLower(dnsName)

	for _, scope := range v2.Scopes {
		lowerSuffix := strings.ToLower(string(scope.DNSSuffix))
		if strings.HasSuffix(lowerName, lowerSuffix) && len(lowerSuffix) > bestLen {
			bestLen = len(lowerSuffix)
			bestRole = scope.Role
		}
	}

	if bestLen >= 0 {
		return bestRole
	}
	return v2.Default
}

// NEW: Helper function to convert LocationDNS slice to string slice
func LocationDNSToStrings(locations []LocationDNS) []string {
	result := make([]string, len(locations))
	for i, loc := range locations {
		result[i] = string(loc)
	}
	return result
}

func validateHierarchies(hierarchies []LocationHierarchy) error {
	disallowedSymbols := []string{"."}

	checkValue := func(value, fieldName string) error {
		for _, symbol := range disallowedSymbols {
			if strings.Contains(value, symbol) {
				return fmt.Errorf("invalid symbol '%s' in %s location '%s'",
					symbol, fieldName, value)
			}
		}
		return nil
	}

	for _, hierarchy := range hierarchies {
		if err := checkValue(hierarchy.Enterprise.Value, "enterprise"); err != nil {
			return err
		}
		if err := checkValue(hierarchy.Site.Value, "site"); err != nil {
			return err
		}
		if err := checkValue(hierarchy.Area.Value, "area"); err != nil {
			return err
		}
		if err := checkValue(hierarchy.ProductionLine.Value, "production line"); err != nil {
			return err
		}
		if err := checkValue(hierarchy.WorkCell.Value, "work cell"); err != nil {
			return err
		}
	}
	return nil
}

// GetRoleForLocation determines the effective role for a specific location from a certificate.
// This function handles both v1 (global role) and v2 (per-location roles) certificates.
//
// Parameters:
//   - cert: The X.509 certificate to examine
//   - location: The location as map[int]string representing the hierarchy levels
//     Example: map[int]string{0: "cologne", 1: "area2", 2: "cell1"} represents "cell1.area2.cologne.umh.internal"
//
// Returns:
//   - Role: The effective role for the location ("admin", "editor", "viewer", or "" if no access)
//   - error: Any parsing errors
func GetRoleForLocation(cert *x509.Certificate, location map[int]string) (Role, error) {
	// Parse role information from certificate (handles both v1 and v2)
	v2, v1, hasV2, hasV1, err := ParseRoleInfo(cert)
	if err != nil && !hasV1 && !hasV2 {
		return "", fmt.Errorf("no role information found in certificate: %w", err)
	}

	// Convert location map to DNS name for v2 processing
	locationDNS := ConvertLocationMapToDNS(location)

	if hasV2 {
		// V2: Per-location roles
		role := EffectiveRoleFor(locationDNS, v2)
		return role, nil
	} else if hasV1 {
		// V1: Global role, but check if certificate has access to this location
		if hasAccessToLocationV1(cert, location) {
			return v1, nil
		}
		return "", nil // No access to this location
	}

	return "", fmt.Errorf("no valid role information found")
}

// GetRoleFromCertificate extracts the role from a certificate (DEPRECATED: use GetRoleForLocation)
// This function is kept for backwards compatibility but should not be used in new code.
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

// ConvertLocationMapToDNS converts a location map to DNS format
// Example: map[int]string{0: "cologne", 1: "area2", 2: "cell1"} -> "cell1.area2.cologne.umh.internal"
func ConvertLocationMapToDNS(location map[int]string) string {
	if len(location) == 0 {
		return ""
	}

	// Find the maximum key to determine the depth
	maxKey := -1
	for k := range location {
		if k > maxKey {
			maxKey = k
		}
	}

	// Build the DNS name from highest level (maxKey) to lowest (0)
	var parts []string
	for i := maxKey; i >= 0; i-- {
		if part, exists := location[i]; exists && part != "" {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	// Add the .umh.internal suffix
	return strings.Join(parts, ".") + ".umh.internal"
}

// hasAccessToLocationV1 checks if a v1 certificate has access to the given location
// based on the stored location hierarchies in the certificate
func hasAccessToLocationV1(cert *x509.Certificate, location map[int]string) bool {
	// Get the location hierarchies from the v1 certificate
	hierarchies, err := GetLocationHierarchiesFromCertificate(cert)
	if err != nil {
		return false
	}

	// Convert the requested location to a hierarchy for comparison
	requestedHierarchy := convertLocationMapToHierarchy(location)

	// Check if any hierarchy in the certificate matches or contains the requested location
	for _, hierarchy := range hierarchies {
		if hierarchyContainsLocation(hierarchy, requestedHierarchy) {
			return true
		}
	}

	return false
}

// convertLocationMapToHierarchy converts a location map to LocationHierarchy
func convertLocationMapToHierarchy(location map[int]string) LocationHierarchy {
	hierarchy := LocationHierarchy{}

	if enterprise, exists := location[0]; exists {
		hierarchy.Enterprise = Location{Type: "Enterprise", Value: enterprise}
	}
	if site, exists := location[1]; exists {
		hierarchy.Site = Location{Type: "Site", Value: site}
	}
	if area, exists := location[2]; exists {
		hierarchy.Area = Location{Type: "Area", Value: area}
	}
	if productionLine, exists := location[3]; exists {
		hierarchy.ProductionLine = Location{Type: "ProductionLine", Value: productionLine}
	}
	if workCell, exists := location[4]; exists {
		hierarchy.WorkCell = Location{Type: "WorkCell", Value: workCell}
	}

	return hierarchy
}

// hierarchyContainsLocation checks if a certificate hierarchy contains or matches the requested location
func hierarchyContainsLocation(certHierarchy, requestedHierarchy LocationHierarchy) bool {
	// Check each level - certificate hierarchy must match or be a wildcard
	if !levelMatches(certHierarchy.Enterprise, requestedHierarchy.Enterprise) {
		return false
	}
	if !levelMatches(certHierarchy.Site, requestedHierarchy.Site) {
		return false
	}
	if !levelMatches(certHierarchy.Area, requestedHierarchy.Area) {
		return false
	}
	if !levelMatches(certHierarchy.ProductionLine, requestedHierarchy.ProductionLine) {
		return false
	}
	if !levelMatches(certHierarchy.WorkCell, requestedHierarchy.WorkCell) {
		return false
	}

	return true
}

// levelMatches checks if a certificate level matches the requested level
func levelMatches(certLevel, requestedLevel Location) bool {
	// If certificate level is wildcard (*), it matches anything
	if certLevel.Value == "*" {
		return true
	}

	// If certificate level is empty, it matches empty requested level
	if certLevel.Value == "" && requestedLevel.Value == "" {
		return true
	}

	// Otherwise, must be exact match
	return certLevel.Value == requestedLevel.Value
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

// NEW V2 CONVENIENCE FUNCTIONS for easy migration and usage

// CreateRoleScope creates a new RoleScope for a DNS suffix and role
func CreateRoleScope(dnsSuffix string, role Role) RoleScope {
	if !strings.HasPrefix(dnsSuffix, ".") {
		dnsSuffix = "." + dnsSuffix
	}
	return RoleScope{
		DNSSuffix: LocationDNS(dnsSuffix),
		Role:      role,
	}
}

// CreateLocationDNSFromHierarchy converts old hierarchy format to new DNS format
// Example: enterprise="umh", site="cologne", area="area2", line="lineA", cell="cell3"
// becomes: "cell3.lineA.area2.cologne.umh.internal"
// Example with wildcards: enterprise="umh", site="cologne", area="*"
// becomes: "*.cologne.umh.internal"
func CreateLocationDNSFromHierarchy(hierarchy LocationHierarchy) LocationDNS {
	parts := []string{}
	hasWildcard := false

	// Build from most specific to least specific (reverse hierarchy)
	// Stop at first wildcard and mark it
	if !hierarchy.WorkCell.IsWildcard() {
		parts = append(parts, hierarchy.WorkCell.Value)
	} else if len(parts) == 0 {
		hasWildcard = true
	}

	if !hierarchy.ProductionLine.IsWildcard() && !hasWildcard {
		parts = append(parts, hierarchy.ProductionLine.Value)
	} else if len(parts) == 0 && !hasWildcard {
		hasWildcard = true
	}

	if !hierarchy.Area.IsWildcard() && !hasWildcard {
		parts = append(parts, hierarchy.Area.Value)
	} else if len(parts) == 0 && !hasWildcard {
		hasWildcard = true
	}

	if !hierarchy.Site.IsWildcard() && !hasWildcard {
		parts = append(parts, hierarchy.Site.Value)
	} else if len(parts) == 0 && !hasWildcard {
		hasWildcard = true
	}

	if !hierarchy.Enterprise.IsWildcard() && !hasWildcard {
		parts = append(parts, hierarchy.Enterprise.Value)
	}

	// Add the domain suffix
	parts = append(parts, "internal")

	locationStr := strings.Join(parts, ".")

	// Add wildcard prefix if we found a wildcard in the hierarchy
	if hasWildcard {
		locationStr = "*." + locationStr
	}

	return LocationDNS(locationStr)
}

// ConvertHierarchiesToLocations converts old v1 hierarchies to v2 location DNS names
func ConvertHierarchiesToLocations(hierarchies []LocationHierarchy) []LocationDNS {
	locations := make([]LocationDNS, 0, len(hierarchies))
	for _, hierarchy := range hierarchies {
		locations = append(locations, CreateLocationDNSFromHierarchy(hierarchy))
	}
	return locations
}

// CreateUniformRoleScopes creates role scopes that give the same role to all locations
func CreateUniformRoleScopes(locations []LocationDNS, role Role) []RoleScope {
	scopes := make([]RoleScope, 0, len(locations))
	for _, location := range locations {
		// Create a scope for this exact location (add leading dot)
		suffix := "." + string(location)
		scopes = append(scopes, CreateRoleScope(suffix, role))
	}
	return scopes
}

// GetEffectiveRoleForLocation is a convenience wrapper around EffectiveRoleFor
func GetEffectiveRoleForLocation(location LocationDNS, cert *x509.Certificate) (Role, error) {
	v2, v1, hasV2, hasV1, err := ParseRoleInfo(cert)
	if err != nil {
		return "", err
	}

	if hasV2 {
		return EffectiveRoleFor(string(location), v2), nil
	} else if hasV1 {
		// V1 certificates have global role for all locations
		return v1, nil
	}

	return "", errors.New("no role information found")
}

// ValidateLocationDNS validates that a location DNS name follows proper hierarchy rules
func ValidateLocationDNS(location LocationDNS) error {
	locationStr := string(location)

	// Must not be empty
	if locationStr == "" {
		return errors.New("location DNS cannot be empty")
	}

	// Must end with .umh.internal
	if !strings.HasSuffix(locationStr, ".umh.internal") {
		return fmt.Errorf("location DNS must end with '.umh.internal': %s", locationStr)
	}

	// Handle wildcard locations (must be exactly "*.something")
	isWildcard := false
	checkStr := locationStr
	if strings.HasPrefix(locationStr, "*") {
		if !strings.HasPrefix(locationStr, "*.") {
			return fmt.Errorf("wildcard location DNS must use '*.domain' format: %s", locationStr)
		}
		isWildcard = true
		checkStr = locationStr[2:] // Remove "*." from the start
	}

	// Must not start with a dot (after wildcard handling)
	if strings.HasPrefix(checkStr, ".") {
		return fmt.Errorf("location DNS cannot start with '.': %s", locationStr)
	}

	// Split checkStr and validate parts
	parts := strings.Split(checkStr, ".")

	// Special validation for global wildcard: *.umh.internal is allowed
	if isWildcard && checkStr == "umh.internal" {
		// This is *.umh.internal which is valid for global access
		if len(parts) != 2 || parts[0] != "umh" || parts[1] != "internal" {
			return fmt.Errorf("invalid global wildcard format: %s", locationStr)
		}
		return nil
	}

	// Must have at least one level before .umh.internal (e.g., site.umh.internal or *.site.umh.internal)
	if len(parts) < 3 { // minimum: [site, umh, internal]
		return fmt.Errorf("location DNS must have at least one location level: %s", locationStr)
	}

	// Check for forbidden generic names (excluding the wildcard asterisk)
	forbiddenNames := []string{"identity", "generic", "default", "any", "all"}
	for _, part := range parts {
		for _, forbidden := range forbiddenNames {
			if strings.EqualFold(part, forbidden) {
				return fmt.Errorf("location DNS contains forbidden generic name '%s': %s", forbidden, locationStr)
			}
		}
	}

	// Wildcard validation: must have at least one level after wildcard
	if isWildcard && len(parts) < 3 {
		return fmt.Errorf("wildcard location DNS must specify parent location: %s", locationStr)
	}

	return nil
}

// IsWildcardLocation checks if a LocationDNS represents a wildcard location
func IsWildcardLocation(location LocationDNS) bool {
	return strings.HasPrefix(string(location), "*.")
}
