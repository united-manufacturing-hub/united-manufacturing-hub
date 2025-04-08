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

package filesystem

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MockFileSystem is a mock implementation of the filesystem.Service interface
type MockFileSystem struct {
	FailureRate         float64 // 0.0 to 1.0
	DelayRange          time.Duration
	FailedOperations    map[string]bool
	ReadFileFunc        func(ctx context.Context, path string) ([]byte, error)
	WriteFileFunc       func(ctx context.Context, path string, data []byte, perm os.FileMode) error
	PathExistsFunc      func(ctx context.Context, path string) (bool, error)
	FileExistsFunc      func(ctx context.Context, path string) (bool, error)
	EnsureDirectoryFunc func(ctx context.Context, path string) error
	RemoveFunc          func(ctx context.Context, path string) error
	RemoveAllFunc       func(ctx context.Context, path string) error
	StatFunc            func(ctx context.Context, path string) (os.FileInfo, error)
	CreateFileFunc      func(ctx context.Context, path string, perm os.FileMode) (*os.File, error)
	ChmodFunc           func(ctx context.Context, path string, mode os.FileMode) error
	ReadDirFunc         func(ctx context.Context, path string) ([]os.DirEntry, error)
	ExecuteCommandFunc  func(ctx context.Context, name string, args ...string) ([]byte, error)
	ChownFunc           func(ctx context.Context, path string, user string, group string) error
	mutex               sync.Mutex
}

// NewMockFileSystem creates a new MockFileSystem instance
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		FailureRate:      0.0,
		DelayRange:       0,
		FailedOperations: make(map[string]bool),
	}
}

// WithFailureRate sets the failure rate for the mock
func (m *MockFileSystem) WithFailureRate(rate float64) *MockFileSystem {
	m.FailureRate = rate
	return m
}

// WithDelayRange sets the delay range for the mock
func (m *MockFileSystem) WithDelayRange(delay time.Duration) *MockFileSystem {
	m.DelayRange = delay
	return m
}

// simulateRandomBehavior decides whether an operation should fail and how long it should delay
func (m *MockFileSystem) simulateRandomBehavior(operation string) (bool, time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.FailedOperations == nil {
		m.FailedOperations = make(map[string]bool)
	}

	// Check if this operation should fail
	shouldFail := rand.Float64() < m.FailureRate
	if shouldFail {
		m.FailedOperations[operation] = true
	}

	// Apply random delay (0 to DelayRange)
	delay := time.Duration(0)
	if m.DelayRange > 0 {
		delay = time.Duration(rand.Int63n(int64(m.DelayRange)))
	}

	return shouldFail, delay
}

// EnsureDirectory creates a directory if it doesn't exist
func (m *MockFileSystem) EnsureDirectory(ctx context.Context, path string) error {
	if m.EnsureDirectoryFunc != nil {
		return m.EnsureDirectoryFunc(ctx, path)
	}

	shouldFail, delay := m.simulateRandomBehavior("EnsureDirectory:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if shouldFail {
		return errors.New("simulated failure in EnsureDirectory")
	}
	return nil
}

// ReadFile reads a file's contents respecting the context
func (m *MockFileSystem) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(ctx, path)
	}

	shouldFail, delay := m.simulateRandomBehavior("ReadFile:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if shouldFail {
		return nil, errors.New("simulated failure in ReadFile")
	}
	return []byte("mock content"), nil
}

// WriteFile writes data to a file respecting the context
func (m *MockFileSystem) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	if m.WriteFileFunc != nil {
		return m.WriteFileFunc(ctx, path, data, perm)
	}

	shouldFail, delay := m.simulateRandomBehavior("WriteFile:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if shouldFail {
		return errors.New("simulated failure in WriteFile")
	}
	return nil
}

