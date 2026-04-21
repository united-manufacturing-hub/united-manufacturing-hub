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

package persistence

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

var (
	globalStore   storage.TriangularStoreInterface
	globalStoreMu sync.RWMutex
)

// SetStore publishes the triangular store for the persistence worker factory
// to consume via Store() during construction. cmd/main.go calls this before
// starting the application supervisor. Mirrors transport.SetChildDeps and the
// ChannelProvider singleton pattern to avoid an untyped extraDeps["store"]
// seam between cmd/main.go and the persistence worker factory.
func SetStore(s storage.TriangularStoreInterface) {
	globalStoreMu.Lock()
	defer globalStoreMu.Unlock()

	globalStore = s
}

// Store returns the triangular store published via SetStore, or nil if not
// yet published. The persistence worker factory calls this during init to
// obtain the store reference; if nil, the factory returns a descriptive
// error rather than panicking.
func Store() storage.TriangularStoreInterface {
	globalStoreMu.RLock()
	defer globalStoreMu.RUnlock()

	return globalStore
}

// ClearStore removes the published store (for test cleanup). Mirrors
// transport.ClearChildDeps.
func ClearStore() {
	globalStoreMu.Lock()
	defer globalStoreMu.Unlock()

	globalStore = nil
}
