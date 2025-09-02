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
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

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

// recordOp records filesystem operation metrics.
func (s *DefaultService) recordOp(op string, path string, start time.Time, err error, cached bool) {
	duration := time.Since(start)

	status := "success"
	if err != nil {
		status = "error"
	}

	cacheStatus := "uncached"
	if cached {
		cacheStatus = "cached"
	}

	// Use the actual path directly
	metrics.RecordFilesystemOp(op, path, status, cacheStatus, duration)
	metrics.RecordFilesystemPathAccess(op, path, cacheStatus)
}

// EnsureDirectory creates a directory if it doesn't exist.
func (s *DefaultService) EnsureDirectory(ctx context.Context, path string) error {
	start := time.Now()

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
			s.recordOp("EnsureDirectory", path, start, err, false)

			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}

		s.recordOp("EnsureDirectory", path, start, nil, false)

		return nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("EnsureDirectory", path, start, err, false)

		return err
	}
}

// ReadFile reads a file's contents respecting the context.
func (s *DefaultService) ReadFile(ctx context.Context, path string) ([]byte, error) {
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
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
		data, err := os.ReadFile(path)
		resCh <- result{err: err, data: data}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		s.recordOp("ReadFile", path, start, res.err, false)

		if res.err != nil {
			return nil, res.err
		}

		return res.data, nil
	case <-ctx.Done():
		err := ctx.Err()
		if err == nil {
			err = errors.New("context cancelled")
		}

		s.recordOp("ReadFile", path, start, err, false)

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
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
		return nil, 0, err
	}

	type result struct {
		err     error
		data    []byte
		newSize int64
	}

	resCh := make(chan result, 1)

	go func() {
		f, err := os.Open(path)
		if err != nil {
			resCh <- result{err: err, data: nil, newSize: 0}

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
		smallBuf := chunkBufferPool.Get().(*[]byte)
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
		s.recordOp("ReadFileRange", path, start, res.err, false)

		return res.data, res.newSize, res.err
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("ReadFileRange", path, start, err, false)

		return nil, 0, err
	}
}

// WriteFile writes data to a file respecting the context.
func (s *DefaultService) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	start := time.Now()

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
			s.recordOp("WriteFile", path, start, err, false)

			return fmt.Errorf("failed to write file %s: %w", path, err)
		}

		s.recordOp("WriteFile", path, start, nil, false)

		return nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("WriteFile", path, start, err, false)

		return err
	}
}

// PathExists checks if a path (file or directory) exists.
func (s *DefaultService) PathExists(ctx context.Context, path string) (bool, error) {
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
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
		s.recordOp("PathExists", path, start, res.err, false)

		if res.err != nil {
			return false, res.err
		}

		return res.exists, nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("PathExists", path, start, err, false)

		return false, err
	}
}

// FileExists checks if a file exists
// Deprecated: use PathExists instead.
func (s *DefaultService) FileExists(ctx context.Context, path string) (bool, error) {
	return s.PathExists(ctx, path)
}

// Remove removes a file or directory.
func (s *DefaultService) Remove(ctx context.Context, path string) error {
	start := time.Now()

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
		s.recordOp("Remove", path, start, err, false)

		return err
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("Remove", path, start, err, false)

		return err
	}
}

// RemoveAll removes a directory and all its contents.
func (s *DefaultService) RemoveAll(ctx context.Context, path string) error {
	start := time.Now()

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
			s.recordOp("RemoveAll", path, start, err, false)

			return fmt.Errorf("failed to remove directory %s: %w", path, err)
		}

		s.recordOp("RemoveAll", path, start, nil, false)

		return nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("RemoveAll", path, start, err, false)

		return err
	}
}

// Stat returns file info.
func (s *DefaultService) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	start := time.Now()

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
			s.recordOp("Stat", path, start, res.err, false)

			return nil, fmt.Errorf("failed to get file info: %w", res.err)
		}

		s.recordOp("Stat", path, start, nil, false)

		return res.info, nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("Stat", path, start, err, false)

		return nil, err
	}
}

