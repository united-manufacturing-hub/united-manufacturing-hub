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

import "strings"

// Default configuration values for the Gatekeeper.
const (
	DefaultVerifiedInboundBufferSize  = 100
	DefaultVerifiedOutboundBufferSize = 100
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

// WithLocation sets the instance location path from the agent config's location map.
func WithLocation(location map[int]string) Option {
	return func(g *Gatekeeper) {
		if len(location) == 0 {
			return
		}
		maxLevel := -1
		for k := range location {
			if k > maxLevel {
				maxLevel = k
			}
		}
		parts := make([]string, 0, maxLevel+1)
		for i := 0; i <= maxLevel; i++ {
			if val, ok := location[i]; ok && val != "" {
				parts = append(parts, val)
			}
		}
		g.locationPath = strings.Join(parts, ".")
	}
}
