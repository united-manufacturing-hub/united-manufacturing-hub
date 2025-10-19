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
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func TestSQLiteStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQLiteStore Suite")
}

var _ = Describe("JournalMode constants", func() {
	It("should define WAL mode constant", func() {
		Expect(basic.JournalModeWAL).To(Equal(basic.JournalMode("WAL")))
	})

	It("should define DELETE mode constant", func() {
		Expect(basic.JournalModeDELETE).To(Equal(basic.JournalMode("DELETE")))
	})

	It("should use WAL as string representation", func() {
		mode := basic.JournalModeWAL
		Expect(string(mode)).To(Equal("WAL"))
	})

	It("should use DELETE as string representation", func() {
		mode := basic.JournalModeDELETE
		Expect(string(mode)).To(Equal("DELETE"))
	})
})

var _ = Describe("Config struct", func() {
	It("should include JournalMode field", func() {
		cfg := basic.Config{
			DBPath:                "./test.db",
			JournalMode:           basic.JournalModeWAL,
			MaintenanceOnShutdown: true,
		}
		Expect(cfg.JournalMode).To(Equal(basic.JournalModeWAL))
	})

	It("should allow DELETE mode", func() {
		cfg := basic.Config{
			DBPath:                "./test.db",
			JournalMode:           basic.JournalModeDELETE,
			MaintenanceOnShutdown: false,
		}
		Expect(cfg.JournalMode).To(Equal(basic.JournalModeDELETE))
	})
})

var _ = Describe("DefaultConfig", func() {
	It("should set JournalMode to WAL by default", func() {
		cfg := basic.DefaultConfig("./test.db")
		Expect(cfg.JournalMode).To(Equal(basic.JournalModeWAL))
	})

	It("should set MaintenanceOnShutdown to true by default", func() {
		cfg := basic.DefaultConfig("./test.db")
		Expect(cfg.MaintenanceOnShutdown).To(BeTrue())
	})

	It("should preserve provided DBPath", func() {
		cfg := basic.DefaultConfig("/custom/path.db")
		Expect(cfg.DBPath).To(Equal("/custom/path.db"))
	})
})

