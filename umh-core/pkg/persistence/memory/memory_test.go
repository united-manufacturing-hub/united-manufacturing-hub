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

var _ = Describe("InMemoryStore", func() {
	var (
		store *InMemoryStore
		ctx   context.Context
	)

	BeforeEach(func() {
		store = NewInMemoryStore()
		ctx = context.Background()
	})

	AfterEach(func() {
		if store != nil {
			store.Close(ctx)
		}
	})

	Describe("NewInMemoryStore", func() {
		It("should create a non-nil store", func() {
			Expect(store).ToNot(BeNil())
		})

		It("should initialize collections map", func() {
			Expect(store.collections).ToNot(BeNil())
		})

		It("should have empty collections map", func() {
			Expect(store.collections).To(BeEmpty())
		})
	})

	Describe("CreateCollection", func() {
		Context("when creating a new collection", func() {
			It("should succeed", func() {
				err := store.CreateCollection(ctx, "test_collection", nil)
				Expect(err).ToNot(HaveOccurred())
				_, exists := store.collections["test_collection"]
				Expect(exists).To(BeTrue())
			})
		})

		Context("when creating a duplicate collection", func() {
			BeforeEach(func() {
				err := store.CreateCollection(ctx, "existing_collection", nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return an error", func() {
				err := store.CreateCollection(ctx, "existing_collection", nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("already exists"))
			})
		})

		Context("when context is nil", func() {
			It("should return an error", func() {
				err := store.CreateCollection(nil, "test_collection", nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
			})
		})
	})

	Describe("DropCollection", func() {
		Context("when dropping an existing collection", func() {
			BeforeEach(func() {
				err := store.CreateCollection(ctx, "test_collection", nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should succeed", func() {
				err := store.DropCollection(ctx, "test_collection")
				Expect(err).ToNot(HaveOccurred())
				_, exists := store.collections["test_collection"]
				Expect(exists).To(BeFalse())
			})
		})

		Context("when dropping a non-existent collection", func() {
			It("should return an error", func() {
				err := store.DropCollection(ctx, "missing_collection")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not exist"))
			})
		})

		Context("when context is nil", func() {
			BeforeEach(func() {
				err := store.CreateCollection(ctx, "test_collection", nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return an error", func() {
				err := store.DropCollection(nil, "test_collection")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
			})
		})
	})

	Describe("Insert", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when inserting a valid document", func() {
			It("should succeed and return correct ID", func() {
				doc := persistence.Document{
					"id":    "doc-1",
					"name":  "Test Document",
					"value": 42,
				}
				id, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).ToNot(HaveOccurred())
				Expect(id).To(Equal("doc-1"))
			})
		})

		Context("when inserting a duplicate ID", func() {
			BeforeEach(func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Original",
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return conflict error", func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Duplicate",
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("conflict"))
			})
		})

		Context("when inserting into non-existent collection", func() {
			It("should return an error", func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Test",
				}
				_, err := store.Insert(ctx, "missing_collection", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not exist"))
			})
		})

		Context("when document has no id field", func() {
			It("should return an error", func() {
				doc := persistence.Document{
					"name": "No ID",
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("non-empty 'id' field"))
			})
		})

		Context("when document has empty id", func() {
			It("should return an error", func() {
				doc := persistence.Document{
					"id":   "",
					"name": "Empty ID",
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("non-empty 'id' field"))
			})
		})

		Context("when document has non-string id", func() {
			It("should return an error", func() {
				doc := persistence.Document{
					"id":   123,
					"name": "Numeric ID",
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("non-empty 'id' field"))
			})
		})

		Context("when context is nil", func() {
			It("should return an error", func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Test",
				}
				_, err := store.Insert(nil, "test_collection", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
			})
		})
	})

	Describe("Get", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when getting an existing document", func() {
			BeforeEach(func() {
				doc := persistence.Document{
					"id":    "doc-1",
					"name":  "Test Document",
					"value": 42,
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return the correct document", func() {
				doc, err := store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(doc["id"]).To(Equal("doc-1"))
				Expect(doc["name"]).To(Equal("Test Document"))
				Expect(doc["value"]).To(Equal(42))
			})
		})

		Context("when getting a missing document", func() {
			It("should return not found error", func() {
				_, err := store.Get(ctx, "test_collection", "missing-doc")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		Context("when getting from non-existent collection", func() {
			It("should return an error", func() {
				_, err := store.Get(ctx, "missing_collection", "doc-1")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not exist"))
			})
		})

		Context("when context is nil", func() {
			It("should return an error", func() {
				_, err := store.Get(nil, "test_collection", "doc-1")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
			})
		})
	})

	Describe("Update", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when updating an existing document", func() {
			BeforeEach(func() {
				doc := persistence.Document{
					"id":    "doc-1",
					"name":  "Original Document",
					"value": 42,
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should succeed and update the document", func() {
				updatedDoc := persistence.Document{
					"id":    "doc-1",
					"name":  "Updated Document",
					"value": 100,
				}
				err := store.Update(ctx, "test_collection", "doc-1", updatedDoc)
				Expect(err).ToNot(HaveOccurred())

				retrievedDoc, err := store.Get(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())
				Expect(retrievedDoc["name"]).To(Equal("Updated Document"))
				Expect(retrievedDoc["value"]).To(Equal(100))
			})
		})

		Context("when updating a missing document", func() {
			It("should return not found error", func() {
				doc := persistence.Document{
					"id":   "missing-doc",
					"name": "New Document",
				}
				err := store.Update(ctx, "test_collection", "missing-doc", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		Context("when updating in non-existent collection", func() {
			It("should return an error", func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Test",
				}
				err := store.Update(ctx, "missing_collection", "doc-1", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not exist"))
			})
		})

		Context("when context is nil", func() {
			BeforeEach(func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Original",
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return an error", func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Updated",
				}
				err := store.Update(nil, "test_collection", "doc-1", doc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
			})
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when deleting an existing document", func() {
			BeforeEach(func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Test Document",
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should succeed and remove the document", func() {
				err := store.Delete(ctx, "test_collection", "doc-1")
				Expect(err).ToNot(HaveOccurred())

				_, err = store.Get(ctx, "test_collection", "doc-1")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when deleting a missing document", func() {
			It("should return not found error", func() {
				err := store.Delete(ctx, "test_collection", "missing-doc")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		Context("when deleting from non-existent collection", func() {
			It("should return an error", func() {
				err := store.Delete(ctx, "missing_collection", "doc-1")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not exist"))
			})
		})

		Context("when context is nil", func() {
			BeforeEach(func() {
				doc := persistence.Document{
					"id":   "doc-1",
					"name": "Test",
				}
				_, err := store.Insert(ctx, "test_collection", doc)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return an error", func() {
				err := store.Delete(nil, "test_collection", "doc-1")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
			})
		})
	})

	Describe("Find", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when finding all documents", func() {
			BeforeEach(func() {
				docs := []persistence.Document{
					{"id": "doc-1", "name": "Document 1"},
					{"id": "doc-2", "name": "Document 2"},
					{"id": "doc-3", "name": "Document 3"},
				}
				for _, doc := range docs {
					_, err := store.Insert(ctx, "test_collection", doc)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("should return all documents", func() {
				docs, err := store.Find(ctx, "test_collection", persistence.Query{})
				Expect(err).ToNot(HaveOccurred())
				Expect(docs).To(HaveLen(3))
			})
		})

		Context("when finding in empty collection", func() {
			It("should return empty slice", func() {
				docs, err := store.Find(ctx, "test_collection", persistence.Query{})
				Expect(err).ToNot(HaveOccurred())
				Expect(docs).To(BeEmpty())
			})
		})

		Context("when finding in non-existent collection", func() {
			It("should return an error", func() {
				_, err := store.Find(ctx, "missing_collection", persistence.Query{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not exist"))
			})
		})

		Context("when context is nil", func() {
			It("should return an error", func() {
				_, err := store.Find(nil, "test_collection", persistence.Query{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
			})
		})
	})

	Describe("Document Isolation", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should isolate documents after insert", func() {
			originalDoc := persistence.Document{
				"id":    "doc-1",
				"name":  "Original",
				"value": 42,
			}
			_, err := store.Insert(ctx, "test_collection", originalDoc)
			Expect(err).ToNot(HaveOccurred())

			originalDoc["name"] = "Modified After Insert"
			originalDoc["value"] = 999

			retrievedDoc, err := store.Get(ctx, "test_collection", "doc-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedDoc["name"]).To(Equal("Original"))
			Expect(retrievedDoc["value"]).To(Equal(42))
		})

		It("should isolate returned documents from store", func() {
			originalDoc := persistence.Document{
				"id":    "doc-1",
				"name":  "Original",
				"value": 42,
			}
			_, err := store.Insert(ctx, "test_collection", originalDoc)
			Expect(err).ToNot(HaveOccurred())

			retrievedDoc, err := store.Get(ctx, "test_collection", "doc-1")
			Expect(err).ToNot(HaveOccurred())

			retrievedDoc["name"] = "Modified After Get"
			retrievedDoc["value"] = 777

			retrievedDoc2, err := store.Get(ctx, "test_collection", "doc-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedDoc2["name"]).To(Equal("Original"))
			Expect(retrievedDoc2["value"]).To(Equal(42))
		})
	})

	Describe("Maintenance", func() {
		It("should succeed without error", func() {
			err := store.Maintenance(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Close", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).ToNot(HaveOccurred())
			doc := persistence.Document{
				"id":   "doc-1",
				"name": "Test",
			}
			_, err = store.Insert(ctx, "test_collection", doc)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should clear all collections", func() {
			err := store.Close(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(store.collections).To(BeEmpty())
		})
	})
})
