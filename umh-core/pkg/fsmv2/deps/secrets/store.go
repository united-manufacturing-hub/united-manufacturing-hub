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

package secrets

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// StoreKey is the dependency key for the store's Handle.
var StoreKey = config.NewDependencyKey[Handle]("secrets.store")

// Store holds secret entries in memory, keyed by hierarchy path and entry name.
type Store struct {
	mu      sync.RWMutex
	entries map[entryID]Value
}

// entryID identifies one entry by its owner's hierarchy path and its name.
type entryID struct {
	path string
	name string
}

// NewStore builds an empty store.
func NewStore() *Store {
	return &Store{entries: make(map[entryID]Value)}
}

// Handle refers to a Store. A worker opens its Scope from it with ForWorker.
type Handle struct {
	store *Store
}

// Handle returns the value to put under StoreKey.
func (s *Store) Handle() Handle {
	return Handle{store: s}
}

func (s *Store) get(key entryID) (Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, found := s.entries[key]

	return value, found
}

func (s *Store) put(key entryID, value Value) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[key] = value
}

func (s *Store) delete(key entryID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, key)
}
