package certificatehandler

import (
	"context"
	"crypto/x509"
)

// Handler manages certificate caching and root CA storage.
type Handler interface {
	GetCertificate(email string) *x509.Certificate
	GetIntermediateCerts(email string) []*x509.Certificate
	GetRootCA() *x509.Certificate
}

// Fetcher fetches and stores certificates for users.
type Fetcher interface {
	RunForUser(ctx context.Context, email string)
}
