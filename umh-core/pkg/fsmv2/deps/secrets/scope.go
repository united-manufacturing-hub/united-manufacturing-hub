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

import "strings"

// Scope is what a worker opens from a handle with ScopeOf. Its Get, Put and
// Delete act on one named entry under the scope's hierarchy path, and they
// take no path.
type Scope struct {
	store *MemoryStore
	path  string
}

// PathSource is satisfied by every worker's dependencies. ScopeOf reads the
// worker's hierarchy path from it.
type PathSource interface {
	GetHierarchyPath() string
}

// ScopeOf opens the scope for the hierarchy path in d. A worker calls it once,
// in its constructor, with the handle from config.LookupDependency(deps,
// secrets.StoreKey) and its own dependencies:
//
//	scope, err := secrets.ScopeOf(handle, deps)
//
// It returns an error, and no usable scope, when the handle has no store
// behind it or the path is empty or malformed. ScopeOf does not check who
// calls it, so code that passes another worker's dependencies, or builds its
// own PathSource, gets that path's scope.
func ScopeOf(h Handle, d PathSource) (Scope, error) {
	if h.store == nil {
		return Scope{}, ErrNoStore
	}
	path := d.GetHierarchyPath()
	if err := CheckPath(path); err != nil {
		return Scope{}, err
	}
	return Scope{store: h.store, path: path}, nil
}

// Get returns the value stored under name in the scope's path, or
// ErrNotFound when no entry of that name is there.
func (s Scope) Get(name string) (Value, error) {
	if !validName(name) {
		return Value{}, ErrInvalidName
	}
	if s.store == nil {
		return Value{}, ErrNotFound
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	val, found := s.store.entries[entryKey{owner: s.path, name: name}]
	if !found {
		return Value{}, ErrNotFound
	}
	return val, nil
}

// Put stores value under name in the scope's path.
func (s Scope) Put(name string, v Value) error {
	if !validName(name) {
		return ErrInvalidName
	}
	if s.store == nil {
		return ErrNotFound
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.entries[entryKey{owner: s.path, name: name}] = v
	return nil
}

// Delete removes the entry under name from the scope's path.
func (s Scope) Delete(name string) error {
	if !validName(name) {
		return ErrInvalidName
	}
	if s.store == nil {
		return ErrNotFound
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	delete(s.store.entries, entryKey{owner: s.path, name: name})
	return nil
}

// Ancestor returns the scope of the nearest ancestor whose worker type
// equals workerType. A worker calls it when it needs a secret that a worker
// above it holds:
//
//	comm, ok := scope.Ancestor("communicator")
//
// It walks up from the parent toward the root and stops at the first segment
// whose type part equals workerType. It returns that ancestor's scope and
// true, or false when no ancestor has that type. It never returns the
// worker's own scope or the root as a fallback, and it refuses an empty
// type. The returned scope can read, write and delete that ancestor's
// entries.
func (s Scope) Ancestor(workerType string) (Scope, bool) {
	if workerType == "" || s.store == nil {
		return Scope{}, false
	}
	segs, ok := splitPath(s.path)
	if !ok {
		return Scope{}, false
	}
	for i := len(segs) - 2; i >= 0; i-- {
		if typePart(segs[i]) == workerType {
			return Scope{store: s.store, path: strings.Join(segs[:i+1], "/")}, true
		}
	}
	return Scope{}, false
}

// typePart returns the text inside a segment's last pair of parentheses. A
// segment with no pair, or an empty pair, has an empty type part.
func typePart(seg string) string {
	i := strings.LastIndex(seg, "(")
	if i < 0 || !strings.HasSuffix(seg, ")") {
		return ""
	}
	return seg[i+1 : len(seg)-1]
}