// PathExists checks if a path exists
func (m *MockFileSystem) PathExists(ctx context.Context, path string) (bool, error) {
	if m.PathExistsFunc != nil {
		return m.PathExistsFunc(ctx, path)
	}

	// For backward compatibility, respect FileExistsFunc if set
	if m.FileExistsFunc != nil {
		return m.FileExistsFunc(ctx, path)
	}

	shouldFail, delay := m.simulateRandomBehavior("PathExists:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	if shouldFail {
		return false, errors.New("simulated failure in PathExists")
	}
	return true, nil
}

// FileExists checks if a file exists
// Deprecated: use PathExists instead
func (m *MockFileSystem) FileExists(ctx context.Context, path string) (bool, error) {
	if m.FileExistsFunc != nil {
		return m.FileExistsFunc(ctx, path)
	}

	// If FileExistsFunc is not set but PathExistsFunc is, use that
	if m.PathExistsFunc != nil {
		return m.PathExistsFunc(ctx, path)
	}

	shouldFail, delay := m.simulateRandomBehavior("FileExists:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	if shouldFail {
		return false, errors.New("simulated failure in FileExists")
	}
	return true, nil
}

// Remove removes a file or directory
func (m *MockFileSystem) Remove(ctx context.Context, path string) error {
	if m.RemoveFunc != nil {
		return m.RemoveFunc(ctx, path)
	}

	shouldFail, delay := m.simulateRandomBehavior("Remove:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if shouldFail {
		return errors.New("simulated failure in Remove")
	}
	return nil
}

// RemoveAll removes a directory and all its contents
func (m *MockFileSystem) RemoveAll(ctx context.Context, path string) error {
	if m.RemoveAllFunc != nil {
		return m.RemoveAllFunc(ctx, path)
	}

	shouldFail, delay := m.simulateRandomBehavior("RemoveAll:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if shouldFail {
		return errors.New("simulated failure in RemoveAll")
	}
	return nil
}

// Stat returns file info
func (m *MockFileSystem) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	if m.StatFunc != nil {
		return m.StatFunc(ctx, path)
	}

	shouldFail, delay := m.simulateRandomBehavior("Stat:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if shouldFail {
		return nil, errors.New("simulated failure in Stat")
	}

	// Return a mock FileInfo
	return &memFileInfo{
		name:  filepath.Base(path),
		size:  0,
		mode:  0644,
		mtime: time.Now(),
		dir:   true,
	}, nil
}

// CreateFile creates a new file with the specified permissions
func (m *MockFileSystem) CreateFile(ctx context.Context, path string, perm os.FileMode) (*os.File, error) {
	if m.CreateFileFunc != nil {
		return m.CreateFileFunc(ctx, path, perm)
	}

	shouldFail, delay := m.simulateRandomBehavior("CreateFile:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if shouldFail {
		return nil, errors.New("simulated failure in CreateFile")
	}
	return nil, errors.New("not implemented")
}

// Chmod changes the mode of the named file
func (m *MockFileSystem) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	if m.ChmodFunc != nil {
		return m.ChmodFunc(ctx, path, mode)
	}

	shouldFail, delay := m.simulateRandomBehavior("Chmod:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if shouldFail {
		return errors.New("simulated failure in Chmod")
	}
	return nil
}

// Chown changes the owner and group of the named file
func (m *MockFileSystem) Chown(ctx context.Context, path string, user string, group string) error {
	if m.ChownFunc != nil {
		return m.ChownFunc(ctx, path, user, group)
	}

	shouldFail, delay := m.simulateRandomBehavior("Chown:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if shouldFail {
		return errors.New("simulated failure in Chown")
	}
	return nil
}

// ReadDir reads a directory, returning all its directory entries
func (m *MockFileSystem) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	if m.ReadDirFunc != nil {
		return m.ReadDirFunc(ctx, path)
	}

	shouldFail, delay := m.simulateRandomBehavior("ReadDir:" + path)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if shouldFail {
		return nil, errors.New("simulated failure in ReadDir")
	}
	return nil, nil
}

