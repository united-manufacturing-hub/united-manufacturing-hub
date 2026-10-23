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
	"fmt"
	"strings"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// StoreKey is the dependency key under which cmd/main.go puts the store's
// handle in the root dependency map. A worker reads it with
// config.LookupDependency(deps, secrets.StoreKey) and passes what it gets to
// ScopeOf; it uses the handle for nothing else.
var StoreKey = config.NewDependencyKey[Handle]("secrets.store")

// MemoryStore holds every secret entry of one process, keyed by the
// hierarchy path of the worker that owns the entry and the entry's name.
// cmd/main.go creates the one store and puts its handle in the root
// dependency map; no worker creates one.
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[entryKey]Value
}

// entryKey is the map key for one entry: the owner path and the name joined.
type entryKey struct {
	owner string
	name  string
}

// NewMemoryStore builds an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[entryKey]Value)}
}

// Handle returns the store's handle, the value cmd/main.go puts under
// StoreKey in the root dependency map. A worker only ever passes it to
// ScopeOf; the handle has no methods of its own.
func (s *MemoryStore) Handle() Handle {
	return Handle{store: s}
}

// Handle is the value that travels through the dependency map. A worker
// passes it to ScopeOf and uses it for nothing else.
type Handle struct {
	store *MemoryStore
}

// CheckPath returns nil if path is a well-formed hierarchy path: non-empty,
// with no leading or trailing "/", and no empty, "." or ".." segment.
// Otherwise it returns an error wrapping ErrInvalidOwner.
func CheckPath(path string) error {
	if _, ok := splitPath(path); !ok {
		return fmt.Errorf("%w: %q", ErrInvalidOwner, path)
	}

	return nil
}

// splitPath returns the path's segments, without the separators. An empty
// segment anywhere means the path is malformed.
func splitPath(p string) ([]string, bool) {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return nil, false
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, false
		}
	}
	return parts, true
}

// validName reports whether name is a plain entry name: non-empty, with no
// path element in it.
func validName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/") && name != "." && name != ".."
}
