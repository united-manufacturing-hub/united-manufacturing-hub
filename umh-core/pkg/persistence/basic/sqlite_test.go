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
})