// ExecuteCommand executes a command with context
func (m *MockFileSystem) ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.ExecuteCommandFunc != nil {
		return m.ExecuteCommandFunc(ctx, name, args...)
	}

	shouldFail, delay := m.simulateRandomBehavior("ExecuteCommand:" + name)

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if shouldFail {
		return nil, errors.New("simulated failure in ExecuteCommand")
	}
	return []byte("mock command output"), nil
}

// WithEnsureDirectoryFunc sets a custom implementation for EnsureDirectory
func (m *MockFileSystem) WithEnsureDirectoryFunc(fn func(ctx context.Context, path string) error) *MockFileSystem {
	m.EnsureDirectoryFunc = fn
	return m
}

// WithReadFileFunc sets a custom implementation for ReadFile
func (m *MockFileSystem) WithReadFileFunc(fn func(ctx context.Context, path string) ([]byte, error)) *MockFileSystem {
	m.ReadFileFunc = fn
	return m
}

// WithWriteFileFunc sets a custom implementation for WriteFile
func (m *MockFileSystem) WithWriteFileFunc(fn func(ctx context.Context, path string, data []byte, perm os.FileMode) error) *MockFileSystem {
	m.WriteFileFunc = fn
	return m
}

// WithFileExistsFunc sets a custom function for FileExists
func (m *MockFileSystem) WithFileExistsFunc(fn func(ctx context.Context, path string) (bool, error)) *MockFileSystem {
	m.FileExistsFunc = fn
	return m
}

// WithPathExistsFunc sets a custom function for PathExists
func (m *MockFileSystem) WithPathExistsFunc(fn func(ctx context.Context, path string) (bool, error)) *MockFileSystem {
	m.PathExistsFunc = fn
	return m
}

// WithRemoveFunc sets a custom implementation for Remove
func (m *MockFileSystem) WithRemoveFunc(fn func(ctx context.Context, path string) error) *MockFileSystem {
	m.RemoveFunc = fn
	return m
}

// WithRemoveAllFunc sets a custom implementation for RemoveAll
func (m *MockFileSystem) WithRemoveAllFunc(fn func(ctx context.Context, path string) error) *MockFileSystem {
	m.RemoveAllFunc = fn
	return m
}

// WithStatFunc sets a custom implementation for Stat
func (m *MockFileSystem) WithStatFunc(fn func(ctx context.Context, path string) (os.FileInfo, error)) *MockFileSystem {
	m.StatFunc = fn
	return m
}

// WithCreateFileFunc sets a custom implementation for CreateFile
func (m *MockFileSystem) WithCreateFileFunc(fn func(ctx context.Context, path string, perm os.FileMode) (*os.File, error)) *MockFileSystem {
	m.CreateFileFunc = fn
	return m
}

// WithChmodFunc sets a custom implementation for Chmod
func (m *MockFileSystem) WithChmodFunc(fn func(ctx context.Context, path string, mode os.FileMode) error) *MockFileSystem {
	m.ChmodFunc = fn
	return m
}

// WithChownFunc sets a custom implementation for Chown
func (m *MockFileSystem) WithChownFunc(fn func(ctx context.Context, path string, user string, group string) error) *MockFileSystem {
	m.ChownFunc = fn
	return m
}

// WithReadDirFunc sets a custom implementation for ReadDir
func (m *MockFileSystem) WithReadDirFunc(fn func(ctx context.Context, path string) ([]os.DirEntry, error)) *MockFileSystem {
	m.ReadDirFunc = fn
	return m
}

// WithExecuteCommandFunc sets a custom implementation for ExecuteCommand
func (m *MockFileSystem) WithExecuteCommandFunc(fn func(ctx context.Context, name string, args ...string) ([]byte, error)) *MockFileSystem {
	m.ExecuteCommandFunc = fn
	return m
}

// NewMockFileInfo creates a new mock FileInfo for testing
func (m *MockFileSystem) NewMockFileInfo(name string, size int64, mode os.FileMode, modTime time.Time, isDir bool) os.FileInfo {
	return &memFileInfo{
		name:  name,
		size:  size,
		mode:  mode,
		mtime: modTime,
		dir:   isDir,
	}
}
