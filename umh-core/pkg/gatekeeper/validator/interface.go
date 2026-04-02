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

// Package validator provides certificate-based permission validation.
package validator

import (
	"crypto/x509"

	"go.uber.org/zap"
)

// Validator validates user permissions based on certificates.
type Validator interface {
	ValidateUserPermissions(userCert *x509.Certificate, action string, location string, rootCA *x509.Certificate, intermediateCerts []*x509.Certificate) (bool, error)
	DecryptRootCA(encryptedCA string, keyMaterial string, salt string) (string, error)
}

// NewValidator creates a Validator (noop or crypto based on build tags).
func NewValidator(log *zap.SugaredLogger) Validator {
	return newValidatorImpl(log)
}
