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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

// Service provides an interface for filesystem operations
// This allows for easier testing and separation of concerns.
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

	// Symlink creates a symbolic link from linkPath to target.
	// linkPath is the path where the symlink will be created.
	// target is the path that the symlink will point to.
	Symlink(ctx context.Context, target, linkPath string) error
}

// chunkBufferPool provides reusable buffers for efficient file reading
// CONCURRENCY SAFETY: sync.Pool is thread-safe and handles concurrent Get/Put operations
// MEMORY EFFICIENCY: Reuses 1MB buffers across goroutines to reduce GC pressure
// BUFFER REUSE: Each buffer is completely overwritten by io.ReadFull before being appended to result.
var chunkBufferPool = sync.Pool{
	New: func() interface{} {
		chunk := make([]byte, constants.S6FileReadChunkSize)

		return &chunk
	},
}

// DefaultService is the default implementation of FileSystemService.
type DefaultService struct{}

// NewDefaultService creates a new DefaultFileSystemService.
func NewDefaultService() *DefaultService {
	return &DefaultService{}
}

// checkContext checks if the context is done before proceeding with an operation.
func (s *DefaultService) checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// EnsureDirectory creates a directory if it doesn't exist.
func (s *DefaultService) EnsureDirectory(ctx context.Context, path string) error {
	err := s.checkContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run operation in goroutine
	go func() {
		errCh <- os.MkdirAll(path, 0750)
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

// ReadFile reads a file's contents respecting the context.
func (s *DefaultService) ReadFile(ctx context.Context, path string) ([]byte, error) {
	err := s.checkContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		err  error
		data []byte
	}

	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		data, err := os.ReadFile(path) //nolint:gosec // G304: Core filesystem service ReadFile method, path managed by filesystem abstraction
		resCh <- result{err: err, data: data}
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
			err = errors.New("context cancelled")
		}

		return nil, err
	}
}

