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

// CachedPath represents a cached path existence check result.
type CachedPath struct {
	expiry time.Time
	err    error
	exists bool
}

// PathCache provides thread-safe caching for path existence checks.
type PathCache struct {
	cache map[string]*CachedPath
	mu    sync.RWMutex
}

// CachedFileContent represents cached file content with metadata for invalidation.
type CachedFileContent struct {
	modTime   time.Time
	lastCheck time.Time // When we last did a stat check
	content   []byte
	size      int64
}

// FileCache provides thread-safe caching for file contents.
type FileCache struct {
	cache map[string]*CachedFileContent
	mu    sync.RWMutex
}

// CachedDirEntry represents cached directory entries.
type CachedDirEntry struct {
	expiry  time.Time
	entries []os.DirEntry
}

// DirCache provides thread-safe caching for directory listings.
type DirCache struct {
	cache map[string]*CachedDirEntry
	mu    sync.RWMutex
}

// DefaultService is the default implementation of FileSystemService.
type DefaultService struct {
	// DESIGN NOTE: We use simple TTL-based caching without eviction.
	// The 1-second TTL naturally limits memory growth for most use cases.
	// LRU or periodic cleanup can be added if memory becomes a concern.
	pathCache PathCache
	fileCache FileCache
	dirCache  DirCache
}

// NewDefaultService creates a new DefaultFileSystemService.
func NewDefaultService() *DefaultService {
	return &DefaultService{
		pathCache: PathCache{
			cache: make(map[string]*CachedPath),
		},
		fileCache: FileCache{
			cache: make(map[string]*CachedFileContent),
		},
		dirCache: DirCache{
			cache: make(map[string]*CachedDirEntry),
		},
	}
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
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run operation in goroutine
	sentry.SafeGo(func() {
		errCh <- os.MkdirAll(path, 0755)
	})

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

// shouldCachePath determines if a path should be cached based on configured prefixes.
func (s *DefaultService) shouldCachePath(path string) bool {
	// Normalize path to handle edge cases like "/run/service/./x"
	path = filepath.Clean(path)
	for _, prefix := range constants.FilesystemCachePrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

// ReadFile reads a file's contents respecting the context.
func (s *DefaultService) ReadFile(ctx context.Context, path string) ([]byte, error) {
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Cache files based on configured path prefixes
	// These are the primary paths that are read frequently
	if s.shouldCachePath(path) {
		// Check cache with stat-based invalidation
		s.fileCache.mu.RLock()
		cached, exists := s.fileCache.cache[path]
		s.fileCache.mu.RUnlock()

		// DESIGN NOTE: We intentionally stat on EVERY read, not just after TTL expires.
		// This ensures we never serve stale content if files are modified externally.
		// The stat cost is acceptable for correctness and simplicity.
		stat, err := os.Stat(path)
		if err != nil {
			// File doesn't exist or error - invalidate cache and return error
			if exists {
				s.fileCache.mu.Lock()
				delete(s.fileCache.cache, path)
				s.fileCache.mu.Unlock()
			}

			metrics.RecordFilesystemOp("ReadFile", path, false, time.Since(start))

			return nil, err
		}

		// If we have a cached version and file hasn't changed, return it
		if exists && cached.modTime.Equal(stat.ModTime()) && cached.size == stat.Size() {
			// Re-check stat every 1 second for all cached files
			// This provides a good balance between freshness and performance
			if time.Since(cached.lastCheck) < constants.FilesystemCacheRecheckInterval {
				// Cache hit - record as cached operation
				metrics.RecordFilesystemOp("ReadFile", path, true, time.Since(start))

				return cached.content, nil
			}
			// Update last check time
			s.fileCache.mu.Lock()
			// Re-check that the entry still exists after acquiring the lock
			if entry, ok := s.fileCache.cache[path]; ok {
				entry.lastCheck = time.Now()
			}

			s.fileCache.mu.Unlock()
			// Cache hit - record as cached operation
			metrics.RecordFilesystemOp("ReadFile", path, true, time.Since(start))

			return cached.content, nil
		}

		// File changed or not in cache - read it
		content, err := s.readFileUncached(ctx, path)
		if err != nil {
			metrics.RecordFilesystemOp("ReadFile", path, false, time.Since(start))

			return nil, err
		}

		// Update cache
		s.fileCache.mu.Lock()
		s.fileCache.cache[path] = &CachedFileContent{
			content:   content,
			modTime:   stat.ModTime(),
			size:      stat.Size(),
			lastCheck: time.Now(),
		}
		s.fileCache.mu.Unlock()

		metrics.RecordFilesystemOp("ReadFile", path, false, time.Since(start))

		return content, nil
	}

	// Don't cache other paths - use original implementation
	content, err := s.readFileUncached(ctx, path)
	metrics.RecordFilesystemOp("ReadFile", path, false, time.Since(start))

	return content, err
}

// readFileUncached performs the actual file read without caching.
func (s *DefaultService) readFileUncached(ctx context.Context, path string) ([]byte, error) {
	// Create a channel for results
	type result struct {
		err  error
		data []byte
	}

	resCh := make(chan result, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		data, err := os.ReadFile(path)

		resCh <- result{err: err, data: data}
	})
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
	if err := s.checkContext(ctx); err != nil {
		return nil, 0, err
	}

	type result struct {
		err     error
		data    []byte
		newSize int64
	}

	resCh := make(chan result, 1)

	sentry.SafeGo(func() {
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
	})

	select {
	case res := <-resCh:
		return res.data, res.newSize, res.err
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

// WriteFile writes data to a file respecting the context.
func (s *DefaultService) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	start := time.Now()

	defer func() {
		metrics.RecordFilesystemOp("WriteFile", path, false, time.Since(start))
	}()

	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		errCh <- os.WriteFile(path, data, perm)
	})

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}
		// NOTE: We intentionally do NOT invalidate caches here.
		// ReadFile uses os.Stat to detect file changes via modTime/size,
		// so cache invalidation happens automatically on the next read.
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PathExists checks if a path (file or directory) exists.
func (s *DefaultService) PathExists(ctx context.Context, path string) (bool, error) {
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
		return false, err
	}

	// Cache all paths with 1-second TTL for simplicity
	// This provides good balance between freshness and performance

	// Check cache
	s.pathCache.mu.RLock()

	if cached, ok := s.pathCache.cache[path]; ok && time.Now().Before(cached.expiry) {
		s.pathCache.mu.RUnlock()
		// Cache hit - record as cached operation
		metrics.RecordFilesystemOp("PathExists", path, true, time.Since(start))

		return cached.exists, cached.err
	}

	s.pathCache.mu.RUnlock()

	// Not in cache or expired - perform actual check
	exists, err := s.pathExistsUncached(ctx, path)
	// Only cache positive results (when path exists) with 1-second TTL
	// Don't cache negative results as paths might be created immediately after
	if err == nil && exists {
		s.pathCache.mu.Lock()
		s.pathCache.cache[path] = &CachedPath{
			exists: exists,
			err:    err,
			expiry: time.Now().Add(constants.FilesystemCacheTTL),
		}
		s.pathCache.mu.Unlock()
	}

	metrics.RecordFilesystemOp("PathExists", path, false, time.Since(start))

	return exists, err
}

