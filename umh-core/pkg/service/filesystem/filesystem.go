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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

// Service provides an interface for filesystem operations
// This allows for easier testing and separation of concerns
type Service interface {
	// EnsureDirectory creates a directory if it doesn't exist
	EnsureDirectory(ctx context.Context, path string) error

	// ReadFile reads a file's contents respecting the context
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// ReadFileRange reads the file starting at byte offset "from" and returns:
	//   - chunk   – the data that was read (nil if nothing new)
	//   - newSize – the file size **after** the read (use it as next offset)
	ReadFileRange(ctx context.Context, path string, from int64) ([]byte, int64, error)

	// WriteFile writes data to a file respecting the context
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error

	// PathExists checks if a file or directory exists at the given path
	PathExists(ctx context.Context, path string) (bool, error)

	// FileExists checks if a file exists
	// Deprecated: use PathExists instead
	FileExists(ctx context.Context, path string) (bool, error)

	// Remove removes a file or directory
	Remove(ctx context.Context, path string) error

	// RemoveAll removes a directory and all its contents
	RemoveAll(ctx context.Context, path string) error

	// Stat returns file info
	Stat(ctx context.Context, path string) (os.FileInfo, error)

	// Chmod changes the mode of the named file
	Chmod(ctx context.Context, path string, mode os.FileMode) error

	// ReadDir reads a directory, returning all its directory entries
	ReadDir(ctx context.Context, path string) ([]os.DirEntry, error)

	// ExecuteCommand executes a command with context
	ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error)

	// Chown changes the owner and group of the named file
	Chown(ctx context.Context, path string, user string, group string) error

	// Glob is a wrapper around filepath.Glob that respects the context
	Glob(ctx context.Context, pattern string) ([]string, error)

	// Rename renames (moves) a file or directory from oldPath to newPath.
	// This operation is atomic on the same filesystem mount.
	Rename(ctx context.Context, oldPath, newPath string) error
}

// DefaultService is the default implementation of FileSystemService
type DefaultService struct{}

// NewDefaultService creates a new DefaultFileSystemService
func NewDefaultService() *DefaultService {
	return &DefaultService{}
}

// checkContext checks if the context is done before proceeding with an operation
func (s *DefaultService) checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// EnsureDirectory creates a directory if it doesn't exist
func (s *DefaultService) EnsureDirectory(ctx context.Context, path string) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run operation in goroutine
	go func() {
		errCh <- os.MkdirAll(path, 0755)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReadFile reads a file's contents respecting the context
func (s *DefaultService) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		data []byte
		err  error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		data, err := os.ReadFile(path)
		resCh <- result{data, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, res.err
		}
		return res.data, nil
	case <-ctx.Done():
		err := ctx.Err()
		if err == nil {
			err = fmt.Errorf("context cancelled")
		}
		return nil, err
	}
}

// ReadFileRange reads the file starting at byte offset "from" and returns:
//   - chunk   – the data that was read (nil if nothing new)
//   - newSize – the file size **after** the read (use it as next offset)
func (s *DefaultService) ReadFileRange(
	ctx context.Context,
	path string,
	from int64,
) ([]byte, int64, error) {

	if err := s.checkContext(ctx); err != nil {
		return nil, 0, err
	}

	type result struct {
		data    []byte
		newSize int64
		err     error
	}
	resCh := make(chan result, 1)

	go func() {
		f, err := os.Open(path)
		if err != nil {
			resCh <- result{nil, 0, err}
			return
		}
		defer func() {
			if err := f.Close(); err != nil {
				// Sending to resCh will block the goroutine, so we don't want to do that
				// Instead, we log the error
				sentry.ReportServiceError(logger.For(logger.ComponentFilesystemService), "ReadFileRange", "failed to close file %s: %s", path, err)
			}
		}()

		// stat *after* open so we have a consistent view
		fi, err := f.Stat()
		if err != nil {
			resCh <- result{nil, 0, err}
			return
		}
		size := fi.Size()

		// nothing new?
		if from >= size {
			resCh <- result{nil, size, nil}
			return
		}

		if _, err = f.Seek(from, io.SeekStart); err != nil {
			resCh <- result{nil, 0, err}
			return
		}

		buf := make([]byte, size-from)
		if _, err = io.ReadFull(f, buf); err != nil {
			resCh <- result{nil, 0, err}
			return
		}

		resCh <- result{buf, size, nil}
	}()

	select {
	case res := <-resCh:
		return res.data, res.newSize, res.err
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

// WriteFile writes data to a file respecting the context
func (s *DefaultService) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.WriteFile(path, data, perm)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PathExists checks if a path (file or directory) exists
func (s *DefaultService) PathExists(ctx context.Context, path string) (bool, error) {
	if err := s.checkContext(ctx); err != nil {
		return false, err
	}

	// Create a channel for results
	type result struct {
		exists bool
		err    error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			resCh <- result{false, nil}
			return
		}
		if err != nil {
			resCh <- result{false, fmt.Errorf("failed to check if path exists: %w", err)}
			return
		}
		resCh <- result{true, nil}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return false, res.err
		}
		return res.exists, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// FileExists checks if a file exists
// Deprecated: use PathExists instead
func (s *DefaultService) FileExists(ctx context.Context, path string) (bool, error) {
	return s.PathExists(ctx, path)
}

// Remove removes a file or directory
func (s *DefaultService) Remove(ctx context.Context, path string) error {
	if err := s.checkContext(ctx); err != nil {
		return err
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.Remove(path)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RemoveAll removes a directory and all its contents
func (s *DefaultService) RemoveAll(ctx context.Context, path string) error {
	if err := s.checkContext(ctx); err != nil {
		return err
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.RemoveAll(path)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to remove directory %s: %w", path, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stat returns file info
func (s *DefaultService) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		info os.FileInfo
		err  error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		info, err := os.Stat(path)
		resCh <- result{info, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("failed to get file info: %w", res.err)
		}
		return res.info, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Chmod changes the mode of the named file
func (s *DefaultService) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.Chmod(path, mode)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to change mode of file %s: %w", path, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReadDir reads a directory, returning all its directory entries
func (s *DefaultService) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		entries []os.DirEntry
		err     error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		entries, err := os.ReadDir(path)
		resCh <- result{entries, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("failed to read directory %s: %w", path, res.err)
		}
		return res.entries, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Chown changes the owner and group of the named file
func (s *DefaultService) Chown(ctx context.Context, path string, user string, group string) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		// Use chown command as os.Chown needs numeric user/group IDs
		cmd := exec.Command("chown", fmt.Sprintf("%s:%s", user, group), path)
		errCh <- cmd.Run()
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to change owner of file %s to %s:%s: %w", path, user, group, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ExecuteCommand executes a command with context
func (s *DefaultService) ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentFilesystem, name+".executeCommand."+name, time.Since(start))
	}()

	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// This method already respects context cancellation through exec.CommandContext
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute command %s: %w", name, err)
	}
	return output, nil
}

// Glob is a wrapper around filepath.Glob that respects the context
func (s *DefaultService) Glob(ctx context.Context, pattern string) ([]string, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		matches []string
		err     error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		matches, err := filepath.Glob(pattern)
		resCh <- result{matches, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("failed to glob pattern %s: %w", pattern, res.err)
		}
		return res.matches, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Rename renames (moves) a file or directory from oldPath to newPath.
// This operation is atomic on the same filesystem mount.
func (s *DefaultService) Rename(ctx context.Context, oldPath, newPath string) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.Rename(oldPath, newPath)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to rename file %s to %s: %w", oldPath, newPath, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
