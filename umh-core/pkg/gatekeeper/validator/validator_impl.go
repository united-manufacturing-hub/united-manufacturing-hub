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

//go:build cryptolib

package validator

import (
	"crypto/x509"

	"go.uber.org/zap"

	cryptolib "github.com/united-manufacturing-hub/ManagementConsole/cryptolib/interfaces"
)

// CryptoValidator delegates to the ManagementConsole cryptolib for permission validation.
type CryptoValidator struct {
	crypto *cryptolib.GoCrypto
	log    *zap.SugaredLogger
}

func newValidatorImpl(log *zap.SugaredLogger) Validator {
	return &CryptoValidator{
		crypto: cryptolib.NewGoCrypto(),
		log:    log,
	}
}

// ValidateUserPermissions validates whether the user (identified by their certificate)
// has permission to perform the specified action on the given location.
func (v *CryptoValidator) ValidateUserPermissions(
	userCert *x509.Certificate,
	action string,
	location string,
	rootCA *x509.Certificate,
	intermediateCerts []*x509.Certificate,
) (bool, error) {
	return v.crypto.ValidateUserPermissions(userCert, action, location, rootCA, intermediateCerts)
}

// DecryptRootCA decrypts an encrypted root CA using the provided key material.
func (v *CryptoValidator) DecryptRootCA(encryptedCA string, keyMaterial string, salt string) (string, error) {
	return v.crypto.DecryptRootCA(encryptedCA, keyMaterial, salt)
}
