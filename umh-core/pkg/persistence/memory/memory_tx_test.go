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
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

func TestBeginTx(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Expected no error from BeginTx, got %v", err)
	}
	if tx == nil {
		t.Fatal("Expected non-nil transaction")
	}
}

func TestTransactionCommit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test Document",
	})
	if err != nil {
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Expected successful commit, got error: %v", err)
	}

	doc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Expected document to exist after commit: %v", err)
	}
	if doc["name"] != "Test Document" {
		t.Errorf("Expected document name 'Test Document', got %v", doc["name"])
	}
}

func TestTransactionRollback(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test Document",
	})
	if err != nil {
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Expected successful rollback, got error: %v", err)
	}

	_, err = store.Get(ctx, "test_collection", "doc-1")
	if err != persistence.ErrNotFound {
		t.Errorf("Expected ErrNotFound after rollback, got %v", err)
	}
}

func TestCommitAfterCommit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("First commit failed: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Errorf("Expected second commit to be idempotent (no error), got %v", err)
	}
}

func TestRollbackAfterRollback(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("First rollback failed: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Errorf("Expected second rollback to be idempotent (no error), got %v", err)
	}
}

func TestCommitAfterRollback(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	err = tx.Commit()
	if err == nil {
		t.Error("Expected error when committing after rollback")
	}
	if !contains(err.Error(), "rolled back") {
		t.Errorf("Expected error about rollback, got %v", err)
	}
}

func TestTransactionIsolationInsert(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test Document",
	})
	if err != nil {
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	_, err = store.Get(ctx, "test_collection", "doc-1")
	if err != persistence.ErrNotFound {
		t.Errorf("Expected document to be invisible before commit, got error: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	doc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Expected document to exist after commit: %v", err)
	}
	if doc["name"] != "Test Document" {
		t.Errorf("Expected document name 'Test Document', got %v", doc["name"])
	}
}

func TestTransactionIsolationUpdate(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":    "doc-1",
		"name":  "Original",
		"value": 42,
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
		"id":    "doc-1",
		"name":  "Updated",
		"value": 100,
	})
	if err != nil {
		t.Fatalf("Failed to update in transaction: %v", err)
	}

	doc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}
	if doc["name"] != "Original" {
		t.Errorf("Expected original name before commit, got %v", doc["name"])
	}
	if doc["value"] != 42 {
		t.Errorf("Expected original value before commit, got %v", doc["value"])
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	doc, err = store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to get document after commit: %v", err)
	}
	if doc["name"] != "Updated" {
		t.Errorf("Expected updated name after commit, got %v", doc["name"])
	}
	if doc["value"] != 100 {
		t.Errorf("Expected updated value after commit, got %v", doc["value"])
	}
}

func TestTransactionIsolationDelete(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test Document",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to delete in transaction: %v", err)
	}

	doc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Errorf("Expected document to still exist before commit, got error: %v", err)
	}
	if doc["name"] != "Test Document" {
		t.Errorf("Expected original document before commit, got %v", doc)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	_, err = store.Get(ctx, "test_collection", "doc-1")
	if err != persistence.ErrNotFound {
		t.Errorf("Expected ErrNotFound after commit, got %v", err)
	}
}

func TestTransactionSeesOwnInsert(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test Document",
	})
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	doc, err := tx.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Transaction should see its own insert: %v", err)
	}
	if doc["name"] != "Test Document" {
		t.Errorf("Expected document name 'Test Document', got %v", doc["name"])
	}
}

func TestTransactionSeesOwnUpdate(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Original",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
		"id":   "doc-1",
		"name": "Updated",
	})
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}

	doc, err := tx.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Transaction should see its own update: %v", err)
	}
	if doc["name"] != "Updated" {
		t.Errorf("Expected updated name 'Updated', got %v", doc["name"])
	}
}

func TestTransactionDoesNotSeeOwnDelete(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test Document",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	_, err = tx.Get(ctx, "test_collection", "doc-1")
	if err != persistence.ErrNotFound {
		t.Errorf("Transaction should not see deleted document, got error: %v", err)
	}
}

