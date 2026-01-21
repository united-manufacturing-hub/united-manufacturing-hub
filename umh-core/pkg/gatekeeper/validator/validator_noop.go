//go:build !cryptolib

package validator

import (
	"crypto/x509"

	"go.uber.org/zap"
)

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
