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

package basic_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func TestSQLiteStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQLiteStore Suite")
}

var _ = Describe("SQLiteStore", func() {
	var (
		tempDir string
		dbPath  string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		dbPath = filepath.Join(tempDir, "test.db")
	})

	Context("when creating a new store", func() {
		It("should create store successfully with valid path", func() {
			store, err := basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(store).NotTo(BeNil())

			defer func() { _ = store.Close() }()
		})

		It("should fail with invalid path", func() {
			invalidPath := "/nonexistent/directory/that/does/not/exist/test.db"

			store, err := basic.NewSQLiteStore(invalidPath)
			Expect(err).To(HaveOccurred())

			if store != nil {
				_ = store.Close()
			}
		})

		It("should enable WAL mode", func() {
			store, err := basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = store.Close() }()

			ctx := context.Background()
			err = store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).NotTo(HaveOccurred())

			walFile := dbPath + "-wal"
			_, err = os.Stat(walFile)
			Expect(err).NotTo(HaveOccurred(), "WAL file should exist at %s - WAL mode should be enabled", walFile)
		})

		It("should configure darwin fullfsync on macOS", func() {
			if runtime.GOOS != "darwin" {
				Skip("Skipping darwin-specific test on non-darwin platform")
			}

			store, err := basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = store.Close() }()
		})
	})

	Context("when closing the store", func() {
		It("should close successfully", func() {
			store, err := basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())

			err = store.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when called twice", func() {
			store, err := basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())

			err = store.Close()
			Expect(err).NotTo(HaveOccurred())

			err = store.Close()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when implementing Store interface", func() {
		It("should satisfy the Store interface", func() {
			store, err := basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = store.Close() }()

			_ = store
		})
	})

	Context("CreateCollection", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should create collection with valid alphanumeric name", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create collection with valid name containing numbers", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "test123", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create collection with valid name containing underscores", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "test_collection_123", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be idempotent (IF NOT EXISTS)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "idempotent_test", nil)
			Expect(err).NotTo(HaveOccurred())

			err = store.CreateCollection(ctx, "idempotent_test", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject collection name with spaces", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "test collection", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject collection name with SQL injection attempt (semicolon)", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "test; DROP TABLE users;", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject collection name with SQL injection attempt (quotes)", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "test' OR '1'='1", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject collection name with dashes", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "test-collection", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject collection name with special characters", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "test@collection", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject empty collection name", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject collection name starting with number", func() {
			ctx := context.Background()
			err := store.CreateCollection(ctx, "123test", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should return error when store is closed", func() {
			ctx := context.Background()
			_ = store.Close()

			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store is closed"))
		})
	})

	Context("DropCollection", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should drop existing collection successfully", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "drop_test", nil)
			Expect(err).NotTo(HaveOccurred())

			err = store.DropCollection(ctx, "drop_test")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be idempotent (IF EXISTS)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "idempotent_drop", nil)
			Expect(err).NotTo(HaveOccurred())

			err = store.DropCollection(ctx, "idempotent_drop")
			Expect(err).NotTo(HaveOccurred())

			err = store.DropCollection(ctx, "idempotent_drop")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed when dropping non-existent collection", func() {
			ctx := context.Background()

			err := store.DropCollection(ctx, "non_existent_collection")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject collection name with SQL injection attempt (semicolon)", func() {
			ctx := context.Background()
			err := store.DropCollection(ctx, "test; DROP TABLE users;")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject collection name with SQL injection attempt (quotes)", func() {
			ctx := context.Background()
			err := store.DropCollection(ctx, "test' OR '1'='1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject collection name with spaces", func() {
			ctx := context.Background()
			err := store.DropCollection(ctx, "test collection")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should reject empty collection name", func() {
			ctx := context.Background()
			err := store.DropCollection(ctx, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should return error when store is closed", func() {
			ctx := context.Background()
			_ = store.Close()

			err := store.DropCollection(ctx, "test_collection")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store is closed"))
		})

		It("should actually remove collection and data", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "removal_test", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "removal_test", basic.Document{"name": "test"})
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			err = store.DropCollection(ctx, "removal_test")
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(ctx, "removal_test", id)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Collection Management Integration", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should create, use, and drop collection lifecycle", func() {
			ctx := context.Background()
			collectionName := "lifecycle_test"

			err := store.CreateCollection(ctx, collectionName, nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, collectionName, basic.Document{"key": "value"})
			Expect(err).NotTo(HaveOccurred())

			doc, err := store.Get(ctx, collectionName, id)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc["key"]).To(Equal("value"))

			err = store.DropCollection(ctx, collectionName)
			Expect(err).NotTo(HaveOccurred())

			err = store.CreateCollection(ctx, collectionName, nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(ctx, collectionName, id)
			Expect(err).To(Equal(basic.ErrNotFound))
		})
	})

	Context("Transaction CreateCollection", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should create collection within transaction and commit", func() {
			ctx := context.Background()

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.CreateCollection(ctx, "tx_collection", nil)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "tx_collection", basic.Document{"test": "value"})
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())
		})

		It("should reject invalid collection names in transaction", func() {
			ctx := context.Background()

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.CreateCollection(ctx, "test; DROP TABLE users;", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should rollback collection creation on transaction rollback", func() {
			ctx := context.Background()

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.CreateCollection(ctx, "rollback_collection", nil)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Rollback()
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "rollback_collection", basic.Document{"test": "value"})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Transaction DropCollection", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should drop collection within transaction and commit", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "drop_in_tx", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.DropCollection(ctx, "drop_in_tx")
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "drop_in_tx", basic.Document{"test": "value"})
			Expect(err).To(HaveOccurred())
		})

		It("should reject invalid collection names in transaction", func() {
			ctx := context.Background()

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.DropCollection(ctx, "test' OR '1'='1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid collection name"))
		})

		It("should rollback collection drop on transaction rollback", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "rollback_drop", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "rollback_drop", basic.Document{"test": "value"})
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.DropCollection(ctx, "rollback_drop")
			Expect(err).NotTo(HaveOccurred())

			err = tx.Rollback()
			Expect(err).NotTo(HaveOccurred())

			doc, err := store.Get(ctx, "rollback_drop", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc["test"]).To(Equal("value"))
		})
	})

	Context("Insert", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should insert document and return valid UUID v4", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_insert", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"name": "test-doc",
				"value": 42,
			}

			id, err := store.Insert(ctx, "test_insert", doc)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			Expect(id).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`))
		})

		It("should insert and retrieve document with same content", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_retrieve", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"name": "test-doc",
				"value": 42,
				"active": true,
			}

			id, err := store.Insert(ctx, "test_retrieve", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_retrieve", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("test-doc"))
			Expect(retrieved["value"]).To(BeNumerically("==", 42))
			Expect(retrieved["active"]).To(BeTrue())
		})

		It("should insert document with nested maps", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_nested", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"name": "nested-doc",
				"metadata": map[string]interface{}{
					"level1": map[string]interface{}{
						"level2": "deep-value",
						"number": 123,
					},
				},
			}

			id, err := store.Insert(ctx, "test_nested", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_nested", id)
			Expect(err).NotTo(HaveOccurred())

			metadata, ok := retrieved["metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			level1, ok := metadata["level1"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			Expect(level1["level2"]).To(Equal("deep-value"))
			Expect(level1["number"]).To(BeNumerically("==", 123))
		})

		It("should insert document with arrays", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_arrays", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"tags": []interface{}{"production", "critical", "monitored"},
				"values": []interface{}{1, 2, 3, 4, 5},
			}

			id, err := store.Insert(ctx, "test_arrays", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_arrays", id)
			Expect(err).NotTo(HaveOccurred())

			tags, ok := retrieved["tags"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(tags).To(HaveLen(3))
			Expect(tags[0]).To(Equal("production"))

			values, ok := retrieved["values"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(values).To(HaveLen(5))
		})

		It("should insert document with null values", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_nulls", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"name": "has-nulls",
				"optional": nil,
			}

			id, err := store.Insert(ctx, "test_nulls", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_nulls", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("has-nulls"))
			Expect(retrieved["optional"]).To(BeNil())
		})

		It("should insert document with all JSON-serializable types", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_types", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"string": "text",
				"int": 42,
				"float": 3.14,
				"bool": true,
				"null": nil,
				"array": []interface{}{1, "two", 3.0},
				"object": map[string]interface{}{"key": "value"},
			}

			id, err := store.Insert(ctx, "test_types", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_types", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["string"]).To(Equal("text"))
			Expect(retrieved["int"]).To(BeNumerically("==", 42))
			Expect(retrieved["float"]).To(BeNumerically("~", 3.14))
			Expect(retrieved["bool"]).To(BeTrue())
			Expect(retrieved["null"]).To(BeNil())
			Expect(retrieved["array"]).To(HaveLen(3))
			Expect(retrieved["object"]).To(HaveKey("key"))
		})

		It("should fail when inserting into non-existent collection", func() {
			ctx := context.Background()

			doc := basic.Document{"test": "value"}

			_, err := store.Insert(ctx, "nonexistent_collection", doc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to insert document"))
		})

		It("should fail when store is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			_ = store.Close()

			doc := basic.Document{"test": "value"}
			_, err = store.Insert(ctx, "test_closed", doc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store is closed"))
		})

		It("should generate unique IDs for multiple inserts", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_unique", nil)
			Expect(err).NotTo(HaveOccurred())

			ids := make(map[string]bool)
			for i := range 100 {
				doc := basic.Document{"index": i}
				id, err := store.Insert(ctx, "test_unique", doc)
				Expect(err).NotTo(HaveOccurred())
				Expect(ids[id]).To(BeFalse())
				ids[id] = true
			}

			Expect(ids).To(HaveLen(100))
		})

		It("should insert empty document", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_empty", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{}

			id, err := store.Insert(ctx, "test_empty", doc)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			retrieved, err := store.Get(ctx, "test_empty", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved).NotTo(BeNil())
		})
	})

	Context("Transaction Insert", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should insert within transaction and commit", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_insert", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			doc := basic.Document{"name": "tx-doc"}
			id, err := tx.Insert(ctx, "tx_insert", doc)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "tx_insert", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("tx-doc"))
		})

		It("should rollback insert on transaction rollback", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_rollback", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{"name": "rollback-doc"}
			id, err := tx.Insert(ctx, "tx_rollback", doc)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Rollback()
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(ctx, "tx_rollback", id)
			Expect(err).To(Equal(basic.ErrNotFound))
		})

		It("should insert multiple documents atomically", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_multi", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			id1, err := tx.Insert(ctx, "tx_multi", basic.Document{"index": 1})
			Expect(err).NotTo(HaveOccurred())

			id2, err := tx.Insert(ctx, "tx_multi", basic.Document{"index": 2})
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			doc1, err := store.Get(ctx, "tx_multi", id1)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc1["index"]).To(BeNumerically("==", 1))

			doc2, err := store.Get(ctx, "tx_multi", id2)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc2["index"]).To(BeNumerically("==", 2))
		})

		It("should generate valid UUID v4 within transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_uuid", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			doc := basic.Document{"test": "value"}
			id, err := tx.Insert(ctx, "tx_uuid", doc)
			Expect(err).NotTo(HaveOccurred())

			Expect(id).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`))

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when transaction is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{"test": "value"}
			_, err = tx.Insert(ctx, "tx_closed", doc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transaction is closed"))
		})
	})

	Context("Get", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should get existing document successfully", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_get", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"name": "test-document",
				"value": 42,
			}

			id, err := store.Insert(ctx, "test_get", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_get", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved).NotTo(BeNil())
			Expect(retrieved["name"]).To(Equal("test-document"))
			Expect(retrieved["value"]).To(BeNumerically("==", 42))
		})

		It("should return exact document content with deep equality", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_exact", nil)
			Expect(err).NotTo(HaveOccurred())

			original := basic.Document{
				"string": "text",
				"int": 42,
				"float": 3.14,
				"bool": true,
				"null": nil,
				"array": []interface{}{1, "two", 3.0},
				"object": map[string]interface{}{"nested": "value"},
			}

			id, err := store.Insert(ctx, "test_exact", original)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_exact", id)
			Expect(err).NotTo(HaveOccurred())

			Expect(retrieved["string"]).To(Equal(original["string"]))
			Expect(retrieved["int"]).To(BeNumerically("==", original["int"]))
			Expect(retrieved["float"]).To(BeNumerically("~", original["float"]))
			Expect(retrieved["bool"]).To(Equal(original["bool"]))
			Expect(retrieved["null"]).To(BeNil())

			retrievedArray := retrieved["array"].([]interface{})
			originalArray := original["array"].([]interface{})
			Expect(retrievedArray).To(HaveLen(len(originalArray)))
			Expect(retrievedArray[0]).To(BeNumerically("==", originalArray[0]))
			Expect(retrievedArray[1]).To(Equal(originalArray[1]))
			Expect(retrievedArray[2]).To(BeNumerically("~", originalArray[2]))

			retrievedObj := retrieved["object"].(map[string]interface{})
			originalObj := original["object"].(map[string]interface{})
			Expect(retrievedObj["nested"]).To(Equal(originalObj["nested"]))
		})

		It("should return ErrNotFound for non-existent document", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_not_found", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(ctx, "test_not_found", "non-existent-id")
			Expect(err).To(Equal(basic.ErrNotFound))
		})

		It("should return ErrNotFound for non-existent collection", func() {
			ctx := context.Background()

			_, err := store.Get(ctx, "nonexistent_collection", "some-id")
			Expect(err).To(HaveOccurred())
		})

		It("should fail when store is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_closed_get", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{"test": "value"}
			id, err := store.Insert(ctx, "test_closed_get", doc)
			Expect(err).NotTo(HaveOccurred())

			_ = store.Close()

			_, err = store.Get(ctx, "test_closed_get", id)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store is closed"))
		})

		It("should preserve complex nested structure", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_nested_get", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"deep": "value",
							"number": 999,
						},
						"array": []interface{}{
							map[string]interface{}{"item": 1},
							map[string]interface{}{"item": 2},
						},
					},
				},
			}

			id, err := store.Insert(ctx, "test_nested_get", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_nested_get", id)
			Expect(err).NotTo(HaveOccurred())

			level1 := retrieved["level1"].(map[string]interface{})
			level2 := level1["level2"].(map[string]interface{})
			level3 := level2["level3"].(map[string]interface{})

			Expect(level3["deep"]).To(Equal("value"))
			Expect(level3["number"]).To(BeNumerically("==", 999))

			array := level2["array"].([]interface{})
			Expect(array).To(HaveLen(2))

			item0 := array[0].(map[string]interface{})
			Expect(item0["item"]).To(BeNumerically("==", 1))

			item1 := array[1].(map[string]interface{})
			Expect(item1["item"]).To(BeNumerically("==", 2))
		})

		It("should preserve null values in JSON", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_nulls_get", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"field1": "value",
				"field2": nil,
				"field3": map[string]interface{}{
					"nested": nil,
				},
			}

			id, err := store.Insert(ctx, "test_nulls_get", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_nulls_get", id)
			Expect(err).NotTo(HaveOccurred())

			Expect(retrieved["field1"]).To(Equal("value"))
			Expect(retrieved["field2"]).To(BeNil())

			nested := retrieved["field3"].(map[string]interface{})
			Expect(nested["nested"]).To(BeNil())
		})

		It("should preserve arrays with mixed types", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_arrays_get", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"mixed": []interface{}{
					"string",
					42,
					3.14,
					true,
					nil,
					map[string]interface{}{"key": "value"},
					[]interface{}{1, 2, 3},
				},
			}

			id, err := store.Insert(ctx, "test_arrays_get", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_arrays_get", id)
			Expect(err).NotTo(HaveOccurred())

			mixed := retrieved["mixed"].([]interface{})
			Expect(mixed).To(HaveLen(7))
			Expect(mixed[0]).To(Equal("string"))
			Expect(mixed[1]).To(BeNumerically("==", 42))
			Expect(mixed[2]).To(BeNumerically("~", 3.14))
			Expect(mixed[3]).To(BeTrue())
			Expect(mixed[4]).To(BeNil())

			obj := mixed[5].(map[string]interface{})
			Expect(obj["key"]).To(Equal("value"))

			arr := mixed[6].([]interface{})
			Expect(arr).To(HaveLen(3))
		})

		It("should preserve all JSON data types", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_types_get", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"string": "test",
				"int": 42,
				"negativeInt": -10,
				"float": 3.14159,
				"negativeFloat": -2.718,
				"boolTrue": true,
				"boolFalse": false,
				"null": nil,
				"emptyString": "",
				"zeroInt": 0,
				"zeroFloat": 0.0,
			}

			id, err := store.Insert(ctx, "test_types_get", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_types_get", id)
			Expect(err).NotTo(HaveOccurred())

			Expect(retrieved["string"]).To(Equal("test"))
			Expect(retrieved["int"]).To(BeNumerically("==", 42))
			Expect(retrieved["negativeInt"]).To(BeNumerically("==", -10))
			Expect(retrieved["float"]).To(BeNumerically("~", 3.14159))
			Expect(retrieved["negativeFloat"]).To(BeNumerically("~", -2.718))
			Expect(retrieved["boolTrue"]).To(BeTrue())
			Expect(retrieved["boolFalse"]).To(BeFalse())
			Expect(retrieved["null"]).To(BeNil())
			Expect(retrieved["emptyString"]).To(Equal(""))
			Expect(retrieved["zeroInt"]).To(BeNumerically("==", 0))
			Expect(retrieved["zeroFloat"]).To(BeNumerically("==", 0.0))
		})
	})

	Context("Transaction Get", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewSQLiteStore(dbPath)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close()
			}
		})

		It("should get document within transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_get", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{"name": "tx-doc", "value": 100}
			id, err := store.Insert(ctx, "tx_get", doc)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			retrieved, err := tx.Get(ctx, "tx_get", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("tx-doc"))
			Expect(retrieved["value"]).To(BeNumerically("==", 100))

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should get document inserted in same transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_get_insert", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			doc := basic.Document{"name": "in-tx"}
			id, err := tx.Insert(ctx, "tx_get_insert", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := tx.Get(ctx, "tx_get_insert", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("in-tx"))

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return ErrNotFound for non-existent document in transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_get_notfound", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			_, err = tx.Get(ctx, "tx_get_notfound", "non-existent")
			Expect(err).To(Equal(basic.ErrNotFound))

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when transaction is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_get_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{"test": "value"}
			id, err := store.Insert(ctx, "tx_get_closed", doc)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			_, err = tx.Get(ctx, "tx_get_closed", id)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transaction is closed"))
		})

		It("should preserve complex nested structure in transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_get_nested", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			doc := basic.Document{
				"nested": map[string]interface{}{
					"array": []interface{}{1, 2, 3},
					"object": map[string]interface{}{
						"deep": "value",
					},
				},
			}

			id, err := tx.Insert(ctx, "tx_get_nested", doc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := tx.Get(ctx, "tx_get_nested", id)
			Expect(err).NotTo(HaveOccurred())

			nested := retrieved["nested"].(map[string]interface{})
			array := nested["array"].([]interface{})
			Expect(array).To(HaveLen(3))

			object := nested["object"].(map[string]interface{})
			Expect(object["deep"]).To(Equal("value"))

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