func TestTransactionRollbackMakesChangesInvisible(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test Document",
	})
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	doc, err := tx.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Should see document before rollback: %v", err)
	}
	if doc["name"] != "Test Document" {
		t.Errorf("Expected document name 'Test Document', got %v", doc["name"])
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	_, err = store.Get(ctx, "test_collection", "doc-1")
	if err != persistence.ErrNotFound {
		t.Errorf("Expected ErrNotFound after rollback, got %v", err)
	}
}

func TestTransactionInsertSuccess(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	id, err := tx.Insert(ctx, "test_collection", persistence.Document{
		"id":    "doc-1",
		"name":  "Test Document",
		"value": 42,
	})
	if err != nil {
		t.Fatalf("Expected successful insert, got error: %v", err)
	}
	if id != "doc-1" {
		t.Errorf("Expected ID 'doc-1', got %q", id)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	doc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}
	if doc["value"] != 42 {
		t.Errorf("Expected value 42, got %v", doc["value"])
	}
}

func TestTransactionInsertDuplicateID(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "First",
	})
	if err != nil {
		t.Fatalf("First insert failed: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Duplicate",
	})
	if err != persistence.ErrConflict {
		t.Errorf("Expected ErrConflict for duplicate ID, got %v", err)
	}
}

func TestTransactionGetFromTransactionCache(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "From Transaction",
	})
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	doc, err := tx.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Get should succeed from transaction cache: %v", err)
	}
	if doc["name"] != "From Transaction" {
		t.Errorf("Expected name 'From Transaction', got %v", doc["name"])
	}
}

func TestTransactionGetFromUnderlyingStore(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "From Store",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	doc, err := tx.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Get should succeed from underlying store: %v", err)
	}
	if doc["name"] != "From Store" {
		t.Errorf("Expected name 'From Store', got %v", doc["name"])
	}
}

func TestTransactionUpdateSuccess(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Original",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
		"id":   "doc-1",
		"name": "Updated",
	})
	if err != nil {
		t.Fatalf("Expected successful update, got error: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	doc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}
	if doc["name"] != "Updated" {
		t.Errorf("Expected updated name, got %v", doc["name"])
	}
}

func TestTransactionUpdateMissingDocument(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Update(ctx, "test_collection", "missing-doc", persistence.Document{
		"id":   "missing-doc",
		"name": "Updated",
	})
	if err != nil {
		t.Errorf("Update of non-existent document should succeed in transaction (creates entry)")
	}
}

func TestTransactionDeleteSuccess(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "To Delete",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Expected successful delete, got error: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	_, err = store.Get(ctx, "test_collection", "doc-1")
	if err != persistence.ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got %v", err)
	}
}

func TestTransactionDeleteMissingDocument(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "missing-doc")
	if err != nil {
		t.Errorf("Delete of non-existent document should succeed (marks as deleted)")
	}
}

func TestTransactionFind(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Document 1",
	})
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-2",
		"name": "Document 2",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	docs, err := tx.Find(ctx, "test_collection", persistence.Query{})
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("Expected 2 documents, got %d", len(docs))
	}
}

func TestNestedTransactionsNotSupported(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	nestedTx, err := tx.BeginTx(ctx)
	if err == nil {
		t.Error("Expected error for nested transaction")
	}
	if nestedTx != nil {
		t.Error("Expected nil transaction for nested transaction")
	}
	if !contains(err.Error(), "nested") {
		t.Errorf("Expected error about nested transactions, got %v", err)
	}
}

func TestBeginTxWithNilContext(t *testing.T) {
	store := NewInMemoryStore()

	tx, err := store.BeginTx(nil)
	if err == nil {
		t.Error("Expected error with nil context")
	}
	if tx != nil {
		t.Error("Expected nil transaction with nil context")
	}
	if !contains(err.Error(), "context cannot be nil") {
		t.Errorf("Expected error about nil context, got %v", err)
	}
}

func TestInsertAfterCommit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "After Commit",
	})
	if err == nil {
		t.Error("Expected error inserting after commit")
	}
	if !contains(err.Error(), "already completed") {
		t.Errorf("Expected error about completed transaction, got %v", err)
	}
}