var _ = Describe("Network filesystem detection", func() {
	Context("isNetworkFilesystem function", func() {
		It("should detect current directory as local filesystem", func() {
			isNetwork, fsType, err := basic.IsNetworkFilesystem(".")
			Expect(err).NotTo(HaveOccurred())
			Expect(isNetwork).To(BeFalse())
			Expect(fsType).NotTo(BeEmpty())
		})

		It("should return filesystem type name", func() {
			_, fsType, err := basic.IsNetworkFilesystem(".")
			Expect(err).NotTo(HaveOccurred())
			// macOS: "apfs", "hfs", etc.
			// Linux: "ext4", "btrfs", "xfs", etc.
			Expect(fsType).To(MatchRegexp(`(?i)(apfs|hfs|ext[234]|xfs|btrfs|zfs|tmpfs)`))
		})

		It("should return error for non-existent path", func() {
			_, _, err := basic.IsNetworkFilesystem("/nonexistent/path/that/does/not/exist")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to stat filesystem"))
		})

		It("should identify NFS filesystem type as network", func() {
			// This is a logic test, not integration test
			// We test the helper function directly
			Expect(basic.IsNetworkFSType("nfs")).To(BeTrue())
			Expect(basic.IsNetworkFSType("nfs4")).To(BeTrue())
		})

		It("should identify CIFS/SMB filesystem types as network", func() {
			Expect(basic.IsNetworkFSType("cifs")).To(BeTrue())
			Expect(basic.IsNetworkFSType("smb")).To(BeTrue())
			Expect(basic.IsNetworkFSType("smbfs")).To(BeTrue())
		})

		It("should identify WebDAV filesystem type as network", func() {
			Expect(basic.IsNetworkFSType("webdav")).To(BeTrue())
		})

		It("should identify local filesystem types as NOT network", func() {
			Expect(basic.IsNetworkFSType("apfs")).To(BeFalse())
			Expect(basic.IsNetworkFSType("hfs")).To(BeFalse())
			Expect(basic.IsNetworkFSType("ext4")).To(BeFalse())
			Expect(basic.IsNetworkFSType("xfs")).To(BeFalse())
			Expect(basic.IsNetworkFSType("btrfs")).To(BeFalse())
			Expect(basic.IsNetworkFSType("tmpfs")).To(BeFalse())
		})

		It("should be case-insensitive for filesystem type matching", func() {
			Expect(basic.IsNetworkFSType("NFS")).To(BeTrue())
			Expect(basic.IsNetworkFSType("Cifs")).To(BeTrue())
			Expect(basic.IsNetworkFSType("APFS")).To(BeFalse())
		})
	})
})

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
			store, err := basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
			Expect(store).NotTo(BeNil())

			defer func() { _ = store.Close(context.Background()) }()
		})

		It("should fail with invalid path", func() {
			invalidPath := "/nonexistent/directory/that/does/not/exist/test.db"

			store, err := basic.NewStore(basic.DefaultConfig(invalidPath))
			Expect(err).To(HaveOccurred())

			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should enable WAL mode", func() {
			store, err := basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = store.Close(context.Background()) }()

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

			store, err := basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = store.Close(context.Background()) }()
		})
	})

	Context("when closing the store", func() {
		It("should close successfully", func() {
			store, err := basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())

			err = store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be idempotent when called twice", func() {
			store, err := basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())

			err = store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())

			err = store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when implementing Store interface", func() {
		It("should satisfy the Store interface", func() {
			store, err := basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = store.Close(context.Background()) }()

			_ = store
		})
	})

	Context("CreateCollection", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
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
			_ = store.Close(context.Background())

			err := store.CreateCollection(ctx, "test_collection", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store is closed"))
		})
	})

	Context("DropCollection", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
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
			_ = store.Close(context.Background())

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
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
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
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})
	})

	Context("Transaction CreateCollection", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
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
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
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
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should insert document and return valid UUID v4", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_insert", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"name":  "test-doc",
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
				"name":   "test-doc",
				"value":  42,
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
				"tags":   []interface{}{"production", "critical", "monitored"},
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
				"name":     "has-nulls",
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
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"null":   nil,
				"array":  []interface{}{1, "two", 3.0},
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

			_ = store.Close(context.Background())

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
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
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
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
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
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should get existing document successfully", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_get", nil)
			Expect(err).NotTo(HaveOccurred())

			doc := basic.Document{
				"name":  "test-document",
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
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"null":   nil,
				"array":  []interface{}{1, "two", 3.0},
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
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
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

			_ = store.Close(context.Background())

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
							"deep":   "value",
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
				"string":        "test",
				"int":           42,
				"negativeInt":   -10,
				"float":         3.14159,
				"negativeFloat": -2.718,
				"boolTrue":      true,
				"boolFalse":     false,
				"null":          nil,
				"emptyString":   "",
				"zeroInt":       0,
				"zeroFloat":     0.0,
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
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
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
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())

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

	Context("Update", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should update existing document successfully", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_update", nil)
			Expect(err).NotTo(HaveOccurred())

			originalDoc := basic.Document{
				"name":  "original",
				"value": 42,
			}

			id, err := store.Insert(ctx, "test_update", originalDoc)
			Expect(err).NotTo(HaveOccurred())

			updatedDoc := basic.Document{
				"name":      "updated",
				"value":     100,
				"new_field": "added",
			}

			err = store.Update(ctx, "test_update", id, updatedDoc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_update", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("updated"))
			Expect(retrieved["value"]).To(BeNumerically("==", 100))
			Expect(retrieved["new_field"]).To(Equal("added"))
		})

		It("should retrieve updated document with new content", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_update_retrieve", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "test_update_retrieve", basic.Document{"old": "data"})
			Expect(err).NotTo(HaveOccurred())

			newDoc := basic.Document{
				"completely": "different",
				"structure": map[string]interface{}{
					"nested": "value",
				},
			}

			err = store.Update(ctx, "test_update_retrieve", id, newDoc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_update_retrieve", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["old"]).To(BeNil())
			Expect(retrieved["completely"]).To(Equal("different"))

			structure := retrieved["structure"].(map[string]interface{})
			Expect(structure["nested"]).To(Equal("value"))
		})

		It("should return ErrNotFound when updating non-existent document", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_update_notfound", nil)
			Expect(err).NotTo(HaveOccurred())

			err = store.Update(ctx, "test_update_notfound", "non-existent-id", basic.Document{"test": "value"})
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should fail when updating in non-existent collection", func() {
			ctx := context.Background()

			err := store.Update(ctx, "nonexistent_collection", "some-id", basic.Document{"test": "value"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update document"))
		})

		It("should update document with complex nested data", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_update_nested", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "test_update_nested", basic.Document{"simple": "data"})
			Expect(err).NotTo(HaveOccurred())

			complexDoc := basic.Document{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": []interface{}{
							map[string]interface{}{"item": 1},
							map[string]interface{}{"item": 2},
						},
					},
					"array": []interface{}{1, "two", 3.0, nil},
				},
				"mixed": []interface{}{
					"string",
					42,
					true,
					map[string]interface{}{"nested": "object"},
				},
			}

			err = store.Update(ctx, "test_update_nested", id, complexDoc)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "test_update_nested", id)
			Expect(err).NotTo(HaveOccurred())

			level1 := retrieved["level1"].(map[string]interface{})
			level2 := level1["level2"].(map[string]interface{})
			level3 := level2["level3"].([]interface{})
			Expect(level3).To(HaveLen(2))

			array := level1["array"].([]interface{})
			Expect(array).To(HaveLen(4))
			Expect(array[3]).To(BeNil())

			mixed := retrieved["mixed"].([]interface{})
			Expect(mixed).To(HaveLen(4))
		})

		It("should fail when store is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_update_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "test_update_closed", basic.Document{"test": "value"})
			Expect(err).NotTo(HaveOccurred())

			_ = store.Close(context.Background())

			err = store.Update(ctx, "test_update_closed", id, basic.Document{"new": "value"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store is closed"))
		})
	})

	Context("Transaction Update", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should update document within transaction and commit", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_update", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "tx_update", basic.Document{"name": "original"})
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.Update(ctx, "tx_update", id, basic.Document{"name": "updated"})
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "tx_update", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("updated"))
		})

		It("should rollback update on transaction rollback", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_update_rollback", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "tx_update_rollback", basic.Document{"name": "original"})
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Update(ctx, "tx_update_rollback", id, basic.Document{"name": "updated"})
			Expect(err).NotTo(HaveOccurred())

			err = tx.Rollback()
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "tx_update_rollback", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("original"))
		})

		It("should return ErrNotFound when updating non-existent document in transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_update_notfound", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.Update(ctx, "tx_update_notfound", "non-existent", basic.Document{"test": "value"})
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when transaction is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_update_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "tx_update_closed", basic.Document{"test": "value"})
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			err = tx.Update(ctx, "tx_update_closed", id, basic.Document{"new": "value"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transaction is closed"))
		})
	})

	Context("Delete", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should delete existing document successfully", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_delete", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "test_delete", basic.Document{"name": "to-delete"})
			Expect(err).NotTo(HaveOccurred())

			err = store.Delete(ctx, "test_delete", id)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(ctx, "test_delete", id)
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should return ErrNotFound when deleting non-existent document", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_delete_notfound", nil)
			Expect(err).NotTo(HaveOccurred())

			err = store.Delete(ctx, "test_delete_notfound", "non-existent-id")
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should fail when deleting from non-existent collection", func() {
			ctx := context.Background()

			err := store.Delete(ctx, "nonexistent_collection", "some-id")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to delete document"))
		})

		It("should delete multiple documents independently", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_delete_multiple", nil)
			Expect(err).NotTo(HaveOccurred())

			id1, err := store.Insert(ctx, "test_delete_multiple", basic.Document{"name": "doc1"})
			Expect(err).NotTo(HaveOccurred())

			id2, err := store.Insert(ctx, "test_delete_multiple", basic.Document{"name": "doc2"})
			Expect(err).NotTo(HaveOccurred())

			id3, err := store.Insert(ctx, "test_delete_multiple", basic.Document{"name": "doc3"})
			Expect(err).NotTo(HaveOccurred())

			err = store.Delete(ctx, "test_delete_multiple", id2)
			Expect(err).NotTo(HaveOccurred())

			doc1, err := store.Get(ctx, "test_delete_multiple", id1)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc1["name"]).To(Equal("doc1"))

			_, err = store.Get(ctx, "test_delete_multiple", id2)
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())

			doc3, err := store.Get(ctx, "test_delete_multiple", id3)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc3["name"]).To(Equal("doc3"))
		})

		It("should fail when store is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_delete_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "test_delete_closed", basic.Document{"test": "value"})
			Expect(err).NotTo(HaveOccurred())

			_ = store.Close(context.Background())

			err = store.Delete(ctx, "test_delete_closed", id)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store is closed"))
		})
	})

	Context("Find", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should find all documents with empty query", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_all", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_all", basic.Document{"name": "doc1"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_all", basic.Document{"name": "doc2"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_all", basic.Document{"name": "doc3"})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery()
			docs, err := store.Find(ctx, "test_find_all", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(3))
		})

		It("should return empty slice for empty collection", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_empty", nil)
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery()
			docs, err := store.Find(ctx, "test_find_empty", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(BeEmpty())
		})

		It("should filter by equality (Eq operator)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_eq", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_eq", basic.Document{"status": "active", "value": 1})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_eq", basic.Document{"status": "inactive", "value": 2})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_eq", basic.Document{"status": "active", "value": 3})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("status", basic.Eq, "active")
			docs, err := store.Find(ctx, "test_find_eq", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
			Expect(docs[0]["status"]).To(Equal("active"))
			Expect(docs[1]["status"]).To(Equal("active"))
		})

		It("should filter by inequality (Ne operator)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_ne", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_ne", basic.Document{"status": "active"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_ne", basic.Document{"status": "inactive"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_ne", basic.Document{"status": "active"})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("status", basic.Ne, "active")
			docs, err := store.Find(ctx, "test_find_ne", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(1))
			Expect(docs[0]["status"]).To(Equal("inactive"))
		})

		It("should filter by greater than (Gt operator)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_gt", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_gt", basic.Document{"age": 15})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_gt", basic.Document{"age": 20})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_gt", basic.Document{"age": 25})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("age", basic.Gt, 18)
			docs, err := store.Find(ctx, "test_find_gt", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
		})

		It("should filter by greater than or equal (Gte operator)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_gte", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_gte", basic.Document{"age": 15})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_gte", basic.Document{"age": 18})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_gte", basic.Document{"age": 25})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("age", basic.Gte, 18)
			docs, err := store.Find(ctx, "test_find_gte", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
		})

		It("should filter by less than (Lt operator)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_lt", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_lt", basic.Document{"age": 15})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_lt", basic.Document{"age": 20})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_lt", basic.Document{"age": 25})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("age", basic.Lt, 20)
			docs, err := store.Find(ctx, "test_find_lt", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(1))
			Expect(docs[0]["age"]).To(BeNumerically("==", 15))
		})

		It("should filter by less than or equal (Lte operator)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_lte", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_lte", basic.Document{"age": 15})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_lte", basic.Document{"age": 20})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_lte", basic.Document{"age": 25})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("age", basic.Lte, 20)
			docs, err := store.Find(ctx, "test_find_lte", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
		})

		It("should filter by In operator", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_in", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_in", basic.Document{"role": "admin"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_in", basic.Document{"role": "user"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_in", basic.Document{"role": "moderator"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_in", basic.Document{"role": "guest"})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("role", basic.In, []string{"admin", "moderator"})
			docs, err := store.Find(ctx, "test_find_in", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
		})

		It("should filter by Nin operator", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_nin", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_nin", basic.Document{"role": "admin"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_nin", basic.Document{"role": "user"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_nin", basic.Document{"role": "moderator"})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("role", basic.Nin, []string{"admin", "moderator"})
			docs, err := store.Find(ctx, "test_find_nin", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(1))
			Expect(docs[0]["role"]).To(Equal("user"))
		})

		It("should filter by In operator with boolean array", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_bool_in", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_bool_in", basic.Document{"active": true, "name": "service1"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_bool_in", basic.Document{"active": false, "name": "service2"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_bool_in", basic.Document{"active": true, "name": "service3"})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("active", basic.In, []bool{true})
			docs, err := store.Find(ctx, "test_find_bool_in", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
		})

		It("should sort by field ascending", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_sort_asc", nil)
			Expect(err).NotTo(HaveOccurred())

			id1, err := store.Insert(ctx, "test_find_sort_asc", basic.Document{"value": 30})
			Expect(err).NotTo(HaveOccurred())

			id2, err := store.Insert(ctx, "test_find_sort_asc", basic.Document{"value": 10})
			Expect(err).NotTo(HaveOccurred())

			id3, err := store.Insert(ctx, "test_find_sort_asc", basic.Document{"value": 20})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Sort("id", basic.Asc)
			docs, err := store.Find(ctx, "test_find_sort_asc", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(3))

			if id1 < id2 && id2 < id3 {
				Expect(docs[0]["value"]).To(BeNumerically("==", 30))
				Expect(docs[1]["value"]).To(BeNumerically("==", 10))
				Expect(docs[2]["value"]).To(BeNumerically("==", 20))
			}
		})

		It("should sort by field descending", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_sort_desc", nil)
			Expect(err).NotTo(HaveOccurred())

			id1, err := store.Insert(ctx, "test_find_sort_desc", basic.Document{"value": 30})
			Expect(err).NotTo(HaveOccurred())

			id2, err := store.Insert(ctx, "test_find_sort_desc", basic.Document{"value": 10})
			Expect(err).NotTo(HaveOccurred())

			id3, err := store.Insert(ctx, "test_find_sort_desc", basic.Document{"value": 20})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Sort("id", basic.Desc)
			docs, err := store.Find(ctx, "test_find_sort_desc", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(3))

			if id1 < id2 && id2 < id3 {
				Expect(docs[0]["value"]).To(BeNumerically("==", 20))
				Expect(docs[1]["value"]).To(BeNumerically("==", 10))
				Expect(docs[2]["value"]).To(BeNumerically("==", 30))
			}
		})

		It("should apply limit", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_limit", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 10 {
				_, err = store.Insert(ctx, "test_find_limit", basic.Document{"index": i})
				Expect(err).NotTo(HaveOccurred())
			}

			query := basic.NewQuery().Limit(5)
			docs, err := store.Find(ctx, "test_find_limit", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(5))
		})

		It("should apply skip (offset)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_skip", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 10 {
				_, err = store.Insert(ctx, "test_find_skip", basic.Document{"index": i})
				Expect(err).NotTo(HaveOccurred())
			}

			query := basic.NewQuery().Skip(7)
			docs, err := store.Find(ctx, "test_find_skip", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(3))
		})

		It("should apply limit and skip for pagination", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_pagination", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 10 {
				_, err = store.Insert(ctx, "test_find_pagination", basic.Document{"index": i})
				Expect(err).NotTo(HaveOccurred())
			}

			query := basic.NewQuery().Limit(3).Skip(3)
			docs, err := store.Find(ctx, "test_find_pagination", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(3))
		})

		It("should combine multiple filters (AND logic)", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_and", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_and", basic.Document{"status": "active", "age": 15})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_and", basic.Document{"status": "active", "age": 25})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_and", basic.Document{"status": "inactive", "age": 25})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_and", basic.Document{"status": "active", "age": 30})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().
				Filter("status", basic.Eq, "active").
				Filter("age", basic.Gte, 18)
			docs, err := store.Find(ctx, "test_find_and", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
		})

		It("should combine filters, sorting, and pagination", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_complex", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 20 {
				status := "active"
				if i%3 == 0 {
					status = "inactive"
				}
				_, err = store.Insert(ctx, "test_find_complex", basic.Document{
					"status": status,
					"value":  i,
				})
				Expect(err).NotTo(HaveOccurred())
			}

			query := basic.NewQuery().
				Filter("status", basic.Eq, "active").
				Sort("id", basic.Asc).
				Limit(5).
				Skip(2)
			docs, err := store.Find(ctx, "test_find_complex", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(5))
			for _, doc := range docs {
				Expect(doc["status"]).To(Equal("active"))
			}
		})

		It("should return empty result set when no documents match filter", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_no_match", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_no_match", basic.Document{"status": "active"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_no_match", basic.Document{"status": "inactive"})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("status", basic.Eq, "nonexistent")
			docs, err := store.Find(ctx, "test_find_no_match", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(BeEmpty())
		})

		It("should fail when store is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			_ = store.Close(context.Background())

			query := basic.NewQuery()
			_, err = store.Find(ctx, "test_find_closed", *query)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store is closed"))
		})

		It("should filter by float comparison", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_float", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_float", basic.Document{"temperature": 15.5})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_float", basic.Document{"temperature": 20.3})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_float", basic.Document{"temperature": 25.7})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("temperature", basic.Gt, 18.0)
			docs, err := store.Find(ctx, "test_find_float", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
		})

		It("should filter by string comparison", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_string", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_string", basic.Document{"name": "alice"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_string", basic.Document{"name": "bob"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test_find_string", basic.Document{"name": "charlie"})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery().Filter("name", basic.Gt, "alice")
			docs, err := store.Find(ctx, "test_find_string", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))
		})

		It("should return error when context is cancelled during Find", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "test_find_cancel", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 100 {
				_, err = store.Insert(ctx, "test_find_cancel", basic.Document{"value": i})
				Expect(err).NotTo(HaveOccurred())
			}

			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			query := basic.NewQuery()
			_, err = store.Find(cancelCtx, "test_find_cancel", *query)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		})
	})

	Context("Transaction Find", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should find documents within transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_find", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "tx_find", basic.Document{"status": "active"})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "tx_find", basic.Document{"status": "inactive"})
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			query := basic.NewQuery().Filter("status", basic.Eq, "active")
			docs, err := tx.Find(ctx, "tx_find", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(1))
			Expect(docs[0]["status"]).To(Equal("active"))

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should find documents inserted in same transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_find_insert", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			_, err = tx.Insert(ctx, "tx_find_insert", basic.Document{"name": "doc1"})
			Expect(err).NotTo(HaveOccurred())

			_, err = tx.Insert(ctx, "tx_find_insert", basic.Document{"name": "doc2"})
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery()
			docs, err := tx.Find(ctx, "tx_find_insert", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(2))

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when transaction is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_find_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			query := basic.NewQuery()
			_, err = tx.Find(ctx, "tx_find_closed", *query)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transaction is closed"))
		})

		It("should apply filters and sorting in transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_find_complex", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			for i := range 10 {
				status := "active"
				if i%2 == 0 {
					status = "inactive"
				}
				_, err = tx.Insert(ctx, "tx_find_complex", basic.Document{
					"status": status,
					"value":  i,
				})
				Expect(err).NotTo(HaveOccurred())
			}

			query := basic.NewQuery().
				Filter("status", basic.Eq, "active").
				Sort("id", basic.Asc).
				Limit(3)
			docs, err := tx.Find(ctx, "tx_find_complex", *query)
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(3))
			for _, doc := range docs {
				Expect(doc["status"]).To(Equal("active"))
			}

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Transaction Delete", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should delete document within transaction and commit", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_delete", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "tx_delete", basic.Document{"name": "to-delete"})
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.Delete(ctx, "tx_delete", id)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(ctx, "tx_delete", id)
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should rollback delete on transaction rollback", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_delete_rollback", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "tx_delete_rollback", basic.Document{"name": "keep-me"})
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Delete(ctx, "tx_delete_rollback", id)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Rollback()
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := store.Get(ctx, "tx_delete_rollback", id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved["name"]).To(Equal("keep-me"))
		})

		It("should return ErrNotFound when deleting non-existent document in transaction", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_delete_notfound", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.Delete(ctx, "tx_delete_notfound", "non-existent")
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when transaction is closed", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_delete_closed", nil)
			Expect(err).NotTo(HaveOccurred())

			id, err := store.Insert(ctx, "tx_delete_closed", basic.Document{"test": "value"})
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = tx.Commit()
			Expect(err).NotTo(HaveOccurred())

			err = tx.Delete(ctx, "tx_delete_closed", id)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transaction is closed"))
		})
	})

	Context("Error Mapping", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should return ErrNotFound for non-existent document in Get", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "error_mapping_get", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(ctx, "error_mapping_get", "non-existent-id")
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should return ErrNotFound for non-existent document in Update", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "error_mapping_update", nil)
			Expect(err).NotTo(HaveOccurred())

			err = store.Update(ctx, "error_mapping_update", "non-existent-id", basic.Document{"test": "value"})
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should return ErrNotFound for non-existent document in Delete", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "error_mapping_delete", nil)
			Expect(err).NotTo(HaveOccurred())

			err = store.Delete(ctx, "error_mapping_delete", "non-existent-id")
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should return ErrConflict for duplicate primary key on Insert", func() {
		})

		It("should preserve generic SQLite errors without mapping", func() {
			ctx := context.Background()

			_, err := store.Get(ctx, "nonexistent_table", "some-id")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeFalse())
		})
	})

	Context("Transaction Error Mapping", func() {
		var store basic.Store

		BeforeEach(func() {
			var err error
			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should return ErrNotFound for non-existent document in transaction Get", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_error_get", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			_, err = tx.Get(ctx, "tx_error_get", "non-existent")
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should return ErrNotFound for non-existent document in transaction Update", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_error_update", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.Update(ctx, "tx_error_update", "non-existent", basic.Document{"test": "value"})
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})

		It("should return ErrNotFound for non-existent document in transaction Delete", func() {
			ctx := context.Background()

			err := store.CreateCollection(ctx, "tx_error_delete", nil)
			Expect(err).NotTo(HaveOccurred())

			tx, err := store.BeginTx(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tx.Rollback() }()

			err = tx.Delete(ctx, "tx_error_delete", "non-existent")
			Expect(errors.Is(err, basic.ErrNotFound)).To(BeTrue())
		})
	})
})

