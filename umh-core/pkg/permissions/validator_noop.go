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

//go:build !cryptolib

package permissions

import (
	"crypto/x509"
	"errors"
)

// ErrNotImplemented is returned by the noop validator in OSS builds
// where the cryptolib is not available.
var ErrNotImplemented = errors.New("permission validation not available in OSS build")

// NoopValidator is a no-op implementation of the Validator interface
// used in OSS builds where the ManagementConsole cryptolib is not available.
type NoopValidator struct{}

// Compile-time check that NoopValidator implements Validator
var _ Validator = (*NoopValidator)(nil)

// NewValidator creates a new NoopValidator for OSS builds.
// This implementation always returns ErrNotImplemented.
func NewValidator() Validator {
	return &NoopValidator{}
}

// ValidateUserPermissions always returns false and ErrNotImplemented
// in OSS builds since the cryptolib is not available.
func (v *NoopValidator) ValidateUserPermissions(
	userCert *x509.Certificate,
	action string,
	location string,
	rootCA *x509.Certificate,
	intermediateCerts []*x509.Certificate,
) (bool, error) {
	return false, ErrNotImplemented
}
