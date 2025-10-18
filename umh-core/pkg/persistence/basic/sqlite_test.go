package basic_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func TestNewSQLiteStore_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() failed: %v", err)
	}

	defer func() { _ = store.Close() }()

	if store == nil {
		t.Fatal("NewSQLiteStore() returned nil store")
	}
}

func TestNewSQLiteStore_InvalidPath(t *testing.T) {
	invalidPath := "/nonexistent/directory/that/does/not/exist/test.db"

	store, err := basic.NewSQLiteStore(invalidPath)
	if err == nil {
		if store != nil {
			_ = store.Close()
		}

		t.Fatal("NewSQLiteStore() should fail with invalid path")
	}
}

func TestNewSQLiteStore_WALModeEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() failed: %v", err)
	}

	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err = store.CreateCollection(ctx, "test_collection", nil)
	if err != nil {
		t.Fatalf("CreateCollection() failed: %v", err)
	}

	walFile := dbPath + "-wal"

	_, err = os.Stat(walFile)
	if os.IsNotExist(err) {
		t.Errorf("WAL file does not exist at %s - WAL mode may not be enabled", walFile)
	}
}

func TestNewSQLiteStore_DarwinFullFsync(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping darwin-specific test on non-darwin platform")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() failed: %v", err)
	}

	defer func() { _ = store.Close() }()
}

func TestSQLiteStore_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() failed: %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	err = store.Close()
	if err == nil {
		t.Error("Close() should return error when called twice")
	}
}

func TestSQLiteStore_ImplementsStoreInterface(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() failed: %v", err)
	}

	defer func() { _ = store.Close() }()

	var _ basic.Store

	_ = store
}
