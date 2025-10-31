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

package basic

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

func validateContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context cannot be nil")
	}
	return nil
}

type InMemoryStore struct {
	mu          sync.RWMutex
	collections map[string]map[string]Document
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		collections: make(map[string]map[string]Document),
	}
}

func (s *InMemoryStore) CreateCollection(ctx context.Context, name string, schema *Schema) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.collections[name]; exists {
		return fmt.Errorf("collection %q already exists", name)
	}

	s.collections[name] = make(map[string]Document)
	return nil
}

func (s *InMemoryStore) DropCollection(ctx context.Context, name string) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.collections[name]; !exists {
		return fmt.Errorf("collection %q does not exist", name)
	}

	delete(s.collections, name)
	return nil
}

func (s *InMemoryStore) Insert(ctx context.Context, collection string, doc Document) (string, error) {
	if err := validateContext(ctx); err != nil {
		return "", err
	}

	id, ok := doc["id"].(string)
	if !ok || id == "" {
		return "", errors.New("document must have non-empty 'id' field")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	coll, exists := s.collections[collection]
	if !exists {
		return "", fmt.Errorf("collection %q does not exist", collection)
	}

	if _, exists := coll[id]; exists {
		return "", ErrConflict
	}

	docCopy := make(Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	coll[id] = docCopy
	return id, nil
}

func (s *InMemoryStore) Get(ctx context.Context, collection string, id string) (Document, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	coll, exists := s.collections[collection]
	if !exists {
		return nil, fmt.Errorf("collection %q does not exist", collection)
	}

	doc, exists := coll[id]
	if !exists {
		return nil, ErrNotFound
	}

	docCopy := make(Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	return docCopy, nil
}

func (s *InMemoryStore) Update(ctx context.Context, collection string, id string, doc Document) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	coll, exists := s.collections[collection]
	if !exists {
		return fmt.Errorf("collection %q does not exist", collection)
	}

	if _, exists := coll[id]; !exists {
		return ErrNotFound
	}

	docCopy := make(Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	coll[id] = docCopy
	return nil
}

func (s *InMemoryStore) Delete(ctx context.Context, collection string, id string) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	coll, exists := s.collections[collection]
	if !exists {
		return fmt.Errorf("collection %q does not exist", collection)
	}

	if _, exists := coll[id]; !exists {
		return ErrNotFound
	}

	delete(coll, id)
	return nil
}

func (s *InMemoryStore) Find(ctx context.Context, collection string, query Query) ([]Document, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	coll, exists := s.collections[collection]
	if !exists {
		return nil, fmt.Errorf("collection %q does not exist", collection)
	}

	var results []Document
	for _, doc := range coll {
		docCopy := make(Document)
		for k, v := range doc {
			docCopy[k] = v
		}
		results = append(results, docCopy)
	}

	return results, nil
}

func (s *InMemoryStore) Maintenance(ctx context.Context) error {
	return nil
}

func (s *InMemoryStore) BeginTx(ctx context.Context) (Tx, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	return &inMemoryTx{
		store:     s,
		committed: false,
		rolledBack: false,
		changes:   make(map[string]map[string]*Document),
		deletes:   make(map[string]map[string]bool),
	}, nil
}

func (s *InMemoryStore) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.collections = make(map[string]map[string]Document)
	return nil
}

type inMemoryTx struct {
	store      *InMemoryStore
	committed  bool
	rolledBack bool
	changes    map[string]map[string]*Document
	deletes    map[string]map[string]bool
	mu         sync.Mutex
}

func (tx *inMemoryTx) CreateCollection(ctx context.Context, name string, schema *Schema) error {
	return tx.store.CreateCollection(ctx, name, schema)
}

func (tx *inMemoryTx) DropCollection(ctx context.Context, name string) error {
	return tx.store.DropCollection(ctx, name)
}

func (tx *inMemoryTx) Insert(ctx context.Context, collection string, doc Document) (string, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return "", errors.New("transaction already completed")
	}

	id, ok := doc["id"].(string)
	if !ok || id == "" {
		return "", errors.New("document must have non-empty 'id' field")
	}

	if tx.changes[collection] == nil {
		tx.changes[collection] = make(map[string]*Document)
	}

	if tx.deletes[collection] != nil && tx.deletes[collection][id] {
		return "", errors.New("document was deleted in this transaction")
	}

	if tx.changes[collection][id] != nil {
		return "", ErrConflict
	}

	docCopy := make(Document)
	for k, v := range doc {
		docCopy[k] = v
	}
	tx.changes[collection][id] = &docCopy

	return id, nil
}

func (tx *inMemoryTx) Get(ctx context.Context, collection string, id string) (Document, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return nil, errors.New("transaction already completed")
	}

	if tx.deletes[collection] != nil && tx.deletes[collection][id] {
		return nil, ErrNotFound
	}

	if tx.changes[collection] != nil {
		if doc := tx.changes[collection][id]; doc != nil {
			docCopy := make(Document)
			for k, v := range *doc {
				docCopy[k] = v
			}
			return docCopy, nil
		}
	}

	return tx.store.Get(ctx, collection, id)
}

func (tx *inMemoryTx) Update(ctx context.Context, collection string, id string, doc Document) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return errors.New("transaction already completed")
	}

	if tx.deletes[collection] != nil && tx.deletes[collection][id] {
		return ErrNotFound
	}

	if tx.changes[collection] == nil {
		tx.changes[collection] = make(map[string]*Document)
	}

	docCopy := make(Document)
	for k, v := range doc {
		docCopy[k] = v
	}
	tx.changes[collection][id] = &docCopy

	return nil
}

func (tx *inMemoryTx) Delete(ctx context.Context, collection string, id string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return errors.New("transaction already completed")
	}

	if tx.deletes[collection] == nil {
		tx.deletes[collection] = make(map[string]bool)
	}

	tx.deletes[collection][id] = true

	if tx.changes[collection] != nil {
		delete(tx.changes[collection], id)
	}

	return nil
}

func (tx *inMemoryTx) Find(ctx context.Context, collection string, query Query) ([]Document, error) {
	return tx.store.Find(ctx, collection, query)
}

func (tx *inMemoryTx) Maintenance(ctx context.Context) error {
	return nil
}

func (tx *inMemoryTx) BeginTx(ctx context.Context) (Tx, error) {
	return nil, errors.New("nested transactions not supported")
}

func (tx *inMemoryTx) Close(ctx context.Context) error {
	return errors.New("cannot close transaction")
}

func (tx *inMemoryTx) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return nil
	}

	if tx.rolledBack {
		return errors.New("transaction was rolled back")
	}

	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()

	for collection, changes := range tx.changes {
		if _, exists := tx.store.collections[collection]; !exists {
			return fmt.Errorf("collection %q does not exist", collection)
		}

		for id, doc := range changes {
			tx.store.collections[collection][id] = *doc
		}
	}

	for collection, deletes := range tx.deletes {
		if _, exists := tx.store.collections[collection]; !exists {
			continue
		}

		for id := range deletes {
			delete(tx.store.collections[collection], id)
		}
	}

	tx.committed = true
	return nil
}

func (tx *inMemoryTx) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.rolledBack || tx.committed {
		return nil
	}

	tx.rolledBack = true
	tx.changes = make(map[string]map[string]*Document)
	tx.deletes = make(map[string]map[string]bool)

	return nil
}
