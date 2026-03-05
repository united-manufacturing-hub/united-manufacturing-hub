// Package certificatehandler manages user certificate caching, retrieval, and
// fetching from the Management Console API.
package certificatehandler

import (
	"context"
	"crypto/x509"
)

// Handler manages certificate caching, retrieval, and fetching.
type Handler interface {
	Certificate(email string) *x509.Certificate
	IntermediateCerts(email string) []*x509.Certificate
	RootCA() *x509.Certificate
	FetchAndStore(ctx context.Context, email string) error
}
