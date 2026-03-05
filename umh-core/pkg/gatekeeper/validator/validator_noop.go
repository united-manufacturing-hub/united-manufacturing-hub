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

//go:build !cryptolib

package validator

import (
	"crypto/x509"

	"go.uber.org/zap"
)

// NoopValidator is a stub validator for OSS builds without the cryptolib dependency.
type NoopValidator struct {
	log *zap.SugaredLogger
}

func newValidatorImpl(log *zap.SugaredLogger) Validator {
	return &NoopValidator{log: log}
}

// ValidateUserPermissions always returns true in the noop implementation.
// This allows all actions through without certificate validation.
func (v *NoopValidator) ValidateUserPermissions(
	_ *x509.Certificate,
	_ string,
	_ string,
	_ *x509.Certificate,
	_ []*x509.Certificate,
) (bool, error) {
	return true, nil
}

// DecryptRootCA returns an error in the noop implementation since
// encryption/decryption requires the cryptolib dependency.
func (v *NoopValidator) DecryptRootCA(_ string, _ string, _ string) (string, error) {
	return "", nil
}
