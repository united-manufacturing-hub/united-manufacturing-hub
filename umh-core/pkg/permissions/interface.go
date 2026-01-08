// Copyright 2026 UMH Systems GmbH
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

package permissions

import (
	"context"
	"crypto/x509"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// Validator defines the interface for permission validation.
// This interface has two implementations:
//   - StubValidator (validator_stub.go): Used in OSS builds, returns "not implemented"
//   - CryptoValidator (validator_impl.go): Used in internal builds with -tags cryptolib,
//     delegates to ManagementConsole's cryptolib for full certificate chain validation
type Validator interface {
	// ValidateUserPermissions validates whether a user has permission to execute
	// a specific action at a given location within the UMH system.
	//
	// This function performs several validation steps:
	//  1. Validates the user certificate against the certificate chain (rootCA + intermediates)
	//  2. Extracts the user's role for the specified location from the certificate
	//  3. Checks if that role is allowed to perform the requested action
	//
	// Parameters:
	//   - userCert: the X.509 certificate of the user attempting the action
	//   - action: the action being requested (e.g., "get-protocol-converter", "deploy-connection")
	//   - location: the location where the action is being performed (e.g., "enterprise.site.area")
	//   - rootCA: the root Certificate Authority certificate (decrypted)
	//   - intermediateCerts: slice of intermediate certificates in the chain
	//
	// Returns:
	//   - bool: true if the user has permission, false otherwise
	//   - error: non-nil if validation fails (certificate invalid, chain broken, etc.)
	ValidateUserPermissions(userCert *x509.Certificate, action string, location string, rootCA *x509.Certificate, intermediateCerts []*x509.Certificate) (bool, error)
}

// GetLocationString converts a location map (0=Enterprise, 1=Site, 2=Area, 3=Line, 4=WorkCell) to a dot-separated string.
func GetLocationString(location map[int]string) string {
	if location == nil {
		return ""
	}
	var parts []string
	for i := 0; i <= 4; i++ {
		if val, ok := location[i]; ok && val != "" {
			parts = append(parts, val)
		} else {
			break
		}
	}
	return strings.Join(parts, ".")
}

// GetLocationFromConfig fetches the instance location from ConfigManager and returns it as a dot-separated string.
func GetLocationFromConfig(configManager config.ConfigManager) string {
	if configManager == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cfg, err := configManager.GetConfig(ctx, 0)
	if err != nil {
		return ""
	}
	return GetLocationString(cfg.Agent.Location)
}
