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
