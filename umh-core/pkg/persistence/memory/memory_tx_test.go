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

package memory

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Transaction Support", func() {
	var (
		store *InMemoryStore
		ctx   context.Context
	)

	BeforeEach(func() {
		store = NewInMemoryStore()
		ctx = context.Background()
		err := store.CreateCollection(ctx, "test_collection", nil)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if store != nil {
			defer func() { _ = store.Close(ctx) }()
		}
	})

	Describe("BeginTx", func() {
		Context("when beginning a transaction", func() {
			It("should return a non-nil transaction", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(tx).ToNot(BeNil())
			})
		})

		Context("when context is nil", func() {
			It("should return an error", func() {
				//nolint:staticcheck // testing nil context behavior
				tx, err := store.BeginTx(nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
				Expect(tx).To(BeNil())
			})
		})
	})

	Describe("Transaction Commit", func() {
		It("should commit changes to store", func() {
			tx, err := store.BeginTx(ctx)
			Expect(err).ToNot(HaveOccurred())

			_, err = tx.Insert(ctx, "test_collection", persistence.Document{
				"id":   "doc-1",
				"name": "Test Document",
			})
			Expect(err).ToNot(HaveOccurred())

			err = tx.Commit()
			Expect(err).ToNot(HaveOccurred())

			doc, err := store.Get(ctx, "test_collection", "doc-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(doc["name"]).To(Equal("Test Document"))
		})
	})

	Describe("Transaction Rollback", func() {
		It("should discard changes", func() {
			tx, err := store.BeginTx(ctx)
			Expect(err).ToNot(HaveOccurred())

			_, err = tx.Insert(ctx, "test_collection", persistence.Document{
				"id":   "doc-1",
				"name": "Test Document",
			})
			Expect(err).ToNot(HaveOccurred())

			err = tx.Rollback()
			Expect(err).ToNot(HaveOccurred())

			_, err = store.Get(ctx, "test_collection", "doc-1")
			Expect(err).To(Equal(persistence.ErrNotFound))
		})
	})

	Describe("Transaction Idempotency", func() {
		Context("when committing twice", func() {
			It("should be idempotent", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when rolling back twice", func() {
			It("should be idempotent", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Rollback()
				Expect(err).ToNot(HaveOccurred())

				err = tx.Rollback()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when committing after rollback", func() {
			It("should return an error", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Rollback()
				Expect(err).ToNot(HaveOccurred())

				err = tx.Commit()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("rolled back"))
			})
		})
	})

	Describe("Transaction Isolation", func() {
		Context("insert operations", func() {
			It("should not be visible before commit", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Test Document",
				})
				Expect(err).ToNot(HaveOccurred())

				_, err = store.Get(ctx, "test_collection", "doc-1")
				Expect(err).To(Equal(persistence.ErrNotFound))

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())

				doc, err := store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("Test Document"))
			})
		})

		Context("update operations", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":    "doc-1",
					"name":  "Original",
					"value": 42,
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should not be visible before commit", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
					"id":    "doc-1",
					"name":  "Updated",
					"value": 100,
				})
				Expect(err).ToNot(HaveOccurred())

				doc, err := store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("Original"))
				Expect(doc["value"]).To(Equal(42))

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())

				doc, err = store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("Updated"))
				Expect(doc["value"]).To(Equal(100))
			})
		})

		Context("delete operations", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Test Document",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should not be visible before commit", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Delete(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())

				doc, err := store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("Test Document"))

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())

				_, err = store.Get(ctx, "test_collection", "doc-1")
				Expect(err).To(Equal(persistence.ErrNotFound))
			})
		})
	})

	Describe("Transaction Sees Own Changes", func() {
		Context("insert operations", func() {
			It("should see own inserts", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Test Document",
				})
				Expect(err).ToNot(HaveOccurred())

				doc, err := tx.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("Test Document"))
			})
		})

		Context("update operations", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Original",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should see own updates", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
					"id":   "doc-1",
					"name": "Updated",
				})
				Expect(err).ToNot(HaveOccurred())

				doc, err := tx.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("Updated"))
			})
		})

		Context("delete operations", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Test Document",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should not see deleted documents", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Delete(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Get(ctx, "test_collection", "doc-1")
				Expect(err).To(Equal(persistence.ErrNotFound))
			})
		})
	})

	Describe("Transaction Rollback Behavior", func() {
		It("should make changes invisible after rollback", func() {
			tx, err := store.BeginTx(ctx)
			Expect(err).ToNot(HaveOccurred())

			_, err = tx.Insert(ctx, "test_collection", persistence.Document{
				"id":   "doc-1",
				"name": "Test Document",
			})
			Expect(err).ToNot(HaveOccurred())

			doc, err := tx.Get(ctx, "test_collection", "doc-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(doc["name"]).To(Equal("Test Document"))

			err = tx.Rollback()
			Expect(err).ToNot(HaveOccurred())

			_, err = store.Get(ctx, "test_collection", "doc-1")
			Expect(err).To(Equal(persistence.ErrNotFound))
		})
	})

	Describe("Transaction Operations", func() {
		Context("insert", func() {
			It("should succeed and commit changes", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				id, err := tx.Insert(ctx, "test_collection", persistence.Document{
					"id":    "doc-1",
					"name":  "Test Document",
					"value": 42,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(id).To(Equal("doc-1"))

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())

				doc, err := store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["value"]).To(Equal(42))
			})

			It("should return conflict for duplicate ID", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "First",
				})
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Duplicate",
				})
				Expect(err).To(Equal(persistence.ErrConflict))
			})

			It("should error for duplicate with existing store document", func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Existing",
				})
				Expect(err).ToNot(HaveOccurred())

				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Duplicate",
				})
				Expect(err).ToNot(HaveOccurred())

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("get", func() {
			It("should get from transaction cache", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "From Transaction",
				})
				Expect(err).ToNot(HaveOccurred())

				doc, err := tx.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("From Transaction"))
			})

			It("should get from underlying store", func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "From Store",
				})
				Expect(err).ToNot(HaveOccurred())

				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				doc, err := tx.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("From Store"))
			})
		})

		Context("update", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Original",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should succeed and commit changes", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
					"id":   "doc-1",
					"name": "Updated",
				})
				Expect(err).ToNot(HaveOccurred())

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())

				doc, err := store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["name"]).To(Equal("Updated"))
			})

			It("should succeed for missing document", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Update(ctx, "test_collection", "missing-doc", persistence.Document{
					"id":   "missing-doc",
					"name": "Updated",
				})
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("delete", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "To Delete",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should succeed and commit changes", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Delete(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())

				_, err = store.Get(ctx, "test_collection", "doc-1")
				Expect(err).To(Equal(persistence.ErrNotFound))
			})

			It("should succeed for missing document", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Delete(ctx, "test_collection", "missing-doc")
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("find", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Document 1",
				})
				Expect(err).ToNot(HaveOccurred())
				_, err = store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-2",
					"name": "Document 2",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should find documents from store", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				docs, err := tx.Find(ctx, "test_collection", persistence.Query{})
				Expect(err).ToNot(HaveOccurred())
				Expect(docs).To(HaveLen(2))
			})
		})
	})

	Describe("Nested Transactions", func() {
		It("should not support nested transactions", func() {
			tx, err := store.BeginTx(ctx)
			Expect(err).ToNot(HaveOccurred())

			nestedTx, err := tx.BeginTx(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nested"))
			Expect(nestedTx).To(BeNil())
		})
	})

	Describe("Operations After Commit", func() {
		var tx persistence.Tx

		BeforeEach(func() {
			var err error
			tx, err = store.BeginTx(ctx)
			Expect(err).ToNot(HaveOccurred())
			err = tx.Commit()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should error on insert", func() {
			_, err := tx.Insert(ctx, "test_collection", persistence.Document{
				"id":   "doc-1",
				"name": "After Commit",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))
		})

		It("should error on get", func() {
			_, err := tx.Get(ctx, "test_collection", "doc-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))
		})

		It("should error on update", func() {
			err := tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
				"id":   "doc-1",
				"name": "Updated",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))
		})

		It("should error on delete", func() {
			err := tx.Delete(ctx, "test_collection", "doc-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))
		})
	})

	Describe("Operations After Rollback", func() {
		var tx persistence.Tx

		BeforeEach(func() {
			var err error
			tx, err = store.BeginTx(ctx)
			Expect(err).ToNot(HaveOccurred())
			err = tx.Rollback()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should error on insert", func() {
			_, err := tx.Insert(ctx, "test_collection", persistence.Document{
				"id":   "doc-1",
				"name": "After Rollback",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))
		})

		It("should error on get", func() {
			_, err := tx.Get(ctx, "test_collection", "doc-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))
		})

		It("should error on update", func() {
			err := tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
				"id":   "doc-1",
				"name": "Updated",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))
		})

		It("should error on delete", func() {
			err := tx.Delete(ctx, "test_collection", "doc-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))
		})
	})

	Describe("Transaction Edge Cases", func() {
		It("should auto-create collection on commit", func() {
			tx, err := store.BeginTx(ctx)
			Expect(err).ToNot(HaveOccurred())

			_, err = tx.Insert(ctx, "missing_collection", persistence.Document{
				"id":   "doc-1",
				"name": "Test",
			})
			Expect(err).ToNot(HaveOccurred())

			err = tx.Commit()
			Expect(err).ToNot(HaveOccurred())

			_, exists := store.collections["missing_collection"]
			Expect(exists).To(BeTrue())

			doc, err := store.Get(ctx, "missing_collection", "doc-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(doc["name"]).To(Equal("Test"))
		})

		Context("delete then insert same ID", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Original",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should error", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Delete(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "New Document",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("deleted"))
			})
		})

		Context("get deleted document", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Test",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return not found", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Delete(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Get(ctx, "test_collection", "doc-1")
				Expect(err).To(Equal(persistence.ErrNotFound))
			})
		})

		Context("update deleted document", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Original",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return not found", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = tx.Delete(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())

				err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
					"id":   "doc-1",
					"name": "Updated",
				})
				Expect(err).To(Equal(persistence.ErrNotFound))
			})
		})

		Context("multiple operations same document", func() {
			It("should chain operations correctly", func() {
				tx, err := store.BeginTx(ctx)
				Expect(err).ToNot(HaveOccurred())

				_, err = tx.Insert(ctx, "test_collection", persistence.Document{
					"id":    "doc-1",
					"name":  "Original",
					"value": 10,
				})
				Expect(err).ToNot(HaveOccurred())

				err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
					"id":    "doc-1",
					"name":  "Updated",
					"value": 20,
				})
				Expect(err).ToNot(HaveOccurred())

				doc, err := tx.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["value"]).To(Equal(20))

				err = tx.Commit()
				Expect(err).ToNot(HaveOccurred())

				finalDoc, err := store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(finalDoc["value"]).To(Equal(20))
			})
		})
	})
})
