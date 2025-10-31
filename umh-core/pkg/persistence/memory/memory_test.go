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

func TestNewInMemoryStore(t *testing.T) {
	store := NewInMemoryStore()
	if store == nil {
		t.Fatal("Expected non-nil store")
	}
	if store.collections == nil {
		t.Fatal("Expected collections map to be initialized")
	}
	if len(store.collections) != 0 {
		t.Errorf("Expected empty collections map, got %d entries", len(store.collections))
	}
}

func TestCreateCollection(t *testing.T) {
	tests := []struct {
		name          string
		collectionName string
		setup         func(store *InMemoryStore)
		wantErr       bool
		errContains   string
	}{
		{
			name:           "create new collection",
			collectionName: "test_collection",
			setup:          func(store *InMemoryStore) {},
			wantErr:        false,
		},
		{
			name:           "create duplicate collection",
			collectionName: "existing_collection",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "existing_collection", nil)
			},
			wantErr:     true,
			errContains: "already exists",
		},
		{
			name:           "create with nil context",
			collectionName: "test_collection",
			setup: func(store *InMemoryStore) {
				// Test will provide nil context
			},
			wantErr:     true,
			errContains: "context cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryStore()
			tt.setup(store)

			ctx := context.Background()
			if tt.errContains == "context cannot be nil" {
				ctx = nil
			}

			err := store.CreateCollection(ctx, tt.collectionName, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if _, exists := store.collections[tt.collectionName]; !exists {
					t.Errorf("Expected collection %q to exist", tt.collectionName)
				}
			}
		})
	}
}

func TestDropCollection(t *testing.T) {
	tests := []struct {
		name          string
		collectionName string
		setup         func(store *InMemoryStore)
		wantErr       bool
		errContains   string
	}{
		{
			name:           "drop existing collection",
			collectionName: "test_collection",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr: false,
		},
		{
			name:           "drop non-existent collection",
			collectionName: "missing_collection",
			setup:          func(store *InMemoryStore) {},
			wantErr:        true,
			errContains:    "does not exist",
		},
		{
			name:           "drop with nil context",
			collectionName: "test_collection",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "context cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryStore()
			tt.setup(store)

			ctx := context.Background()
			if tt.errContains == "context cannot be nil" {
				ctx = nil
			}

			err := store.DropCollection(ctx, tt.collectionName)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if _, exists := store.collections[tt.collectionName]; exists {
					t.Errorf("Expected collection %q to be dropped", tt.collectionName)
				}
			}
		})
	}
}