func TestGetAfterCommit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	_, err = tx.Get(ctx, "test_collection", "doc-1")
	if err == nil {
		t.Error("Expected error getting after commit")
	}
	if !contains(err.Error(), "already completed") {
		t.Errorf("Expected error about completed transaction, got %v", err)
	}
}

func TestUpdateAfterCommit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Original",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
		"id":   "doc-1",
		"name": "Updated",
	})
	if err == nil {
		t.Error("Expected error updating after commit")
	}
	if !contains(err.Error(), "already completed") {
		t.Errorf("Expected error about completed transaction, got %v", err)
	}
}

func TestDeleteAfterCommit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "doc-1")
	if err == nil {
		t.Error("Expected error deleting after commit")
	}
	if !contains(err.Error(), "already completed") {
		t.Errorf("Expected error about completed transaction, got %v", err)
	}
}

func TestInsertAfterRollback(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "After Rollback",
	})
	if err == nil {
		t.Error("Expected error inserting after rollback")
	}
	if !contains(err.Error(), "already completed") {
		t.Errorf("Expected error about completed transaction, got %v", err)
	}
}

func TestGetAfterRollback(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	_, err = tx.Get(ctx, "test_collection", "doc-1")
	if err == nil {
		t.Error("Expected error getting after rollback")
	}
	if !contains(err.Error(), "already completed") {
		t.Errorf("Expected error about completed transaction, got %v", err)
	}
}

func TestUpdateAfterRollback(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Original",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
		"id":   "doc-1",
		"name": "Updated",
	})
	if err == nil {
		t.Error("Expected error updating after rollback")
	}
	if !contains(err.Error(), "already completed") {
		t.Errorf("Expected error about completed transaction, got %v", err)
	}
}

func TestDeleteAfterRollback(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "doc-1")
	if err == nil {
		t.Error("Expected error deleting after rollback")
	}
	if !contains(err.Error(), "already completed") {
		t.Errorf("Expected error about completed transaction, got %v", err)
	}
}

func TestTransactionOnNonExistentCollection(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "missing_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test",
	})
	if err != nil {
		t.Errorf("Insert should succeed (will fail on commit if collection missing)")
	}

	err = tx.Commit()
	if err == nil {
		t.Error("Expected error committing to non-existent collection")
	}
	if !contains(err.Error(), "does not exist") {
		t.Errorf("Expected error about collection not existing, got %v", err)
	}
}

func TestTransactionInsertDuplicateWithExistingDocument(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Existing",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Duplicate",
	})
	if err != nil {
		t.Errorf("Insert should succeed in transaction even if document exists in store")
	}

	err = tx.Commit()
	if err != nil {
		t.Errorf("Commit should succeed (transaction insert overwrites existing)")
	}
}

func TestTransactionDeleteThenInsertSameID(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Original",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "New Document",
	})
	if err == nil {
		t.Error("Expected error inserting after delete in same transaction")
	}
	if !contains(err.Error(), "deleted") {
		t.Errorf("Expected error about deleted document, got %v", err)
	}
}

func TestTransactionGetDeletedDocument(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = tx.Get(ctx, "test_collection", "doc-1")
	if err != persistence.ErrNotFound {
		t.Errorf("Expected ErrNotFound for deleted document, got %v", err)
	}
}

func TestTransactionUpdateDeletedDocument(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Original",
	})

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Delete(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
		"id":   "doc-1",
		"name": "Updated",
	})
	if err != persistence.ErrNotFound {
		t.Errorf("Expected ErrNotFound updating deleted document, got %v", err)
	}
}

func TestTransactionMultipleOperationsSameDocument(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.Insert(ctx, "test_collection", persistence.Document{
		"id":    "doc-1",
		"name":  "Original",
		"value": 10,
	})
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	err = tx.Update(ctx, "test_collection", "doc-1", persistence.Document{
		"id":    "doc-1",
		"name":  "Updated",
		"value": 20,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	doc, err := tx.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if doc["value"] != 20 {
		t.Errorf("Expected value 20 after update, got %v", doc["value"])
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	finalDoc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to get final document: %v", err)
	}
	if finalDoc["value"] != 20 {
		t.Errorf("Expected final value 20, got %v", finalDoc["value"])
	}
}
