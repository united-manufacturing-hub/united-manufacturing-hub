package certificate

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/logger"
	"gopkg.in/yaml.v3"
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

	// OID for the location-role extension
	oidExtensionRoleLocation asn1.ObjectIdentifier = append(UMH_PEN, 2, 1)
)

var ERR_RoleLocation_Not_Found = errors.New("locationRoles extension not found in certificate")

// validateExtensionSize checks if a serialized extension value exceeds the configured size limit.
// This function enforces the CertificateExtensionSizeLimit to prevent DoS attacks and
// maintain reasonable certificate sizes.
//
// Parameters:
//   - extensionValue: The ASN.1 DER encoded extension value
//   - extensionType: Human-readable name of the extension type for error reporting
//
// Returns:
//   - error: nil if size is within limit, error if size exceeds CertificateExtensionSizeLimit
func validateExtensionSize(extensionValue []byte, extensionType string) error {
	size := len(extensionValue)
	if size > CertificateExtensionSizeLimit {
		logger.StdOutLogger("Certificate", "Extension size limit exceeded: %s extension is %d bytes (limit: %d bytes)",
			extensionType, size, CertificateExtensionSizeLimit)
		return fmt.Errorf("%s extension size %d bytes exceeds limit of %d bytes",
			extensionType, size, CertificateExtensionSizeLimit)
	}
	return nil
}

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

// LocationType represents the type of location in the hierarchy (for ExtensionLocation)
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

// Location represents a specific location with a type and value (for ExtensionLocation)
type Location struct {
	Type  LocationType
	Value string
}

// NewLocation creates a new Location with the given type and value (for ExtensionLocation)
func NewLocation(locType LocationType, value string) Location {
	return Location{
		Type:  locType,
		Value: value,
	}
}

// NewWildcardLocation creates a new Location with the given type and a wildcard value (for ExtensionLocation)
func NewWildcardLocation(locType LocationType) Location {
	return Location{
		Type:  locType,
		Value: "*",
	}
}

// IsWildcard returns true if the location is a wildcard (any value is allowed) (for ExtensionLocation)
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

// String returns a string representation of the location (for ExtensionLocation)
func (l Location) String() string {
	return fmt.Sprintf("%s:%s", l.Type, l.filterString(l.Value))
}

// LocationHierarchy represents a complete location hierarchy (for ExtensionLocation)
type LocationHierarchy struct {
	Enterprise     Location
	Site           Location
	Area           Location
	ProductionLine Location
	WorkCell       Location
}

// LocataionRoles maps the locations to roles for the LocationRoles extension
type LocationRoles map[string]Role

// NewLocationHierarchy creates a new LocationHierarchy with the given locations (for ExtensionLocation)
func NewLocationHierarchy(enterprise, site, area, productionLine, workCell Location) LocationHierarchy {
	return LocationHierarchy{
		Enterprise:     enterprise,
		Site:           site,
		Area:           area,
		ProductionLine: productionLine,
		WorkCell:       workCell,
	}
}

// RoleExtension represents the role extension data (for ExtensionRole)
type RoleExtension struct {
	Role Role
}

// LocationExtension represents the location extension data (for ExtensionLocation)
type LocationExtension struct {
	Hierarchies []LocationHierarchy
}

// ValidateLocationHierarchy validates that the location hierarchy is correct  (for ExtensionLocation)
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
	// Validate and normalize role using centralized parseRole function
	validatedRole, err := parseRole(role)
	if err != nil {
		return errors.Join(err, errors.New("failed to validate role for extension"))
	}
	role = validatedRole

	// Marshal the role to ASN.1 DER encoding
	value, err := asn1.Marshal(string(role))
	if err != nil {
		return errors.Join(err, errors.New("failed to marshal role extension"))
	}

	// Validate extension size against configured limit
	if err := validateExtensionSize(value, "Role"); err != nil {
		return errors.Join(err, errors.New("role extension size validation failed"))
	}

	// Add the extension to the template
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       oidExtensionRole,
		Critical: false, // Non-critical so clients that don't understand it can ignore it
		Value:    value,
	})

	return nil
}

func AddLocationRoleExtension(template *x509.Certificate, locationRoles LocationRoles) error {
	// Validate LocationRoles using the comprehensive validation function
	if err := ValidateLocationRoles(locationRoles); err != nil {
		return errors.Join(err, errors.New("invalid LocationRoles for certificate extension"))
	}

	//marshal the LocationRoles
	locationRolesBytes, err := yaml.Marshal(locationRoles)
	if err != nil {
		return fmt.Errorf("failed to yaml marshal locationRoles: %w", err)
	}

	value, err := asn1.Marshal(string(locationRolesBytes))
	if err != nil {
		return errors.Join(err, errors.New("failed to asn1 marshal locationRoles"))
	}

	// Validate extension size against configured limit
	if err := validateExtensionSize(value, "LocationRole"); err != nil {
		return errors.Join(err, errors.New("locationRole extension size validation failed"))
	}

	// Add the extension to the template
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       oidExtensionRoleLocation,
		Critical: false,
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

	// Validate extension size against configured limit
	if err := validateExtensionSize(value, "Location"); err != nil {
		return errors.Join(err, errors.New("location extension size validation failed"))
	}

	// Add the extension to the template
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       oidExtensionLocation,
		Critical: false, // Non-critical so clients that don't understand it can ignore it
		Value:    value,
	})

	return nil
}

// for ExtensionLocation
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

// GetRoleFromCertificate extracts the role from a certificate (TODO: deprecate)
func GetRoleFromCertificate(cert *x509.Certificate) (Role, error) {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionRole) {
			// Validate extension size against configured limit
			if err := validateExtensionSize(ext.Value, "Role"); err != nil {
				return "", errors.Join(err, errors.New("role extension size validation failed during parsing"))
			}

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

func GetLocationRolesFromCertificate(cert *x509.Certificate) (LocationRoles, error) {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionRoleLocation) {
			// Validate extension size against configured limit
			if err := validateExtensionSize(ext.Value, "LocationRole"); err != nil {
				return LocationRoles{}, errors.Join(err, errors.New("locationRole extension size validation failed during parsing"))
			}

			var locationRoles LocationRoles
			var locationRolesString string
			_, err := asn1.Unmarshal(ext.Value, &locationRolesString)
			if err != nil {
				return LocationRoles{}, errors.Join(err, errors.New("failed to asn1 unmarshal locationRole extension"))
			}
			err = yaml.Unmarshal([]byte(locationRolesString), &locationRoles)
			if err != nil {
				return LocationRoles{}, errors.Join(err, errors.New("failed to decode locationRole extension"))
			}

			// Validate all roles and locations using comprehensive validation
			if err := ValidateLocationRoles(locationRoles); err != nil {
				return LocationRoles{}, errors.Join(err, errors.New("invalid LocationRoles in certificate"))
			}

			return locationRoles, nil

		}
	}
	return LocationRoles{}, ERR_RoleLocation_Not_Found
}

// GetLocationHierarchiesFromCertificate extracts the location hierarchies from a certificate (TODO: deprecate)
func GetLocationHierarchiesFromCertificate(cert *x509.Certificate) ([]LocationHierarchy, error) {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionLocation) {
			// Validate extension size against configured limit
			if err := validateExtensionSize(ext.Value, "Location"); err != nil {
				return nil, errors.Join(err, errors.New("location extension size validation failed during parsing"))
			}

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