func TestInsert(t *testing.T) {
	tests := []struct {
		name        string
		collection  string
		doc         persistence.Document
		setup       func(store *InMemoryStore)
		wantErr     bool
		errContains string
	}{
		{
			name:       "insert valid document",
			collection: "test_collection",
			doc: persistence.Document{
				"id":    "doc-1",
				"name":  "Test Document",
				"value": 42,
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr: false,
		},
		{
			name:       "insert duplicate ID",
			collection: "test_collection",
			doc: persistence.Document{
				"id":   "doc-1",
				"name": "Duplicate",
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Original",
				})
			},
			wantErr:     true,
			errContains: "conflict",
		},
		{
			name:       "insert into non-existent collection",
			collection: "missing_collection",
			doc: persistence.Document{
				"id":   "doc-1",
				"name": "Test",
			},
			setup:       func(store *InMemoryStore) {},
			wantErr:     true,
			errContains: "does not exist",
		},
		{
			name:       "insert document without id",
			collection: "test_collection",
			doc: persistence.Document{
				"name": "No ID",
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "non-empty 'id' field",
		},
		{
			name:       "insert document with empty id",
			collection: "test_collection",
			doc: persistence.Document{
				"id":   "",
				"name": "Empty ID",
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "non-empty 'id' field",
		},
		{
			name:       "insert document with non-string id",
			collection: "test_collection",
			doc: persistence.Document{
				"id":   123,
				"name": "Numeric ID",
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "non-empty 'id' field",
		},
		{
			name:       "insert with nil context",
			collection: "test_collection",
			doc: persistence.Document{
				"id":   "doc-1",
				"name": "Test",
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "context cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryStore()
			tt.setup(store)

			ctx := context.Background()
			if tt.errContains == "context cannot be nil" {
				ctx = nil
			}

			id, err := store.Insert(ctx, tt.collection, tt.doc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				expectedID := tt.doc["id"].(string)
				if id != expectedID {
					t.Errorf("Expected ID %q, got %q", expectedID, id)
				}
			}
		})
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name        string
		collection  string
		id          string
		setup       func(store *InMemoryStore)
		wantErr     bool
		errContains string
		wantDoc     persistence.Document
	}{
		{
			name:       "get existing document",
			collection: "test_collection",
			id:         "doc-1",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":    "doc-1",
					"name":  "Test Document",
					"value": 42,
				})
			},
			wantErr: false,
			wantDoc: persistence.Document{
				"id":    "doc-1",
				"name":  "Test Document",
				"value": 42,
			},
		},
		{
			name:       "get missing document",
			collection: "test_collection",
			id:         "missing-doc",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:       "get from non-existent collection",
			collection: "missing_collection",
			id:         "doc-1",
			setup:      func(store *InMemoryStore) {},
			wantErr:    true,
			errContains: "does not exist",
		},
		{
			name:       "get with nil context",
			collection: "test_collection",
			id:         "doc-1",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "context cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryStore()
			tt.setup(store)

			ctx := context.Background()
			if tt.errContains == "context cannot be nil" {
				ctx = nil
			}

			doc, err := store.Get(ctx, tt.collection, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if !docEqual(doc, tt.wantDoc) {
					t.Errorf("Expected document %v, got %v", tt.wantDoc, doc)
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name        string
		collection  string
		id          string
		doc         persistence.Document
		setup       func(store *InMemoryStore)
		wantErr     bool
		errContains string
	}{
		{
			name:       "update existing document",
			collection: "test_collection",
			id:         "doc-1",
			doc: persistence.Document{
				"id":    "doc-1",
				"name":  "Updated Document",
				"value": 100,
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":    "doc-1",
					"name":  "Original Document",
					"value": 42,
				})
			},
			wantErr: false,
		},
		{
			name:       "update missing document",
			collection: "test_collection",
			id:         "missing-doc",
			doc: persistence.Document{
				"id":   "missing-doc",
				"name": "New Document",
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:       "update in non-existent collection",
			collection: "missing_collection",
			id:         "doc-1",
			doc: persistence.Document{
				"id":   "doc-1",
				"name": "Test",
			},
			setup:       func(store *InMemoryStore) {},
			wantErr:     true,
			errContains: "does not exist",
		},
		{
			name:       "update with nil context",
			collection: "test_collection",
			id:         "doc-1",
			doc: persistence.Document{
				"id":   "doc-1",
				"name": "Updated",
			},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Original",
				})
			},
			wantErr:     true,
			errContains: "context cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryStore()
			tt.setup(store)

			ctx := context.Background()
			if tt.errContains == "context cannot be nil" {
				ctx = nil
			}

			err := store.Update(ctx, tt.collection, tt.id, tt.doc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				updatedDoc, err := store.Get(context.Background(), tt.collection, tt.id)
				if err != nil {
					t.Fatalf("Failed to get updated document: %v", err)
				}
				if !docEqual(updatedDoc, tt.doc) {
					t.Errorf("Expected updated document %v, got %v", tt.doc, updatedDoc)
				}
			}
		})
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name        string
		collection  string
		id          string
		setup       func(store *InMemoryStore)
		wantErr     bool
		errContains string
	}{
		{
			name:       "delete existing document",
			collection: "test_collection",
			id:         "doc-1",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Test Document",
				})
			},
			wantErr: false,
		},
		{
			name:       "delete missing document",
			collection: "test_collection",
			id:         "missing-doc",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:       "delete from non-existent collection",
			collection: "missing_collection",
			id:         "doc-1",
			setup:      func(store *InMemoryStore) {},
			wantErr:    true,
			errContains: "does not exist",
		},
		{
			name:       "delete with nil context",
			collection: "test_collection",
			id:         "doc-1",
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Test",
				})
			},
			wantErr:     true,
			errContains: "context cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryStore()
			tt.setup(store)

			ctx := context.Background()
			if tt.errContains == "context cannot be nil" {
				ctx = nil
			}

			err := store.Delete(ctx, tt.collection, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				_, err := store.Get(context.Background(), tt.collection, tt.id)
				if err == nil {
					t.Errorf("Expected document to be deleted")
				}
			}
		})
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		name        string
		collection  string
		query       persistence.Query
		setup       func(store *InMemoryStore)
		wantErr     bool
		errContains string
		wantCount   int
	}{
		{
			name:       "find all documents",
			collection: "test_collection",
			query:      persistence.Query{},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":   "doc-1",
					"name": "Document 1",
				})
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":   "doc-2",
					"name": "Document 2",
				})
				store.Insert(context.Background(), "test_collection", persistence.Document{
					"id":   "doc-3",
					"name": "Document 3",
				})
			},
			wantErr:   false,
			wantCount: 3,
		},
		{
			name:       "find in empty collection",
			collection: "test_collection",
			query:      persistence.Query{},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:        "find in non-existent collection",
			collection:  "missing_collection",
			query:       persistence.Query{},
			setup:       func(store *InMemoryStore) {},
			wantErr:     true,
			errContains: "does not exist",
		},
		{
			name:       "find with nil context",
			collection: "test_collection",
			query:      persistence.Query{},
			setup: func(store *InMemoryStore) {
				store.CreateCollection(context.Background(), "test_collection", nil)
			},
			wantErr:     true,
			errContains: "context cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryStore()
			tt.setup(store)

			ctx := context.Background()
			if tt.errContains == "context cannot be nil" {
				ctx = nil
			}

			docs, err := store.Find(ctx, tt.collection, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(docs) != tt.wantCount {
					t.Errorf("Expected %d documents, got %d", tt.wantCount, len(docs))
				}
			}
		})
	}
}

