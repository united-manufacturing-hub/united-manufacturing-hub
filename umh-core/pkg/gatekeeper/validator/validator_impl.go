//go:build cryptolib

package validator

import (
	"crypto/x509"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib"
)

type CryptoValidator struct {
	log *zap.SugaredLogger
}

func newValidatorImpl(log *zap.SugaredLogger) Validator {
	return &CryptoValidator{log: log}
}

// ValidateUserPermissions validates whether the user (identified by their certificate)
// has permission to perform the specified action on the given location.
// It delegates to the cryptolib.ValidateUserPermissions function.
func (v *CryptoValidator) ValidateUserPermissions(
	userCert *x509.Certificate,
	action string,
	location string,
	rootCA *x509.Certificate,
	intermediateCerts []*x509.Certificate,
) (bool, error) {
	return cryptolib.ValidateUserPermissions(userCert, action, location, rootCA, intermediateCerts)
}

// DecryptRootCA decrypts an encrypted root CA using the provided key material.
// It delegates to the cryptolib.DecryptRootCA function.
func (v *CryptoValidator) DecryptRootCA(encryptedCA string, keyMaterial string, salt string) (string, error) {
	return cryptolib.DecryptRootCA(encryptedCA, keyMaterial, salt)
}
