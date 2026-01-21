package certificatehandler

import "crypto/x509"

// Handler manages certificate caching and root CA storage.
type Handler interface {
	GetCertificate(email string) *x509.Certificate
	GetIntermediateCerts(email string) []*x509.Certificate
	GetRootCA() *x509.Certificate
}