// invalidatePathCache invalidates cache entries for a path and all subpaths.
func (s *DefaultService) invalidatePathCache(path string) {
	// Safely create path with trailing slash, avoiding double slashes
	pathWithSlash := path
	if !strings.HasSuffix(path, "/") {
		pathWithSlash = path + "/"
	}

	// Invalidate path existence cache
	s.pathCache.mu.Lock()
	// Delete exact path match
	delete(s.pathCache.cache, path)
	// Delete any cached paths that are under this path (for RemoveAll)
	for cachedPath := range s.pathCache.cache {
		if strings.HasPrefix(cachedPath, pathWithSlash) {
			delete(s.pathCache.cache, cachedPath)
		}
	}

	s.pathCache.mu.Unlock()

	// Invalidate directory cache
	s.dirCache.mu.Lock()
	// Delete exact path match
	delete(s.dirCache.cache, path)
	// Delete parent directory's cache since its contents changed
	parentDir := filepath.Dir(path)
	delete(s.dirCache.cache, parentDir)
	// Delete any cached directories under this path (for RemoveAll)
	for cachedPath := range s.dirCache.cache {
		if strings.HasPrefix(cachedPath, pathWithSlash) {
			delete(s.dirCache.cache, cachedPath)
		}
	}

	s.dirCache.mu.Unlock()

	// Invalidate file cache for any files under this path
	s.fileCache.mu.Lock()

	for cachedPath := range s.fileCache.cache {
		if cachedPath == path || strings.HasPrefix(cachedPath, pathWithSlash) {
			delete(s.fileCache.cache, cachedPath)
		}
	}

	s.fileCache.mu.Unlock()
}

