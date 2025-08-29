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
	"strings"

	"gopkg.in/yaml.v3"
)

var UMH_PEN = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59193}

// OIDs for our custom extensions
// 1: V1 of our extensions
// 1.1: Role extension
// 1.2: Location extension.
var (
	// OID for the location-role extension.
	oidExtensionRoleLocation asn1.ObjectIdentifier = append(UMH_PEN, 2, 1)
)

var ErrRoleLocationNotFound = errors.New("locationRoles extension not found in certificate")

// Role represents the role of a certificate holder.
type Role string

const (
	// RoleAdmin represents an administrator role.
	RoleAdmin Role = "Admin"

	// RoleViewer represents a viewer role.
	RoleViewer Role = "Viewer"

	// RoleEditor represents an editor role.
	RoleEditor Role = "Editor"
)

// LocationType represents the type of location in the hierarchy (for ExtensionLocation).
type LocationType string

const (
	// LocationTypeEnterprise represents the enterprise level.
	LocationTypeEnterprise LocationType = "Enterprise"

	// LocationTypeSite represents a site level.
	LocationTypeSite LocationType = "Site"

	// LocationTypeArea represents an area level.
	LocationTypeArea LocationType = "Area"

	// LocationTypeProductionLine represents a production line level.
	LocationTypeProductionLine LocationType = "ProductionLine"

	// LocationTypeWorkCell represents a work cell level.
	LocationTypeWorkCell LocationType = "WorkCell"
)

// LocataionRoles maps the locations to roles for the LocationRoles extension.
type LocationRoles map[string]Role

func GetRoleForLocation(cert *x509.Certificate, location string) (Role, error) {
	locationRoles, err := GetLocationRolesFromCertificate(cert)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to get locationRoles from certificate"))
	}

	for currentLocation, role := range locationRoles {
		if currentLocation == location {
			return role, nil
		}
	}

	return "", errors.New("did not find this location")
}

func GetLocationRolesFromCertificate(cert *x509.Certificate) (LocationRoles, error) {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionRoleLocation) {
			var locationRoles LocationRoles

			var locataionRolesString string

			_, err := asn1.Unmarshal(ext.Value, &locataionRolesString)
			if err != nil {
				return LocationRoles{}, errors.Join(err, errors.New("failed to asn1 unmarshal locationRole extension"))
			}

			err = yaml.Unmarshal([]byte(locataionRolesString), &locationRoles)
			if err != nil {
				return LocationRoles{}, errors.Join(err, errors.New("failed to yaml unmarshal locationRole extension value"))
			}

			// validate all roles and locations
			for location, role := range locationRoles {
				if role != RoleAdmin && role != RoleViewer && role != RoleEditor {
					return LocationRoles{}, fmt.Errorf("invalid role: %s", role)
				}

				if len(strings.Split(location, ".")) == 0 {
					return LocationRoles{}, fmt.Errorf("invalid location: %s", location)
				}
			}

			return locationRoles, nil
		}
	}

	return LocationRoles{}, ErrRoleLocationNotFound
}