// ReadFileRange reads the file starting at byte offset "from" and returns:
//   - chunk   – the data that was read (nil if nothing new)
//   - newSize – the file size **after** the read (use it as next offset)
//
// PERFORMANCE OPTIMIZATIONS:
// 1. MEMORY EFFICIENT: Pre-allocates result buffer to avoid repeated allocations during append()
// 2. BUFFER POOLING: Reuses 1MB working buffers via sync.Pool to reduce GC pressure
// 3. CONTEXT AWARE: Checks deadline before each chunk, returns SUCCESS (not timeout) if insufficient time
// 4. CHUNKED I/O: Reads in 1MB chunks for optimal I/O performance while maintaining memory control
//
// CORRECTNESS GUARANTEES:
// - Result buffer contains ONLY actual file data (no padding/zeros)
// - newSize is always the correct next read offset (original_from + bytes_read)
// - Graceful degradation: partial reads return success, allowing incremental progress
// - Thread-safe: Buffer pool handles concurrent access safely.
func (s *DefaultService) ReadFileRange(
	ctx context.Context,
	path string,
	from int64,
) ([]byte, int64, error) {
	err := s.checkContext(ctx)
	if err != nil {
		return nil, 0, err
	}

	type result struct {
		err     error
		data    []byte
		newSize int64
	}

	resCh := make(chan result, 1)

	go func() {
		f, err := os.Open(path) //nolint:gosec // G304: Core filesystem service ReadFileRange method, path managed by filesystem abstraction
		if err != nil {
			resCh <- result{err: err, data: nil, newSize: 0}

			return
		}

		defer func() {
			err := f.Close()
			if err != nil {
				// Sending to resCh will block the goroutine, so we don't want to do that
				// Instead, we log the error
				sentry.ReportServiceError(logger.For(logger.ComponentFilesystemService), "ReadFileRange", "failed to close file %s: %s", path, err)
			}
		}()

		// stat *after* open so we have a consistent view
		fi, err := f.Stat()
		if err != nil {
			resCh <- result{err: err, data: nil, newSize: 0}

			return
		}

		size := fi.Size()

		// nothing new?
		if from >= size {
			resCh <- result{err: nil, data: nil, newSize: size}

			return
		}

		if _, err = f.Seek(from, io.SeekStart); err != nil {
			resCh <- result{err: err, data: nil, newSize: 0}

			return
		}

		// CONTEXT-AWARE TIMING: Check remaining time before each chunk to avoid timeout failures
		// If less than timeBuffer remains, gracefully exit with partial data instead of timing out
		deadline, ok := ctx.Deadline()
		if !ok {
			resCh <- result{err: errors.New("context deadline not set"), data: nil, newSize: 0}

			return
		}

		timeBuffer := time.Until(deadline) / 2

		// MEMORY PREALLOCATION: Pre-allocate capacity but start with zero length
		// This avoids repeated allocations during append() while not wasting memory
		expectedSize := size - from
		buf := make([]byte, 0, expectedSize)

		// BUFFER POOL USAGE: Get reusable 1MB buffer to minimize allocations
		// The buffer gets completely overwritten by io.ReadFull each time
		smallBuf, ok := chunkBufferPool.Get().(*[]byte)
		if !ok {
			panic("chunkBufferPool returned unexpected type")
		}
		// Sanity check (can't happen unless code changes)
		if smallBuf == nil {
			resCh <- result{err: errors.New("smallBuf is nil"), data: nil, newSize: 0}

			return
		}

		defer chunkBufferPool.Put(smallBuf)

		for {
			// GRACEFUL EARLY EXIT: Check if enough time remains for another chunk
			// Returns SUCCESS with partial data instead of TIMEOUT failure
			if deadline, ok := ctx.Deadline(); ok {
				if remaining := time.Until(deadline); remaining < timeBuffer {
					// NEWSIZE CALCULATION: from + bytes_read = next offset to read from
					resCh <- result{err: nil, data: buf, newSize: from + int64(len(buf))}

					return
				}
			}

			// Read chunk: io.ReadFull either reads exactly len(smallBuf) bytes OR returns error
			n, err := io.ReadFull(f, *smallBuf)
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				if n > 0 {
					// PARTIAL READ: Use smallBuf[:n] to avoid appending garbage data
					// This is why buf contains NO EXTRA ZEROS - we only append actual file data
					buf = append(buf, (*smallBuf)[:n]...)
				}

				break
			}

			if err != nil {
				resCh <- result{err: err, data: nil, newSize: 0}

				return
			}

			// FULL READ SUCCESS: io.ReadFull guarantees smallBuf contains exactly 1MB of file data
			// No slicing needed here - the entire buffer contains valid data
			buf = append(buf, *smallBuf...)
		}

		// FINAL RESULT: buf contains only actual file data, newSize = next read offset
		resCh <- result{err: nil, data: buf, newSize: from + int64(len(buf))}
	}()

	select {
	case res := <-resCh:
		return res.data, res.newSize, res.err
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

// WriteFile writes data to a file respecting the context.
func (s *DefaultService) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	err := s.checkContext(ctx)
	if err != nil {
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

// PathExists checks if a path (file or directory) exists.
func (s *DefaultService) PathExists(ctx context.Context, path string) (bool, error) {
	err := s.checkContext(ctx)
	if err != nil {
		return false, err
	}

	// Create a channel for results
	type result struct {
		err    error
		exists bool
	}

	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			resCh <- result{err: nil, exists: false}

			return
		}

		if err != nil {
			resCh <- result{err: fmt.Errorf("failed to check if path exists: %w", err), exists: false}

			return
		}

		resCh <- result{err: nil, exists: true}
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
// Deprecated: use PathExists instead.
func (s *DefaultService) FileExists(ctx context.Context, path string) (bool, error) {
	return s.PathExists(ctx, path)
}

// Remove removes a file or directory.
func (s *DefaultService) Remove(ctx context.Context, path string) error {
	err := s.checkContext(ctx)
	if err != nil {
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

// RemoveAll removes a directory and all its contents.
func (s *DefaultService) RemoveAll(ctx context.Context, path string) error {
	err := s.checkContext(ctx)
	if err != nil {
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

// Stat returns file info.
func (s *DefaultService) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	err := s.checkContext(ctx)
	if err != nil {
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

// Chmod changes the mode of the named file.
func (s *DefaultService) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	err := s.checkContext(ctx)
	if err != nil {
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

// ReadDir reads a directory, returning all its directory entries.
func (s *DefaultService) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	err := s.checkContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		err     error
		entries []os.DirEntry
	}

	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		entries, err := os.ReadDir(path)
		resCh <- result{err: err, entries: entries}
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

// Chown changes the owner and group of the named file.
func (s *DefaultService) Chown(ctx context.Context, path string, user string, group string) error {
	err := s.checkContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Validate inputs to prevent command injection
	if err := validateChownInputs(path, user, group); err != nil {
		return fmt.Errorf("invalid chown parameters: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		// Use chown command as os.Chown needs numeric user/group IDs
		cmd := exec.Command("chown", fmt.Sprintf("%s:%s", user, group), path) //nolint:gosec // G204: Inputs validated by validateChownInputs, safe subprocess execution
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

// ExecuteCommand executes a command with context.
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

// Glob is a wrapper around filepath.Glob that respects the context.
func (s *DefaultService) Glob(ctx context.Context, pattern string) ([]string, error) {
	err := s.checkContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		err     error
		matches []string
	}

	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		matches, err := filepath.Glob(pattern)
		resCh <- result{err: err, matches: matches}
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
	err := s.checkContext(ctx)
	if err != nil {
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

// Symlink creates a symbolic link from linkPath to target.
func (s *DefaultService) Symlink(ctx context.Context, target, linkPath string) error {
	err := s.checkContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.Symlink(target, linkPath)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to create symlink %s -> %s: %w", linkPath, target, err)
		}

		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// validateChownInputs validates user, group, and path inputs to prevent command injection.
func validateChownInputs(path, user, group string) error {
	// Validate path - must be absolute and not contain shell metacharacters
	if !filepath.IsAbs(path) {
		return errors.New("path must be absolute")
	}

	// Check for dangerous characters in path
	if strings.ContainsAny(path, ";|&$`<>(){}[]?*") {
		return errors.New("path contains illegal characters")
	}

	// Validate user/group names - allow alphanumeric, underscore, dash, and dots
	validName := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

	if user == "" {
		return errors.New("user cannot be empty")
	}

	if !validName.MatchString(user) {
		return errors.New("user contains illegal characters")
	}

	if group == "" {
		return errors.New("group cannot be empty")
	}

	if !validName.MatchString(group) {
		return errors.New("group contains illegal characters")
	}

	return nil
}
