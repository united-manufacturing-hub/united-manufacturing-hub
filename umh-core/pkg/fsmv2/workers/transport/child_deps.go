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

package transport

import "sync"

var (
	globalChildDeps *TransportDependencies
	childDepsMu     sync.RWMutex
)

// SetChildDeps publishes the transport worker's dependencies for push/pull
// children to consume via ChildDeps(). Transport's constructor calls this
// after building its deps. Mirrors the ChannelProvider singleton pattern
// to avoid an untyped extraDeps seam between parent and children.
func SetChildDeps(d *TransportDependencies) {
	childDepsMu.Lock()
	defer childDepsMu.Unlock()

	globalChildDeps = d
}

// ChildDeps returns the transport worker's dependencies published via
// SetChildDeps, or nil if not yet published. Push and pull child workers
// call this during their factory init to obtain the parent reference.
func ChildDeps() *TransportDependencies {
	childDepsMu.RLock()
	defer childDepsMu.RUnlock()

	return globalChildDeps
}

// ClearChildDeps removes the published deps (for test cleanup). Mirrors
// ClearChannelProvider.
func ClearChildDeps() {
	childDepsMu.Lock()
	defer childDepsMu.Unlock()

	globalChildDeps = nil
}
