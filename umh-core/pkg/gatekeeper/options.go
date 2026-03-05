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
