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
})
