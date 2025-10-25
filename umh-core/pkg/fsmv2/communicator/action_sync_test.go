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

package communicator_test

import (
	"context"
	"errors"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	// "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
	// "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	// csesync "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/sync"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func VerifyActionIdempotency(action interface {
	Execute(context.Context) error
}, iterations int, verifyState func()) {
	ctx := context.Background()

	for i := 0; i < iterations; i++ {
		err := action.Execute(ctx)
		Expect(err).ToNot(HaveOccurred(), "Action should succeed on iteration %d", i+1)
	}

	verifyState()
}

type mockStore struct {
	collections map[string]map[string]basic.Document
	mu          sync.RWMutex
}

func newMockStore() *mockStore {
	return &mockStore{
		collections: make(map[string]map[string]basic.Document),
	}
}

func (m *mockStore) CreateCollection(ctx context.Context, name string, schema *basic.Schema) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[name] != nil {
		return errors.New("collection already exists")
	}

	m.collections[name] = make(map[string]basic.Document)
	return nil
}

func (m *mockStore) DropCollection(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[name] == nil {
		return basic.ErrNotFound
	}

	delete(m.collections, name)
	return nil
}

func (m *mockStore) Insert(ctx context.Context, collection string, doc basic.Document) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[collection] == nil {
		m.collections[collection] = make(map[string]basic.Document)
	}

	id := doc["id"].(string)

	docCopy := make(basic.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	m.collections[collection][id] = docCopy
	return id, nil
}

func (m *mockStore) Get(ctx context.Context, collection string, id string) (basic.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.collections[collection] == nil {
		return nil, basic.ErrNotFound
	}

	doc, exists := m.collections[collection][id]
	if !exists {
		return nil, basic.ErrNotFound
	}

	docCopy := make(basic.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	return docCopy, nil
}

func (m *mockStore) Update(ctx context.Context, collection string, id string, doc basic.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[collection] == nil {
		return basic.ErrNotFound
	}

	if _, exists := m.collections[collection][id]; !exists {
		return basic.ErrNotFound
	}

	docCopy := make(basic.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	m.collections[collection][id] = docCopy
	return nil
}

func (m *mockStore) Delete(ctx context.Context, collection string, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[collection] == nil {
		return basic.ErrNotFound
	}

	if _, exists := m.collections[collection][id]; !exists {
		return basic.ErrNotFound
	}

	delete(m.collections[collection], id)
	return nil
}

func (m *mockStore) Find(ctx context.Context, collection string, query basic.Query) ([]basic.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.collections[collection] == nil {
		return []basic.Document{}, nil
	}

	results := make([]basic.Document, 0)
	for _, doc := range m.collections[collection] {
		if matchesQuery(doc, query) {
			docCopy := make(basic.Document)
			for k, v := range doc {
				docCopy[k] = v
			}
			results = append(results, docCopy)
		}
	}

	return results, nil
}

func (m *mockStore) BeginTx(ctx context.Context) (basic.Tx, error) {
	return &mockTx{store: m}, nil
}

func (m *mockStore) Maintenance(ctx context.Context) error {
	return nil
}

func (m *mockStore) Close(ctx context.Context) error {
	return nil
}

type mockTx struct {
	store *mockStore
}

func (m *mockTx) Get(ctx context.Context, collection string, id string) (basic.Document, error) {
	return m.store.Get(ctx, collection, id)
}

func (m *mockTx) Insert(ctx context.Context, collection string, doc basic.Document) (string, error) {
	return m.store.Insert(ctx, collection, doc)
}

func (m *mockTx) Update(ctx context.Context, collection string, id string, doc basic.Document) error {
	return m.store.Update(ctx, collection, id, doc)
}

func (m *mockTx) Delete(ctx context.Context, collection string, id string) error {
	return m.store.Delete(ctx, collection, id)
}

func (m *mockTx) Find(ctx context.Context, collection string, query basic.Query) ([]basic.Document, error) {
	return m.store.Find(ctx, collection, query)
}

func (m *mockTx) BeginTx(ctx context.Context) (basic.Tx, error) {
	return m.store.BeginTx(ctx)
}

func (m *mockTx) CreateCollection(ctx context.Context, name string, schema *basic.Schema) error {
	return m.store.CreateCollection(ctx, name, schema)
}

func (m *mockTx) DropCollection(ctx context.Context, name string) error {
	return m.store.DropCollection(ctx, name)
}

func (m *mockTx) Maintenance(ctx context.Context) error {
	return m.store.Maintenance(ctx)
}

func (m *mockTx) Close(ctx context.Context) error {
	return nil
}

func (m *mockTx) Commit() error {
	return nil
}

func (m *mockTx) Rollback() error {
	return nil
}

func matchesQuery(doc basic.Document, query basic.Query) bool {
	for _, filter := range query.Filters {
		docValue, ok := doc[filter.Field]
		if !ok {
			return false
		}

		switch filter.Op {
		case basic.Gt:
			if docValueInt, ok := docValue.(int64); ok {
				if filterValueInt, ok := filter.Value.(int64); ok {
					if docValueInt <= filterValueInt {
						return false
					}
				}
			}
		case basic.Eq:
			if docValue != filter.Value {
				return false
			}
		}
	}

	return true
}

var _ = Describe("SyncAction", func() {
	var (
		action       *communicator.SyncAction
		ctx          context.Context
		orchestrator *MockOrchestrator
	)

	BeforeEach(func() {
		ctx = context.Background()
		orchestrator = NewMockOrchestrator()
		action = communicator.NewSyncAction(orchestrator)
	})

	Describe("Name", func() {
		It("should return action name", func() {
			Expect(action.Name()).To(Equal("Sync"))
		})
	})

	Describe("Execute", func() {
		Context("with successful tick", func() {
			It("should execute without error", func() {
				err := action.Execute(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should call orchestrator Tick", func() {
				err := action.Execute(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(orchestrator.TickCallCount()).To(Equal(1))
			})
		})

		Context("with tick failure", func() {
			BeforeEach(func() {
				orchestrator.SetTickError(errors.New("sync failed"))
			})

			It("should return error from tick", func() {
				err := action.Execute(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("sync failed"))
			})
		})

		Context("with context cancellation", func() {
			It("should respect context cancellation", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				orchestrator.SetTickError(context.Canceled)
				cancel()

				err := action.Execute(cancelCtx)
				Expect(err).To(MatchError(context.Canceled))
			})
		})
	})

	Describe("Idempotency", func() {
		Context("when executed multiple times", func() {
			It("should be idempotent", func() {
				VerifyActionIdempotency(action, 3, func() {
					Expect(orchestrator.TickCallCount()).To(Equal(3))
				})
			})
		})

		Context("when tick fails", func() {
			BeforeEach(func() {
				orchestrator.SetTickError(errors.New("temporary error"))
			})

			It("should allow retries", func() {
				err := action.Execute(ctx)
				Expect(err).To(HaveOccurred())

				orchestrator.SetTickError(nil)

				err = action.Execute(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
