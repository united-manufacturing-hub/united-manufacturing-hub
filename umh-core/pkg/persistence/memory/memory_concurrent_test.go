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
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Concurrent Operations", func() {
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
			store.Close(ctx)
		}
	})

	Describe("Concurrent Reads", func() {
		Context("same document", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":    "doc-1",
					"name":  "Test Document",
					"value": 42,
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle concurrent reads without corruption", func() {
				const numReaders = 50
				var wg sync.WaitGroup
				errors := make(chan error, numReaders)

				wg.Add(numReaders)
				for i := range numReaders {
					go func(readerID int) {
						defer GinkgoRecover()
						defer wg.Done()

						doc, err := store.Get(ctx, "test_collection", "doc-1")
						if err != nil {
							errors <- fmt.Errorf("reader %d failed: %w", readerID, err)

							return
						}

						if doc["name"] != "Test Document" {
							errors <- fmt.Errorf("reader %d got corrupted data: %v", readerID, doc["name"])

							return
						}
						if doc["value"] != 42 {
							errors <- fmt.Errorf("reader %d got corrupted value: %v", readerID, doc["value"])

							return
						}
					}(i)
				}

				wg.Wait()
				close(errors)

				Expect(errors).To(BeEmpty())
			})
		})

		Context("different documents", func() {
			BeforeEach(func() {
				const numDocs = 20
				for i := range numDocs {
					_, err := store.Insert(ctx, "test_collection", persistence.Document{
						"id":    fmt.Sprintf("doc-%d", i),
						"value": i * 10,
					})
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("should handle concurrent reads without corruption", func() {
				const numDocs = 20
				const readersPerDoc = 5
				var wg sync.WaitGroup
				errors := make(chan error, numDocs*readersPerDoc)

				for docID := 0; docID < numDocs; docID++ {
					for reader := range readersPerDoc {
						wg.Add(1)
						go func(docIdx, readerIdx int) {
							defer GinkgoRecover()
							defer wg.Done()

							docID := fmt.Sprintf("doc-%d", docIdx)
							doc, err := store.Get(ctx, "test_collection", docID)
							if err != nil {
								errors <- fmt.Errorf("doc %s reader %d failed: %w", docID, readerIdx, err)

								return
							}

							expectedValue := docIdx * 10
							if doc["value"] != expectedValue {
								errors <- fmt.Errorf("doc %s reader %d got wrong value: expected %d, got %v",
									docID, readerIdx, expectedValue, doc["value"])

								return
							}
						}(docID, reader)
					}
				}

				wg.Wait()
				close(errors)

				Expect(errors).To(BeEmpty())
			})
		})
	})

	Describe("Concurrent Writes", func() {
		Context("different documents", func() {
			It("should handle concurrent inserts without errors", func() {
				const numWriters = 50
				var wg sync.WaitGroup
				errors := make(chan error, numWriters)

				wg.Add(numWriters)
				for i := range numWriters {
					go func(writerID int) {
						defer GinkgoRecover()
						defer wg.Done()

						docID := fmt.Sprintf("doc-%d", writerID)
						_, err := store.Insert(ctx, "test_collection", persistence.Document{
							"id":      docID,
							"writer":  writerID,
							"message": fmt.Sprintf("Written by writer %d", writerID),
						})
						if err != nil {
							errors <- fmt.Errorf("writer %d failed: %w", writerID, err)
						}
					}(i)
				}

				wg.Wait()
				close(errors)

				Expect(errors).To(BeEmpty())

				docs, err := store.Find(ctx, "test_collection", persistence.Query{})
				Expect(err).ToNot(HaveOccurred())
				Expect(docs).To(HaveLen(numWriters))
			})
		})
	})

	Describe("Concurrent Updates", func() {
		BeforeEach(func() {
			const numDocs = 50
			for i := range numDocs {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":    fmt.Sprintf("doc-%d", i),
					"value": 0,
				})
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should handle concurrent updates without errors", func() {
			const numDocs = 50
			var wg sync.WaitGroup
			errors := make(chan error, numDocs)

			wg.Add(numDocs)
			for i := range numDocs {
				go func(docIdx int) {
					defer GinkgoRecover()
					defer wg.Done()

					docID := fmt.Sprintf("doc-%d", docIdx)
					err := store.Update(ctx, "test_collection", docID, persistence.Document{
						"id":    docID,
						"value": docIdx * 100,
					})
					if err != nil {
						errors <- fmt.Errorf("update doc %s failed: %w", docID, err)
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			Expect(errors).To(BeEmpty())

			for i := range numDocs {
				docID := fmt.Sprintf("doc-%d", i)
				doc, err := store.Get(ctx, "test_collection", docID)
				Expect(err).ToNot(HaveOccurred())
				expectedValue := i * 100
				Expect(doc["value"]).To(Equal(expectedValue))
			}
		})
	})

	Describe("Concurrent Deletes", func() {
		BeforeEach(func() {
			const numDocs = 50
			for i := range numDocs {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":   fmt.Sprintf("doc-%d", i),
					"data": fmt.Sprintf("Data %d", i),
				})
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should handle concurrent deletes without errors", func() {
			const numDocs = 50
			var wg sync.WaitGroup
			errors := make(chan error, numDocs)

			wg.Add(numDocs)
			for i := range numDocs {
				go func(docIdx int) {
					defer GinkgoRecover()
					defer wg.Done()

					docID := fmt.Sprintf("doc-%d", docIdx)
					err := store.Delete(ctx, "test_collection", docID)
					if err != nil {
						errors <- fmt.Errorf("delete doc %s failed: %w", docID, err)
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			Expect(errors).To(BeEmpty())

			docs, err := store.Find(ctx, "test_collection", persistence.Query{})
			Expect(err).ToNot(HaveOccurred())
			Expect(docs).To(BeEmpty())
		})
	})

	Describe("Mixed Read-Write Operations", func() {
		BeforeEach(func() {
			const numInitialDocs = 20
			for i := range numInitialDocs {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":    fmt.Sprintf("doc-%d", i),
					"value": i,
				})
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should handle concurrent reads, writes, and updates", func() {
			const numInitialDocs = 20
			const numReaders = 30
			const numWriters = 10
			const numUpdaters = 10

			var wg sync.WaitGroup
			errors := make(chan error, numReaders+numWriters+numUpdaters)

			for i := range numReaders {
				wg.Add(1)
				go func(readerID int) {
					defer GinkgoRecover()
					defer wg.Done()

					docID := fmt.Sprintf("doc-%d", readerID%numInitialDocs)
					doc, err := store.Get(ctx, "test_collection", docID)
					if err != nil {
						errors <- fmt.Errorf("reader %d failed: %w", readerID, err)

						return
					}
					if doc["id"] != docID {
						errors <- fmt.Errorf("reader %d got wrong document", readerID)
					}
				}(i)
			}

			for i := range numWriters {
				wg.Add(1)
				go func(writerID int) {
					defer GinkgoRecover()
					defer wg.Done()

					docID := fmt.Sprintf("new-doc-%d", writerID)
					_, err := store.Insert(ctx, "test_collection", persistence.Document{
						"id":    docID,
						"value": writerID + 1000,
					})
					if err != nil {
						errors <- fmt.Errorf("writer %d failed: %w", writerID, err)
					}
				}(i)
			}

			for i := range numUpdaters {
				wg.Add(1)
				go func(updaterID int) {
					defer GinkgoRecover()
					defer wg.Done()

					docID := fmt.Sprintf("doc-%d", updaterID%numInitialDocs)
					err := store.Update(ctx, "test_collection", docID, persistence.Document{
						"id":      docID,
						"value":   updaterID + 2000,
						"updated": true,
					})
					if err != nil {
						errors <- fmt.Errorf("updater %d failed: %w", updaterID, err)
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			Expect(errors).To(BeEmpty())

			docs, err := store.Find(ctx, "test_collection", persistence.Query{})
			Expect(err).ToNot(HaveOccurred())

			expectedTotal := numInitialDocs + numWriters
			Expect(docs).To(HaveLen(expectedTotal))
		})
	})

	Describe("Readers Get Consistent Snapshots", func() {
		BeforeEach(func() {
			_, err := store.Insert(ctx, "test_collection", persistence.Document{
				"id":     "doc-1",
				"field1": 1,
				"field2": 1,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should never see inconsistent snapshots", func() {
			const numReaders = 50
			const numWriters = 10
			var wg sync.WaitGroup
			errors := make(chan error, numReaders+numWriters)

			for i := range numReaders {
				wg.Add(1)
				go func(readerID int) {
					defer GinkgoRecover()
					defer wg.Done()

					doc, err := store.Get(ctx, "test_collection", "doc-1")
					if err != nil {
						errors <- fmt.Errorf("reader %d failed: %w", readerID, err)

						return
					}

					field1, ok1 := doc["field1"].(int)
					field2, ok2 := doc["field2"].(int)

					if !ok1 || !ok2 {
						errors <- fmt.Errorf("reader %d got non-int fields", readerID)

						return
					}

					if field1 != field2 {
						errors <- fmt.Errorf("reader %d saw inconsistent snapshot: field1=%d, field2=%d",
							readerID, field1, field2)
					}
				}(i)
			}

			for i := range numWriters {
				wg.Add(1)
				go func(writerID int) {
					defer GinkgoRecover()
					defer wg.Done()

					newValue := writerID + 100
					err := store.Update(ctx, "test_collection", "doc-1", persistence.Document{
						"id":     "doc-1",
						"field1": newValue,
						"field2": newValue,
					})
					if err != nil {
						errors <- fmt.Errorf("writer %d failed: %w", writerID, err)
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			Expect(errors).To(BeEmpty())
		})
	})

	Describe("Concurrent Transactions", func() {
		Context("independent documents", func() {
			It("should handle concurrent transactions without errors", func() {
				const numTransactions = 20
				var wg sync.WaitGroup
				errors := make(chan error, numTransactions)

				wg.Add(numTransactions)
				for i := range numTransactions {
					go func(txID int) {
						defer GinkgoRecover()
						defer wg.Done()

						tx, err := store.BeginTx(ctx)
						if err != nil {
							errors <- fmt.Errorf("tx %d begin failed: %w", txID, err)

							return
						}

						docID := fmt.Sprintf("tx-doc-%d", txID)
						_, err = tx.Insert(ctx, "test_collection", persistence.Document{
							"id":    docID,
							"txID":  txID,
							"value": txID * 10,
						})
						if err != nil {
							errors <- fmt.Errorf("tx %d insert failed: %w", txID, err)
							tx.Rollback()

							return
						}

						err = tx.Commit()
						if err != nil {
							errors <- fmt.Errorf("tx %d commit failed: %w", txID, err)
						}
					}(i)
				}

				wg.Wait()
				close(errors)

				Expect(errors).To(BeEmpty())

				docs, err := store.Find(ctx, "test_collection", persistence.Query{})
				Expect(err).ToNot(HaveOccurred())
				Expect(docs).To(HaveLen(numTransactions))
			})
		})

		Context("shared document with isolation", func() {
			BeforeEach(func() {
				_, err := store.Insert(ctx, "test_collection", persistence.Document{
					"id":    "shared-doc",
					"value": 0,
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should maintain transaction isolation", func() {
				const numReaders = 20
				const numWriters = 5
				var wg sync.WaitGroup
				errors := make(chan error, numReaders+numWriters)

				for i := range numReaders {
					wg.Add(1)
					go func(readerID int) {
						defer GinkgoRecover()
						defer wg.Done()

						tx, err := store.BeginTx(ctx)
						if err != nil {
							errors <- fmt.Errorf("reader tx %d begin failed: %w", readerID, err)

							return
						}
						defer tx.Rollback()

						doc, err := tx.Get(ctx, "test_collection", "shared-doc")
						if err != nil {
							errors <- fmt.Errorf("reader tx %d get failed: %w", readerID, err)

							return
						}

						if _, ok := doc["value"].(int); !ok {
							errors <- fmt.Errorf("reader tx %d got invalid value type", readerID)
						}
					}(i)
				}

				for i := range numWriters {
					wg.Add(1)
					go func(writerID int) {
						defer GinkgoRecover()
						defer wg.Done()

						tx, err := store.BeginTx(ctx)
						if err != nil {
							errors <- fmt.Errorf("writer tx %d begin failed: %w", writerID, err)

							return
						}

						err = tx.Update(ctx, "test_collection", "shared-doc", persistence.Document{
							"id":    "shared-doc",
							"value": writerID + 100,
						})
						if err != nil {
							errors <- fmt.Errorf("writer tx %d update failed: %w", writerID, err)
							tx.Rollback()

							return
						}

						err = tx.Commit()
						if err != nil {
							errors <- fmt.Errorf("writer tx %d commit failed: %w", writerID, err)
						}
					}(i)
				}

				wg.Wait()
				close(errors)

				Expect(errors).To(BeEmpty())

				doc, err := store.Get(ctx, "test_collection", "shared-doc")
				Expect(err).ToNot(HaveOccurred())
				_, ok := doc["value"].(int)
				Expect(ok).To(BeTrue())
			})
		})

		Context("commit and rollback", func() {
			It("should handle mixed commit and rollback", func() {
				const numTransactions = 30
				var wg sync.WaitGroup
				errors := make(chan error, numTransactions)

				wg.Add(numTransactions)
				for i := range numTransactions {
					go func(txID int) {
						defer GinkgoRecover()
						defer wg.Done()

						tx, err := store.BeginTx(ctx)
						if err != nil {
							errors <- fmt.Errorf("tx %d begin failed: %w", txID, err)

							return
						}

						docID := fmt.Sprintf("doc-%d", txID)
						_, err = tx.Insert(ctx, "test_collection", persistence.Document{
							"id":   docID,
							"data": txID,
						})
						if err != nil {
							errors <- fmt.Errorf("tx %d insert failed: %w", txID, err)
							tx.Rollback()

							return
						}

						if txID%2 == 0 {
							err = tx.Commit()
							if err != nil {
								errors <- fmt.Errorf("tx %d commit failed: %w", txID, err)
							}
						} else {
							err = tx.Rollback()
							if err != nil {
								errors <- fmt.Errorf("tx %d rollback failed: %w", txID, err)
							}
						}
					}(i)
				}

				wg.Wait()
				close(errors)

				Expect(errors).To(BeEmpty())

				docs, err := store.Find(ctx, "test_collection", persistence.Query{})
				Expect(err).ToNot(HaveOccurred())

				expectedCommitted := numTransactions / 2
				Expect(docs).To(HaveLen(expectedCommitted))

				for _, doc := range docs {
					txID, ok := doc["data"].(int)
					Expect(ok).To(BeTrue())
					Expect(txID % 2).To(Equal(0))
				}
			})
		})
	})

	Describe("Concurrent Collection Operations", func() {
		It("should handle concurrent collection creation and inserts", func() {
			const numCollections = 20
			var wg sync.WaitGroup
			errors := make(chan error, numCollections*2)

			for i := range numCollections {
				wg.Add(1)
				go func(collID int) {
					defer GinkgoRecover()
					defer wg.Done()

					collName := fmt.Sprintf("collection-%d", collID)
					err := store.CreateCollection(ctx, collName, nil)
					if err != nil {
						errors <- fmt.Errorf("create collection %s failed: %w", collName, err)
					}
				}(i)
			}

			wg.Wait()

			for i := range numCollections {
				wg.Add(1)
				go func(collID int) {
					defer GinkgoRecover()
					defer wg.Done()

					collName := fmt.Sprintf("collection-%d", collID)
					_, err := store.Insert(ctx, collName, persistence.Document{
						"id":   fmt.Sprintf("doc-%d", collID),
						"data": collID,
					})
					if err != nil {
						errors <- fmt.Errorf("insert into collection %s failed: %w", collName, err)
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			Expect(errors).To(BeEmpty())

			for i := range numCollections {
				collName := fmt.Sprintf("collection-%d", i)
				docs, err := store.Find(ctx, collName, persistence.Query{})
				Expect(err).ToNot(HaveOccurred())
				Expect(docs).To(HaveLen(1))
			}
		})
	})

	Describe("Concurrent Collection Drop", func() {
		BeforeEach(func() {
			const numCollections = 20
			for i := range numCollections {
				collName := fmt.Sprintf("collection-%d", i)
				err := store.CreateCollection(ctx, collName, nil)
				Expect(err).ToNot(HaveOccurred())
				_, err = store.Insert(ctx, collName, persistence.Document{
					"id":   "doc-1",
					"data": i,
				})
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should handle concurrent collection drops", func() {
			const numCollections = 20
			var wg sync.WaitGroup
			errors := make(chan error, numCollections)

			wg.Add(numCollections)
			for i := range numCollections {
				go func(collID int) {
					defer GinkgoRecover()
					defer wg.Done()

					collName := fmt.Sprintf("collection-%d", collID)
					err := store.DropCollection(ctx, collName)
					if err != nil {
						errors <- fmt.Errorf("drop collection %s failed: %w", collName, err)
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			Expect(errors).To(BeEmpty())

			for i := range numCollections {
				collName := fmt.Sprintf("collection-%d", i)
				_, err := store.Find(ctx, collName, persistence.Query{})
				Expect(err).To(HaveOccurred())
			}
		})
	})

	Describe("High Volume Stress Test", Label("stress"), func() {
		It("should handle high volume concurrent operations", func() {
			Skip("Skipping stress test in short mode")

			const numGoroutines = 100
			const operationsPerGoroutine = 100

			var wg sync.WaitGroup
			errors := make(chan error, numGoroutines*operationsPerGoroutine)

			for g := range numGoroutines {
				wg.Add(1)
				go func(goroutineID int) {
					defer GinkgoRecover()
					defer wg.Done()

					for op := range operationsPerGoroutine {
						docID := fmt.Sprintf("g%d-doc%d", goroutineID, op)

						_, err := store.Insert(ctx, "test_collection", persistence.Document{
							"id":          docID,
							"goroutineID": goroutineID,
							"operation":   op,
						})
						if err != nil {
							errors <- fmt.Errorf("g%d op%d insert failed: %w", goroutineID, op, err)

							continue
						}

						_, err = store.Get(ctx, "test_collection", docID)
						if err != nil {
							errors <- fmt.Errorf("g%d op%d get failed: %w", goroutineID, op, err)

							continue
						}

						err = store.Update(ctx, "test_collection", docID, persistence.Document{
							"id":          docID,
							"goroutineID": goroutineID,
							"operation":   op,
							"updated":     true,
						})
						if err != nil {
							errors <- fmt.Errorf("g%d op%d update failed: %w", goroutineID, op, err)

							continue
						}

						if op%2 == 0 {
							err = store.Delete(ctx, "test_collection", docID)
							if err != nil {
								errors <- fmt.Errorf("g%d op%d delete failed: %w", goroutineID, op, err)
							}
						}
					}
				}(g)
			}

			wg.Wait()
			close(errors)

			errorList := make([]error, 0)
			for err := range errors {
				errorList = append(errorList, err)
				if len(errorList) > 10 {
					break
				}
			}
			Expect(errorList).To(BeEmpty(), "Expected no errors but got: %v", errorList)

			docs, err := store.Find(ctx, "test_collection", persistence.Query{})
			Expect(err).ToNot(HaveOccurred())

			expectedDocs := numGoroutines * operationsPerGoroutine / 2
			Expect(docs).To(HaveLen(expectedDocs))
		})
	})

	Describe("Document Mutation Protection", func() {
		BeforeEach(func() {
			originalDoc := persistence.Document{
				"id":    "doc-1",
				"field": "original",
			}
			_, err := store.Insert(ctx, "test_collection", originalDoc)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should protect documents from concurrent mutation", func() {
			const numMutators = 50
			var wg sync.WaitGroup
			errors := make(chan error, numMutators)

			wg.Add(numMutators)
			for i := range numMutators {
				go func(mutatorID int) {
					defer GinkgoRecover()
					defer wg.Done()

					doc, err := store.Get(ctx, "test_collection", "doc-1")
					if err != nil {
						errors <- fmt.Errorf("mutator %d get failed: %w", mutatorID, err)

						return
					}

					doc["field"] = fmt.Sprintf("mutated-by-%d", mutatorID)
					doc["newfield"] = mutatorID

					retrievedAgain, err := store.Get(ctx, "test_collection", "doc-1")
					if err != nil {
						errors <- fmt.Errorf("mutator %d second get failed: %w", mutatorID, err)

						return
					}

					if retrievedAgain["field"] != "original" {
						errors <- fmt.Errorf("mutator %d saw mutation propagate to store", mutatorID)
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			Expect(errors).To(BeEmpty())

			finalDoc, err := store.Get(ctx, "test_collection", "doc-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDoc["field"]).To(Equal("original"))
			_, exists := finalDoc["newfield"]
			Expect(exists).To(BeFalse())
		})
	})
})
