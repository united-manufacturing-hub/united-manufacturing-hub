package examples

import (
	"context"
	"io/fs"
	"os"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// mockFilesystem is an in-memory filesystem.Service for scenarios that drive
// a helloworld child's mood file. It implements only ReadFile, the one method
// helloworld calls; any other method of the interface panics.
type mockFilesystem struct {
	filesystem.Service

	mu    sync.Mutex
	files map[string][]byte
}

// SetFile stores contents under path, replacing any previous version.
func (m *mockFilesystem) SetFile(path, contents string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.files[path] = []byte(contents)
}

// RemoveFile deletes path from the mock, so a later read reports it missing.
func (m *mockFilesystem) RemoveFile(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.files, path)
}

// ReadFile returns a copy of the bytes stored under path, or os.ErrNotExist
// wrapped in a *fs.PathError when the mock holds no such path.
func (m *mockFilesystem) ReadFile(_ context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	contents, ok := m.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: path, Err: os.ErrNotExist}
	}

	out := make([]byte, len(contents))
	copy(out, contents)

	return out, nil
}
