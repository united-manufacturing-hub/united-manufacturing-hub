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
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

func TestConcurrentReads_SameDocument(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":    "doc-1",
		"name":  "Test Document",
		"value": 42,
	})

	const numReaders = 50
	var wg sync.WaitGroup
	errors := make(chan error, numReaders)

	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func(readerID int) {
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

	for err := range errors {
		t.Error(err)
	}
}

func TestConcurrentReads_DifferentDocuments(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	const numDocs = 20
	for i := 0; i < numDocs; i++ {
		store.Insert(ctx, "test_collection", persistence.Document{
			"id":    fmt.Sprintf("doc-%d", i),
			"value": i * 10,
		})
	}

	const readersPerDoc = 5
	var wg sync.WaitGroup
	errors := make(chan error, numDocs*readersPerDoc)

	for docID := 0; docID < numDocs; docID++ {
		for reader := 0; reader < readersPerDoc; reader++ {
			wg.Add(1)
			go func(docIdx, readerIdx int) {
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

	for err := range errors {
		t.Error(err)
	}
}

func TestConcurrentWrites_DifferentDocuments(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	const numWriters = 50
	var wg sync.WaitGroup
	errors := make(chan error, numWriters)

	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(writerID int) {
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

	for err := range errors {
		t.Error(err)
	}

	docs, err := store.Find(ctx, "test_collection", persistence.Query{})
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(docs) != numWriters {
		t.Errorf("Expected %d documents, got %d", numWriters, len(docs))
	}
}

func TestConcurrentUpdates_DifferentDocuments(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	const numDocs = 50
	for i := 0; i < numDocs; i++ {
		store.Insert(ctx, "test_collection", persistence.Document{
			"id":    fmt.Sprintf("doc-%d", i),
			"value": 0,
		})
	}

	var wg sync.WaitGroup
	errors := make(chan error, numDocs)

	wg.Add(numDocs)
	for i := 0; i < numDocs; i++ {
		go func(docIdx int) {
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

	for err := range errors {
		t.Error(err)
	}

	for i := 0; i < numDocs; i++ {
		docID := fmt.Sprintf("doc-%d", i)
		doc, err := store.Get(ctx, "test_collection", docID)
		if err != nil {
			t.Errorf("Get doc %s failed: %v", docID, err)
			continue
		}
		expectedValue := i * 100
		if doc["value"] != expectedValue {
			t.Errorf("Doc %s has wrong value: expected %d, got %v", docID, expectedValue, doc["value"])
		}
	}
}

func TestConcurrentDeletes_DifferentDocuments(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	const numDocs = 50
	for i := 0; i < numDocs; i++ {
		store.Insert(ctx, "test_collection", persistence.Document{
			"id":   fmt.Sprintf("doc-%d", i),
			"data": fmt.Sprintf("Data %d", i),
		})
	}

	var wg sync.WaitGroup
	errors := make(chan error, numDocs)

	wg.Add(numDocs)
	for i := 0; i < numDocs; i++ {
		go func(docIdx int) {
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

	for err := range errors {
		t.Error(err)
	}

	docs, err := store.Find(ctx, "test_collection", persistence.Query{})
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("Expected 0 documents after concurrent deletes, got %d", len(docs))
	}
}

func TestMixedReadWrite_ConcurrentOperations(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	const numInitialDocs = 20
	for i := 0; i < numInitialDocs; i++ {
		store.Insert(ctx, "test_collection", persistence.Document{
			"id":    fmt.Sprintf("doc-%d", i),
			"value": i,
		})
	}

	const numReaders = 30
	const numWriters = 10
	const numUpdaters = 10

	var wg sync.WaitGroup
	errors := make(chan error, numReaders+numWriters+numUpdaters)

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
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

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
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

	for i := 0; i < numUpdaters; i++ {
		wg.Add(1)
		go func(updaterID int) {
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

	for err := range errors {
		t.Error(err)
	}

	docs, err := store.Find(ctx, "test_collection", persistence.Query{})
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	expectedTotal := numInitialDocs + numWriters
	if len(docs) != expectedTotal {
		t.Errorf("Expected %d total documents, got %d", expectedTotal, len(docs))
	}
}

func TestReadersGetConsistentSnapshots(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":     "doc-1",
		"field1": 1,
		"field2": 1,
	})

	const numReaders = 50
	const numWriters = 10
	var wg sync.WaitGroup
	errors := make(chan error, numReaders+numWriters)

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
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

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
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

	for err := range errors {
		t.Error(err)
	}
}

func TestConcurrentTransactions_IndependentDocuments(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	const numTransactions = 20
	var wg sync.WaitGroup
	errors := make(chan error, numTransactions)

	wg.Add(numTransactions)
	for i := 0; i < numTransactions; i++ {
		go func(txID int) {
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

	for err := range errors {
		t.Error(err)
	}

	docs, err := store.Find(ctx, "test_collection", persistence.Query{})
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(docs) != numTransactions {
		t.Errorf("Expected %d documents from transactions, got %d", numTransactions, len(docs))
	}
}

func TestConcurrentTransactions_Isolation(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":    "shared-doc",
		"value": 0,
	})

	const numReaders = 20
	const numWriters = 5
	var wg sync.WaitGroup
	errors := make(chan error, numReaders+numWriters)

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
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

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
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

	for err := range errors {
		t.Error(err)
	}

	doc, err := store.Get(ctx, "test_collection", "shared-doc")
	if err != nil {
		t.Fatalf("Final get failed: %v", err)
	}
	if _, ok := doc["value"].(int); !ok {
		t.Errorf("Final document has invalid value")
	}
}

func TestConcurrentTransactions_CommitRollback(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	const numTransactions = 30
	var wg sync.WaitGroup
	errors := make(chan error, numTransactions)

	wg.Add(numTransactions)
	for i := 0; i < numTransactions; i++ {
		go func(txID int) {
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

	for err := range errors {
		t.Error(err)
	}

	docs, err := store.Find(ctx, "test_collection", persistence.Query{})
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	expectedCommitted := numTransactions / 2
	if len(docs) != expectedCommitted {
		t.Errorf("Expected %d committed documents, got %d", expectedCommitted, len(docs))
	}

	for _, doc := range docs {
		txID, ok := doc["data"].(int)
		if !ok {
			t.Errorf("Document has invalid data field")
			continue
		}
		if txID%2 != 0 {
			t.Errorf("Found document from rolled-back transaction: txID=%d", txID)
		}
	}
}

func TestConcurrentCollectionOperations(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	const numCollections = 20
	var wg sync.WaitGroup
	errors := make(chan error, numCollections*2)

	for i := 0; i < numCollections; i++ {
		wg.Add(1)
		go func(collID int) {
			defer wg.Done()

			collName := fmt.Sprintf("collection-%d", collID)
			err := store.CreateCollection(ctx, collName, nil)
			if err != nil {
				errors <- fmt.Errorf("create collection %s failed: %w", collName, err)
			}
		}(i)
	}

	wg.Wait()

	for i := 0; i < numCollections; i++ {
		wg.Add(1)
		go func(collID int) {
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

	for err := range errors {
		t.Error(err)
	}

	for i := 0; i < numCollections; i++ {
		collName := fmt.Sprintf("collection-%d", i)
		docs, err := store.Find(ctx, collName, persistence.Query{})
		if err != nil {
			t.Errorf("Find in collection %s failed: %v", collName, err)
			continue
		}
		if len(docs) != 1 {
			t.Errorf("Collection %s expected 1 document, got %d", collName, len(docs))
		}
	}
}

func TestConcurrentCollectionDrop(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	const numCollections = 20
	for i := 0; i < numCollections; i++ {
		collName := fmt.Sprintf("collection-%d", i)
		store.CreateCollection(ctx, collName, nil)
		store.Insert(ctx, collName, persistence.Document{
			"id":   "doc-1",
			"data": i,
		})
	}

	var wg sync.WaitGroup
	errors := make(chan error, numCollections)

	wg.Add(numCollections)
	for i := 0; i < numCollections; i++ {
		go func(collID int) {
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

	for err := range errors {
		t.Error(err)
	}

	for i := 0; i < numCollections; i++ {
		collName := fmt.Sprintf("collection-%d", i)
		_, err := store.Find(ctx, collName, persistence.Query{})
		if err == nil {
			t.Errorf("Collection %s should not exist after drop", collName)
		}
	}
}

func TestHighVolumeStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	const numGoroutines = 100
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for op := 0; op < operationsPerGoroutine; op++ {
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

	errorCount := 0
	for err := range errors {
		t.Error(err)
		errorCount++
		if errorCount > 10 {
			t.Fatal("Too many errors, stopping")
		}
	}

	docs, err := store.Find(ctx, "test_collection", persistence.Query{})
	if err != nil {
		t.Fatalf("Final find failed: %v", err)
	}

	expectedDocs := numGoroutines * operationsPerGoroutine / 2
	if len(docs) != expectedDocs {
		t.Errorf("Expected approximately %d documents after stress test, got %d", expectedDocs, len(docs))
	}
}

func TestDocumentMutationProtection(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	originalDoc := persistence.Document{
		"id":    "doc-1",
		"field": "original",
	}
	store.Insert(ctx, "test_collection", originalDoc)

	const numMutators = 50
	var wg sync.WaitGroup
	errors := make(chan error, numMutators)

	wg.Add(numMutators)
	for i := 0; i < numMutators; i++ {
		go func(mutatorID int) {
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

	for err := range errors {
		t.Error(err)
	}

	finalDoc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Final get failed: %v", err)
	}
	if finalDoc["field"] != "original" {
		t.Errorf("Store document was mutated: %v", finalDoc["field"])
	}
	if _, exists := finalDoc["newfield"]; exists {
		t.Errorf("Store document has field added by mutator")
	}
}