func TestDocumentIsolation(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)

	originalDoc := persistence.Document{
		"id":    "doc-1",
		"name":  "Original",
		"value": 42,
	}

	store.Insert(ctx, "test_collection", originalDoc)

	originalDoc["name"] = "Modified After Insert"
	originalDoc["value"] = 999

	retrievedDoc, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if retrievedDoc["name"] != "Original" {
		t.Errorf("Expected name 'Original', got %v (document not isolated)", retrievedDoc["name"])
	}
	if retrievedDoc["value"] != 42 {
		t.Errorf("Expected value 42, got %v (document not isolated)", retrievedDoc["value"])
	}

	retrievedDoc["name"] = "Modified After Get"
	retrievedDoc["value"] = 777

	retrievedDoc2, err := store.Get(ctx, "test_collection", "doc-1")
	if err != nil {
		t.Fatalf("Failed to get document second time: %v", err)
	}

	if retrievedDoc2["name"] != "Original" {
		t.Errorf("Expected name 'Original', got %v (returned document not isolated)", retrievedDoc2["name"])
	}
	if retrievedDoc2["value"] != 42 {
		t.Errorf("Expected value 42, got %v (returned document not isolated)", retrievedDoc2["value"])
	}
}

func TestMaintenance(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.Maintenance(ctx)
	if err != nil {
		t.Errorf("Expected Maintenance to succeed, got error: %v", err)
	}
}

func TestClose(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.CreateCollection(ctx, "test_collection", nil)
	store.Insert(ctx, "test_collection", persistence.Document{
		"id":   "doc-1",
		"name": "Test",
	})

	err := store.Close(ctx)
	if err != nil {
		t.Errorf("Expected Close to succeed, got error: %v", err)
	}

	if len(store.collections) != 0 {
		t.Errorf("Expected all collections to be cleared after Close, got %d", len(store.collections))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func docEqual(a, b persistence.Document) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