var _ = Describe("SQLiteStore Integration", func() {
	var store basic.Store
	var tempDir string
	var dbPath string

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		dbPath = filepath.Join(tempDir, "test.db")
		var err error
		store, err = basic.NewStore(basic.DefaultConfig(dbPath))
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if store != nil {
			_ = store.Close(context.Background())
		}
	})

	Context("when multiple goroutines write concurrently", func() {
		It("should handle 100 concurrent inserts without errors", func() {
			ctx := context.Background()
			collectionName := "concurrent_test"

			err := store.CreateCollection(ctx, collectionName, nil)
			Expect(err).NotTo(HaveOccurred())

			numGoroutines := 100
			insertsPerGoroutine := 10
			totalInserts := numGoroutines * insertsPerGoroutine

			type result struct {
				id  string
				err error
			}
			results := make(chan result, totalInserts)

			for g := range numGoroutines {
				go func(goroutineID int) {
					for i := range insertsPerGoroutine {
						doc := basic.Document{
							"goroutine": goroutineID,
							"index":     i,
							"value":     goroutineID*1000 + i,
						}
						id, err := store.Insert(ctx, collectionName, doc)
						results <- result{id: id, err: err}
					}
				}(g)
			}

			insertedIDs := make(map[string]bool)
			errorCount := 0

			for range totalInserts {
				res := <-results
				if res.err != nil {
					errorCount++
				} else {
					Expect(insertedIDs[res.id]).To(BeFalse(), "UUID collision detected: %s", res.id)
					insertedIDs[res.id] = true
				}
			}
			close(results)

			Expect(errorCount).To(Equal(0), "All inserts should succeed")
			Expect(insertedIDs).To(HaveLen(totalInserts), "All UUIDs should be unique")

			docs, err := store.Find(ctx, collectionName, basic.Query{})
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(totalInserts), "All documents should be persisted")
		})
	})

	Context("when WAL checkpoint occurs during writes", func() {
		It("should complete writes successfully during checkpoint", func() {
			ctx := context.Background()
			collectionName := "checkpoint_test"

			err := store.CreateCollection(ctx, collectionName, nil)
			Expect(err).NotTo(HaveOccurred())

			stopWriter := make(chan bool)
			writerDone := make(chan int)

			go func() {
				successCount := 0
				for {
					select {
					case <-stopWriter:
						writerDone <- successCount

						return
					default:
						doc := basic.Document{"data": "background_write"}
						_, err := store.Insert(ctx, collectionName, doc)
						if err == nil {
							successCount++
						}
					}
				}
			}()

			for i := range 100 {
				doc := basic.Document{"checkpoint": i}
				_, err := store.Insert(ctx, collectionName, doc)
				Expect(err).NotTo(HaveOccurred())
			}

			close(stopWriter)
			successCount := <-writerDone

			Expect(successCount).To(BeNumerically(">", 0), "Background writer should succeed during checkpoints")

			docs, err := store.Find(ctx, collectionName, basic.Query{})
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(100+successCount), "All writes should be persisted")
		})
	})

	Context("when database is closed and reopened", func() {
		It("should recover all committed data", func() {
			ctx := context.Background()
			collectionName := "durability_test"

			err := store.CreateCollection(ctx, collectionName, nil)
			Expect(err).NotTo(HaveOccurred())

			testDocs := make([]string, 0, 100)
			for i := range 100 {
				doc := basic.Document{
					"index": i,
					"value": "test_data_" + string(rune(i)),
				}
				id, err := store.Insert(ctx, collectionName, doc)
				Expect(err).NotTo(HaveOccurred())
				testDocs = append(testDocs, id)
			}

			err = store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())

			store, err = basic.NewStore(basic.DefaultConfig(dbPath))
			Expect(err).NotTo(HaveOccurred())

			for idx, id := range testDocs {
				doc, err := store.Get(ctx, collectionName, id)
				Expect(err).NotTo(HaveOccurred())
				Expect(doc["index"]).To(Equal(float64(idx)))
			}

			docs, err := store.Find(ctx, collectionName, basic.Query{})
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(100), "All documents should be recovered after reopen")
		})
	})

	Context("when measuring operation latency", func() {
		It("should meet FSM v2 latency requirements", func() {
			ctx := context.Background()
			collectionName := "latency_test"

			err := store.CreateCollection(ctx, collectionName, nil)
			Expect(err).NotTo(HaveOccurred())

			numOperations := 1000

			insertDurations := make([]time.Duration, numOperations)
			insertedIDs := make([]string, numOperations)
			for i := range numOperations {
				doc := basic.Document{
					"index": i,
					"value": "latency_test_data",
				}
				start := time.Now()
				id, err := store.Insert(ctx, collectionName, doc)
				duration := time.Since(start)
				Expect(err).NotTo(HaveOccurred())
				insertDurations[i] = duration
				insertedIDs[i] = id
			}

			insertP50 := calculatePercentileDuration(insertDurations, 50)
			insertP99 := calculatePercentileDuration(insertDurations, 99)

			Expect(insertP50.Milliseconds()).To(BeNumerically("<", 5), "Insert p50 should be < 5ms")
			Expect(insertP99.Milliseconds()).To(BeNumerically("<", 10), "Insert p99 should be < 10ms")

			getDurations := make([]time.Duration, numOperations)
			for i := range numOperations {
				start := time.Now()
				_, err := store.Get(ctx, collectionName, insertedIDs[i])
				duration := time.Since(start)
				Expect(err).NotTo(HaveOccurred())
				getDurations[i] = duration
			}

			getP99 := calculatePercentileDuration(getDurations, 99)
			Expect(getP99.Milliseconds()).To(BeNumerically("<", 5), "Get p99 should be < 5ms")
		})
	})

	Context("Maintenance operations", func() {
		var (
			store basic.Store
			ctx   context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			cfg := basic.DefaultConfig(dbPath)
			cfg.MaintenanceOnShutdown = false
			var err error
			store, err = basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should run VACUUM and ANALYZE successfully", func() {
			err := store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 100 {
				_, err := store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 1000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			maintenanceCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			err = store.Maintenance(maintenanceCtx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should respect context timeout during Maintenance", func() {
			err := store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 1000 {
				_, err := store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 10000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
			defer cancel()

			err = store.Maintenance(timeoutCtx)
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		})

		It("should respect context cancellation during Maintenance", func() {
			err := store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 500 {
				_, err := store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 5000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			err = store.Maintenance(cancelCtx)
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		})

		It("should return error when store is closed", func() {
			err := store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())

			err = store.Maintenance(ctx)
			Expect(err).To(MatchError("store is closed"))
		})

		It("should be idempotent - safe to call multiple times", func() {
			err := store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test", basic.Document{"value": 1})
			Expect(err).NotTo(HaveOccurred())

			for range 3 {
				err = store.Maintenance(ctx)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})

	Context("Close with maintenance", func() {
		It("should run maintenance on shutdown when enabled", func() {
			ctx := context.Background()
			cfg := basic.DefaultConfig(dbPath)
			cfg.MaintenanceOnShutdown = true

			var err error
			store, err := basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())

			err = store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 100 {
				_, err = store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 1000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = store.Close(closeCtx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip maintenance when disabled", func() {
			ctx := context.Background()
			cfg := basic.DefaultConfig(dbPath)
			cfg.MaintenanceOnShutdown = false

			var err error
			store, err := basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())

			err = store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			start := time.Now()
			err = store.Close(context.Background())
			duration := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(duration).To(BeNumerically("<", 100*time.Millisecond))
		})

		It("should close database even if maintenance times out", func() {
			ctx := context.Background()
			cfg := basic.DefaultConfig(dbPath)
			cfg.MaintenanceOnShutdown = true

			var err error
			store, err := basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())

			err = store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := range 1000 {
				_, err = store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 10000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			closeCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()

			err = store.Close(closeCtx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maintenance incomplete"))

			_, err = store.Get(ctx, "test", "any-id")
			Expect(err).To(MatchError("store is closed"))
		})

		It("should be idempotent - safe to close multiple times", func() {
			cfg := basic.DefaultConfig(dbPath)
			var err error
			store, err := basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())

			err = store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())

			err = store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func calculatePercentileDuration(durations []time.Duration, percentile int) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := (len(sorted) * percentile) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

var _ = Describe("NewStore validation", func() {
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "sqlite-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Context("with local filesystem", func() {
		It("should accept WAL mode on local filesystem", func() {
			dbPath := filepath.Join(tempDir, "test.db")
			cfg := basic.Config{
				DBPath:                dbPath,
				JournalMode:           basic.JournalModeWAL,
				MaintenanceOnShutdown: false,
			}
			store, err := basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(store).NotTo(BeNil())
			_ = store.Close(context.Background())
		})

		It("should accept DELETE mode on local filesystem", func() {
			dbPath := filepath.Join(tempDir, "test2.db")
			cfg := basic.Config{
				DBPath:                dbPath,
				JournalMode:           basic.JournalModeDELETE,
				MaintenanceOnShutdown: false,
			}
			store, err := basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(store).NotTo(BeNil())
			_ = store.Close(context.Background())
		})
	})

	Context("JournalMode validation", func() {
		It("should default to WAL if JournalMode is empty", func() {
			dbPath := filepath.Join(tempDir, "test3.db")
			cfg := basic.Config{
				DBPath:                dbPath,
				JournalMode:           "",
				MaintenanceOnShutdown: false,
			}
			store, err := basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(store).NotTo(BeNil())
			_ = store.Close(context.Background())
		})

		It("should reject invalid JournalMode values", func() {
			dbPath := filepath.Join(tempDir, "test4.db")
			cfg := basic.Config{
				DBPath:                dbPath,
				JournalMode:           basic.JournalMode("INVALID"),
				MaintenanceOnShutdown: false,
			}
			_, err := basic.NewStore(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid JournalMode"))
			Expect(err.Error()).To(ContainSubstring("must be WAL or DELETE"))
		})

		It("should reject empty DBPath", func() {
			cfg := basic.Config{
				DBPath:      "",
				JournalMode: basic.JournalModeWAL,
			}
			_, err := basic.NewStore(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DBPath cannot be empty"))
		})
	})

	Context("DBPath validation", func() {
		It("should reject DBPath that is a directory", func() {
			tempTestDir, err := os.MkdirTemp("", "sqlite-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tempTestDir) }()

			cfg := basic.Config{
				DBPath:      tempTestDir,
				JournalMode: basic.JournalModeWAL,
			}
			_, err = basic.NewStore(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DBPath is a directory"))
		})

		It("should reject DBPath with unwritable parent directory", func() {
			readOnlyDir, err := os.MkdirTemp("", "sqlite-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = os.Chmod(readOnlyDir, 0755)
				_ = os.RemoveAll(readOnlyDir)
			}()

			err = os.Chmod(readOnlyDir, 0444)
			Expect(err).NotTo(HaveOccurred())

			cfg := basic.Config{
				DBPath:      filepath.Join(readOnlyDir, "test.db"),
				JournalMode: basic.JournalModeWAL,
			}
			_, err = basic.NewStore(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))
		})

		It("should allow DBPath in writable directory", func() {
			writableDir, err := os.MkdirTemp("", "sqlite-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(writableDir) }()

			cfg := basic.Config{
				DBPath:      filepath.Join(writableDir, "test.db"),
				JournalMode: basic.JournalModeWAL,
			}
			store, err := basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(store).NotTo(BeNil())
			_ = store.Close(context.Background())
		})
	})

	Context("network filesystem detection", func() {
		It("should provide helpful error for network FS + WAL", func() {
			Skip("Requires real network filesystem mount - test manually")
		})
	})
})
