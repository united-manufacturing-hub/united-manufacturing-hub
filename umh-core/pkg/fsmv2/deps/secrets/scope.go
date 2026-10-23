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
)

// Key names one entry. Declare it once, as a package-level var next to the
// worker that owns the entry, and pass that var to every Get, Put and Delete:
//
//	var JWTKey = secrets.NewKey("transport.jwt")
type Key struct {
	name string
}

// NewKey returns the key for the entry called name. Name it
// "<worker type>.<entry>", as in "transport.jwt". It panics when name is
// empty, contains "/", or is "." or "..", so a bad name fails when the
// declaring package loads.
func NewKey(name string) Key {
	if !validName(name) {
		panic(fmt.Sprintf("secrets.NewKey(%q): a name must be non-empty, contain no \"/\", and not be \".\" or \"..\"", name))
	}

	return Key{name: name}
}

// Scope is one worker's view of the store: Get, Put and Delete act on entries
// under that worker's hierarchy path.
type Scope struct {
	store *Store
	path  string
}

// PathSource is satisfied by every worker's dependencies. ForWorker reads the
// worker's hierarchy path from it.
type PathSource interface {
	GetHierarchyPath() string
}

// ForWorker returns the scope for the hierarchy path in d. It returns
// ErrNoStore when the handle has no store behind it, and an error wrapping
// ErrInvalidPath when the path is empty or malformed.
func (h Handle) ForWorker(d PathSource) (Scope, error) {
	if h.store == nil {
		return Scope{}, ErrNoStore
	}
	path := d.GetHierarchyPath()
	if err := checkPath(path); err != nil {
		return Scope{}, err
	}

	return Scope{store: h.store, path: path}, nil
}

// Get returns the value stored under key, or ErrNotFound when nothing is
// stored there yet.
func (s Scope) Get(key Key) (Value, error) {
	entry, err := s.entryFor(key)
	if err != nil {
		return Value{}, err
	}
	value, found := s.store.get(entry)
	if !found {
		return Value{}, ErrNotFound
	}

	return value, nil
}

// Put stores value under key.
func (s Scope) Put(key Key, value Value) error {
	entry, err := s.entryFor(key)
	if err != nil {
		return err
	}
	s.store.put(entry, value)

	return nil
}

// Delete removes the entry under key; a key with no entry is not an error.
func (s Scope) Delete(key Key) error {
	entry, err := s.entryFor(key)
	if err != nil {
		return err
	}
	s.store.delete(entry)

	return nil
}

// entryFor returns the entryID for key under the scope's path.
func (s Scope) entryFor(key Key) (entryID, error) {
	if s.store == nil {
		return entryID{}, ErrNoStore
	}
	if key.name == "" {
		return entryID{}, ErrInvalidName
	}

	return entryID{path: s.path, name: key.name}, nil
}

// checkPath returns nil if path is a well-formed hierarchy path: non-empty,
// with no empty, "." or ".." segment. Otherwise it returns an error wrapping
// ErrInvalidPath.
func checkPath(path string) error {
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("%w: %q", ErrInvalidPath, path)
		}
	}

	return nil
}

// validName reports whether name is non-empty, contains no "/", and is not
// "." or "..".
func validName(name string) bool {
	return name != "" && !strings.Contains(name, "/") && name != "." && name != ".."
}