// Chmod changes the mode of the named file.
func (s *DefaultService) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	start := time.Now()

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
			s.recordOp("Chmod", path, start, err, false)

			return fmt.Errorf("failed to change mode of file %s: %w", path, err)
		}

		s.recordOp("Chmod", path, start, nil, false)

		return nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("Chmod", path, start, err, false)

		return err
	}
}

// ReadDir reads a directory, returning all its directory entries.
func (s *DefaultService) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
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
			s.recordOp("ReadDir", path, start, res.err, false)

			return nil, fmt.Errorf("failed to read directory %s: %w", path, res.err)
		}

		s.recordOp("ReadDir", path, start, nil, false)

		return res.entries, nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("ReadDir", path, start, err, false)

		return nil, err
	}
}

// Chown changes the owner and group of the named file.
func (s *DefaultService) Chown(ctx context.Context, path string, user string, group string) error {
	start := time.Now()

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
			s.recordOp("Chown", fmt.Sprintf("%s:%s->%s", user, group, path), start, err, false)

			return fmt.Errorf("failed to change owner of file %s to %s:%s: %w", path, user, group, err)
		}

		s.recordOp("Chown", fmt.Sprintf("%s:%s->%s", user, group, path), start, nil, false)

		return nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("Chown", fmt.Sprintf("%s:%s->%s", user, group, path), start, err, false)

		return err
	}
}

// ExecuteCommand executes a command with context.
func (s *DefaultService) ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	start := time.Now()

	cmdStr := name
	if len(args) > 0 {
		cmdStr = fmt.Sprintf("%s %s", name, strings.Join(args, " "))
	}

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentFilesystem, name+".executeCommand."+name, time.Since(start))
	}()

	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// This method already respects context cancellation through exec.CommandContext
	cmd := exec.CommandContext(ctx, name, args...)

	output, err := cmd.CombinedOutput()
	s.recordOp("ExecuteCommand", cmdStr, start, err, false)

	if err != nil {
		return nil, fmt.Errorf("failed to execute command %s: %w", name, err)
	}

	return output, nil
}

// Glob is a wrapper around filepath.Glob that respects the context.
func (s *DefaultService) Glob(ctx context.Context, pattern string) ([]string, error) {
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
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
			s.recordOp("Glob", pattern, start, res.err, false)

			return nil, fmt.Errorf("failed to glob pattern %s: %w", pattern, res.err)
		}

		s.recordOp("Glob", pattern, start, nil, false)

		return res.matches, nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("Glob", pattern, start, err, false)

		return nil, err
	}
}

// Rename renames (moves) a file or directory from oldPath to newPath.
// This operation is atomic on the same filesystem mount.
func (s *DefaultService) Rename(ctx context.Context, oldPath, newPath string) error {
	start := time.Now()

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
			s.recordOp("Rename", fmt.Sprintf("%s->%s", oldPath, newPath), start, err, false)

			return fmt.Errorf("failed to rename file %s to %s: %w", oldPath, newPath, err)
		}

		s.recordOp("Rename", fmt.Sprintf("%s->%s", oldPath, newPath), start, nil, false)

		return nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("Rename", fmt.Sprintf("%s->%s", oldPath, newPath), start, err, false)

		return err
	}
}

// Symlink creates a symbolic link from linkPath to target.
func (s *DefaultService) Symlink(ctx context.Context, target, linkPath string) error {
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
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
			s.recordOp("Symlink", fmt.Sprintf("%s->%s", target, linkPath), start, err, false)

			return fmt.Errorf("failed to create symlink %s -> %s: %w", linkPath, target, err)
		}

		s.recordOp("Symlink", fmt.Sprintf("%s->%s", target, linkPath), start, nil, false)

		return nil
	case <-ctx.Done():
		err := ctx.Err()
		s.recordOp("Symlink", fmt.Sprintf("%s->%s", target, linkPath), start, err, false)

		return err
	}
}