// pathExistsUncached performs the actual path existence check without caching.
func (s *DefaultService) pathExistsUncached(ctx context.Context, path string) (bool, error) {
	// Create a channel for results
	type result struct {
		err    error
		exists bool
	}

	resCh := make(chan result, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		// Use Stat to follow symlinks and check if the target exists
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			resCh <- result{err: nil, exists: false}

			return
		}

		if err != nil {
			resCh <- result{err: fmt.Errorf("failed to check if path exists (%s): %w", path, err), exists: false}

			return
		}

		resCh <- result{err: nil, exists: true}
	})

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
	if err := s.checkContext(ctx); err != nil {
		return err
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		errCh <- os.Remove(path)
	})

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		// Always invalidate cache, even on error
		// This ensures we don't serve stale data if remove partially succeeded
		s.pathCache.mu.Lock()
		delete(s.pathCache.cache, path)
		s.pathCache.mu.Unlock()
		
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RemoveAll removes a directory and all its contents.
func (s *DefaultService) RemoveAll(ctx context.Context, path string) error {
	if err := s.checkContext(ctx); err != nil {
		return err
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		errCh <- os.RemoveAll(path)
	})

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		// Always invalidate cache entries, even on error
		// We don't know what state the filesystem is in after a failed RemoveAll
		s.invalidatePathCache(path)
		
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
	start := time.Now()

	defer func() {
		metrics.RecordFilesystemOp("Stat", path, false, time.Since(start))
	}()

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
	sentry.SafeGo(func() {
		info, err := os.Stat(path)
		resCh <- result{info, err}
	})

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
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		errCh <- os.Chmod(path, mode)
	})

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
	start := time.Now()

	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Check cache first
	s.dirCache.mu.RLock()

	if cached, ok := s.dirCache.cache[path]; ok && time.Now().Before(cached.expiry) {
		s.dirCache.mu.RUnlock()
		// Cache hit - record as cached operation
		metrics.RecordFilesystemOp("ReadDir", path, true, time.Since(start))

		return cached.entries, nil
	}

	s.dirCache.mu.RUnlock()

	// Not in cache or expired - perform actual read
	entries, err := s.readDirUncached(ctx, path)
	if err != nil {
		metrics.RecordFilesystemOp("ReadDir", path, false, time.Since(start))

		return nil, err
	}

	// Cache the result with 1-second TTL
	s.dirCache.mu.Lock()
	s.dirCache.cache[path] = &CachedDirEntry{
		entries: entries,
		expiry:  time.Now().Add(constants.FilesystemCacheTTL),
	}
	s.dirCache.mu.Unlock()

	metrics.RecordFilesystemOp("ReadDir", path, false, time.Since(start))

	return entries, nil
}

// readDirUncached performs the actual directory read without caching.
func (s *DefaultService) readDirUncached(ctx context.Context, path string) ([]os.DirEntry, error) {
	// Create a channel for results
	type result struct {
		err     error
		entries []os.DirEntry
	}

	resCh := make(chan result, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		entries, err := os.ReadDir(path)
		resCh <- result{err: err, entries: entries}
	})

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
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		// Use chown command as os.Chown needs numeric user/group IDs
		cmd := exec.Command("chown", fmt.Sprintf("%s:%s", user, group), path)
		errCh <- cmd.Run()
	})

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

	sentry.SafeGo(func() {
		matches, err := filepath.Glob(pattern)
		resCh <- result{err: err, matches: matches}
	})

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
	sentry.SafeGo(func() {
		errCh <- os.Rename(oldPath, newPath)
	})

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to rename file %s to %s: %w", oldPath, newPath, err)
		}

		// Invalidate cache for both old and new paths
		s.invalidatePathCache(oldPath)
		s.invalidatePathCache(newPath)

		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Symlink creates a symbolic link from linkPath to target.
func (s *DefaultService) Symlink(ctx context.Context, target, linkPath string) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	sentry.SafeGo(func() {
		errCh <- os.Symlink(target, linkPath)
	})

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to create symlink %s -> %s: %w", linkPath, target, err)
		}

		// Invalidate cache for the link path since we just created it
		// This ensures PathExists will check the actual filesystem
		s.pathCache.mu.Lock()
		delete(s.pathCache.cache, linkPath)
		s.pathCache.mu.Unlock()

		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
