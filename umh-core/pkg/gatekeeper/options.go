package gatekeeper

import "time"

// Default configuration values for the Gatekeeper.
const (
	DefaultVerifiedInboundBufferSize  = 100
	DefaultVerifiedOutboundBufferSize = 100
	DefaultCertFetchInterval          = time.Minute
)

// Option configures optional Gatekeeper parameters.
type Option func(*Gatekeeper)

// WithVerifiedInboundBufferSize sets the buffer size for the verified inbound channel.
func WithVerifiedInboundBufferSize(size int) Option {
	return func(g *Gatekeeper) { g.verifiedInboundSize = size }
}

// WithVerifiedOutboundBufferSize sets the buffer size for the verified outbound channel.
func WithVerifiedOutboundBufferSize(size int) Option {
	return func(g *Gatekeeper) { g.verifiedOutboundSize = size }
}

// WithCertFetchInterval sets how often the certificate fetcher runs.
func WithCertFetchInterval(d time.Duration) Option {
	return func(g *Gatekeeper) { g.certFetchInterval = d }
}
