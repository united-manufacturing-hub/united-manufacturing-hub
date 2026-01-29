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

package persistence_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

type mockStore struct {
	beginTxCalled int
	beginTxErr    error
	txCommitted   bool
	txRolledBack  bool
}

func (m *mockStore) BeginTx(ctx context.Context) (persistence.Tx, error) {
	m.beginTxCalled++
	if m.beginTxErr != nil {
		return nil, m.beginTxErr
	}

	return &mockTx{store: m}, nil
}

func (m *mockStore) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	panic("not implemented")
}

func (m *mockStore) DropCollection(ctx context.Context, name string) error {
	panic("not implemented")
}

func (m *mockStore) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	panic("not implemented")
}

func (m *mockStore) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	panic("not implemented")
}

func (m *mockStore) Update(ctx context.Context, collection string, id string, doc persistence.Document) error {
	panic("not implemented")
}

func (m *mockStore) Delete(ctx context.Context, collection string, id string) error {
	panic("not implemented")
}

func (m *mockStore) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	panic("not implemented")
}

func (m *mockStore) Maintenance(ctx context.Context) error {
	panic("not implemented")
}

func (m *mockStore) Close(ctx context.Context) error {
	panic("not implemented")
}

type mockTx struct {
	store *mockStore
}

func (t *mockTx) Commit() error {
	t.store.txCommitted = true

	return nil
}

func (t *mockTx) Rollback() error {
	t.store.txRolledBack = true

	return nil
}

func (t *mockTx) BeginTx(ctx context.Context) (persistence.Tx, error) {
	panic("not implemented")
}

func (t *mockTx) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	return nil
}

func (t *mockTx) DropCollection(ctx context.Context, name string) error {
	return nil
}

func (t *mockTx) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	return "", nil
}

func (t *mockTx) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	return nil, nil
}

func (t *mockTx) Update(ctx context.Context, collection string, id string, doc persistence.Document) error {
	return nil
}

func (t *mockTx) Delete(ctx context.Context, collection string, id string) error {
	return nil
}

func (t *mockTx) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	return nil, nil
}

func (t *mockTx) Maintenance(ctx context.Context) error {
	panic("not implemented")
}

func (t *mockTx) Close(ctx context.Context) error {
	panic("not implemented")
}

var _ = Describe("Transaction", func() {
	var store *mockStore

	BeforeEach(func() {
		store = &mockStore{}
	})

	Describe("WithTransaction", func() {
		It("should commit on success", func() {
			err := persistence.WithTransaction(context.Background(), store, func(tx persistence.Tx) error {
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(store.txCommitted).To(BeTrue())
			Expect(store.txRolledBack).To(BeFalse())
		})

		It("should rollback on error", func() {
			expectedErr := errors.New("transaction failed")

			err := persistence.WithTransaction(context.Background(), store, func(tx persistence.Tx) error {
				return expectedErr
			})

			Expect(err).To(HaveOccurred())
			Expect(store.txCommitted).To(BeFalse())
			Expect(store.txRolledBack).To(BeTrue())
		})

		It("should rollback on panic and re-panic", func() {
			defer func() {
				r := recover()
				Expect(r).NotTo(BeNil())
				Expect(store.txRolledBack).To(BeTrue())
				Expect(store.txCommitted).To(BeFalse())
			}()

			_ = persistence.WithTransaction(context.Background(), store, func(tx persistence.Tx) error {
				panic("something went wrong")
			})
		})

		It("should return error when context is cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := persistence.WithTransaction(ctx, store, func(tx persistence.Tx) error {
				return nil
			})

			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
			Expect(store.txCommitted).To(BeFalse())
		})
	})

	Describe("WithRetry", func() {
		It("should succeed on first attempt", func() {
			callCount := 0

			err := persistence.WithRetry(context.Background(), store, 3, func(tx persistence.Tx) error {
				callCount++

				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(1))
		})

		It("should retry on conflict until success", func() {
			callCount := 0

			err := persistence.WithRetry(context.Background(), store, 3, func(tx persistence.Tx) error {
				callCount++
				if callCount < 3 {
					return persistence.ErrConflict
				}

				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(3))
		})

		It("should return error when max retries exceeded", func() {
			callCount := 0

			err := persistence.WithRetry(context.Background(), store, 3, func(tx persistence.Tx) error {
				callCount++

				return persistence.ErrConflict
			})

			Expect(errors.Is(err, persistence.ErrConflict)).To(BeTrue())
			Expect(callCount).To(Equal(4))
		})

		It("should not retry for non-conflict errors", func() {
			callCount := 0
			otherErr := errors.New("other error")

			err := persistence.WithRetry(context.Background(), store, 3, func(tx persistence.Tx) error {
				callCount++

				return otherErr
			})

			Expect(err).To(HaveOccurred())
			Expect(callCount).To(Equal(1))
		})

		It("should return error for negative maxRetries", func() {
			err := persistence.WithRetry(context.Background(), store, -1, func(tx persistence.Tx) error {
				return nil
			})

			Expect(err).To(HaveOccurred())
			Expect(store.beginTxCalled).To(Equal(0))
		})
	})
})
