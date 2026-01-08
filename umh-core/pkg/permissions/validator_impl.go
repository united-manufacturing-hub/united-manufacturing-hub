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

//go:build cryptolib

package permissions

import (
	"crypto/x509"

	v2 "github.com/united-manufacturing-hub/ManagementConsole/cryptolib/interfaces"
)

// CryptoValidator is the full implementation of the Validator interface
// used in internal builds where the ManagementConsole cryptolib is available.
// It delegates all permission validation to the cryptolib's GoCrypto.
type CryptoValidator struct {
	crypto *v2.GoCrypto
}

// Compile-time check that CryptoValidator implements Validator
var _ Validator = (*CryptoValidator)(nil)

// NewValidator creates a new CryptoValidator for internal builds.
// This implementation uses ManagementConsole's cryptolib for full
// certificate chain validation and permission checking.
func NewValidator() Validator {
	return &CryptoValidator{
		crypto: v2.NewGoCrypto(),
	}
}

// ValidateUserPermissions validates whether a user has permission to execute
// a specific action at a given location. It delegates to the cryptolib's
// ValidateUserPermissions which handles:
//   - Certificate chain validation (userCert → intermediates → rootCA)
//   - Role extraction for the location (V2 LocationRoles or V1 hierarchies)
//   - Action permission checking based on role
func (v *CryptoValidator) ValidateUserPermissions(
	userCert *x509.Certificate,
	action string,
	location string,
	rootCA *x509.Certificate,
	intermediateCerts []*x509.Certificate,
) (bool, error) {
	return v.crypto.ValidateUserPermissions(userCert, action, location, rootCA, intermediateCerts)
}
