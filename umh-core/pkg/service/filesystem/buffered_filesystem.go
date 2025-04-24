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
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// Package filesystem implements various filesystem service interfaces.
//
// BufferedFilesystem (BufferedService) is an experimental implementation intended
// to reduce redundant I/O operations by caching filesystem reads and writes,
// especially in scenarios like S6 log retrieval and configuration file access.
//
// How It Works:
// - The buffered filesystem wraps a "real" filesystem service (e.g., DefaultService)
//   and caches file operations for specified directories (e.g., constants.S6BaseDir,
//   constants.S6LogBaseDir). This allows repeated file reads to be served from memory,
//   reducing disk I/O.
// - It is designed to support parallelized file operations by preloading directories
//   and filtering out files that should be ignored (e.g., archived logs prefixed with "@40000000").
//
// How to Enable:
// Instead of initializing the filesystem service as follows:
//
//     // Use the default, unbuffered filesystem service
//     filesystemService := filesystem.NewDefaultService()
//
// you can opt-in for the buffered service:
//
//     // Create a buffered filesystem service that caches reads for S6 directories
//     filesystemService := filesystem.NewBufferedServiceWithDirs(
//         filesystem.NewDefaultService(),
//         []string{constants.S6BaseDir, constants.S6LogBaseDir},
//         constants.FilesAndDirectoriesToIgnore,
//     )
//
// Limitations and Known Issues:
// 1. Increased Complexity:
//    - The buffering and parallelization mechanisms add significant code complexity,
//      making maintenance and debugging more challenging.
// 2. Performance Impact:
//    - Initial performance tests showed higher latencies (e.g., 0.5 quantile: ~49ms,
//      0.99 quantile: ~88ms) compared to the default implementation
//      (e.g., 0.5 quantile: ~19ms, 0.99 quantile: ~34ms).
// 3. Synchronization and Edge Cases:
//    - Potential mismatches between the "virtual" buffered view and the "real" filesystem,
//      particularly when files are modified concurrently (e.g., changes by the supervisor)
//      or when file permissions change.
// 4. Log Reading Bottleneck:
//    - The primary performance bottleneck remains in reading and processing large log files.
//      Implementing proper log rotation and incremental reading (e.g., using FIFO buffers)
//      would further complicate the design.
//
// Experimental Status:
// This implementation is experimental and subject to change. It is recommended to
// use the default unbuffered filesystem service in production until the buffering
// strategy is refined and its benefits are clearly demonstrated.

// CachedFile represents a file or directory in the filesystem
type CachedFile struct {
	Info  os.FileInfo
	IsDir bool
}

// DirectoryCache represents a snapshot of a directory tree
type DirectoryCache struct {
	Files map[string]*CachedFile // relative path -> file info
}

// ReadDirectoryTree reads a directory tree from disk and returns a cache of its contents
func ReadDirectoryTree(ctx context.Context, service Service, root string) (*DirectoryCache, error) {

	dc := &DirectoryCache{
		Files: make(map[string]*CachedFile),
	}

	// First check if root exists
	rootInfo, err := service.Stat(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory tree: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("failed to walk directory tree: root is not a directory")
	}

	// Start with root directory
	entries, err := service.ReadDir(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory tree: %w", err)
	}

	// Process each entry recursively
	for _, entry := range entries {
		fullPath := filepath.Join(root, entry.Name())
		relPath, err := filepath.Rel(root, fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get relative path: %w", err)
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("failed to get file info: %w", err)
		}

		// Skip root directory
		if relPath == "." {
			continue
		}

		isDir := info.IsDir()

		// Check if this is a symlink
		if info.Mode()&os.ModeSymlink != 0 {
			// Get the target of the symlink
			target, err := os.Readlink(fullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read symlink %s: %w", fullPath, err)
			}

			// If the target is not absolute, make it absolute relative to the directory containing the symlink
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(fullPath), target)
			}

			// Check if the target exists
			targetInfo, err := os.Stat(target)
			if err != nil {
				return nil, fmt.Errorf("failed to stat symlink target %s: %w", target, err)
			}

			// Update isDir based on the symlink's target
			isDir = targetInfo.IsDir()
		}

		dc.Files[relPath] = &CachedFile{
			Info:  info,
			IsDir: isDir,
		}

		// If it's a directory (or symlink to directory), recurse
		if isDir {
			subDc, err := ReadDirectoryTree(ctx, service, fullPath)
			if err != nil {
				return nil, err
			}
			// Add all files from subdirectory
			for subPath, subFile := range subDc.Files {
				dc.Files[filepath.Join(relPath, subPath)] = subFile
			}
		}
	}

	return dc, nil
}

// BufferedService implements the Service interface in a "buffered" fashion.
//   - On SyncFromDisk(), it reads the real filesystem into memory (metadata and selected contents).
//   - On Read/Stat calls, it only returns data from the in-memory cache. If not present, returns os.ErrNotExist.
//   - On Write/Remove calls, it marks the file as changed in memory and defers actual disk writes until SyncToDisk().
//   - On SyncToDisk(), it writes only changed files to disk, skipping those that have changed on disk externally.
type BufferedService struct {
	// base is the underlying filesystem service that actually performs disk I/O.
	base Service

	// mu guards all maps below
	mu sync.Mutex

	// files stores the "current in-memory snapshot."
	// Key: absolute path on disk.
	// Value: fileState describing content, modTime when we read it, etc.
	files map[string]fileState

	// changed tracks which files the reconcile loop has modified in memory.
	// If a file is in changed[], we plan to write it out in SyncToDisk(). If removed==true, we plan to remove it.
	changed map[string]fileChange

	// syncDirs is a list of directories that should be synced from disk.
	syncDirs []string

	// maxFileSize is a threshold for reading big logs. If a file is bigger, we skip it. It should not happen as logs have a 1MB limit.
	maxFileSize int64

	// pathsToIgnore is a list of paths to skip during sync
	pathsToIgnore []string

	// verifyPermissions determines if we should check permission information
	verifyPermissions bool

	// currentUser caches the current user info for permission checks
	currentUser *user.User

	// fileReadWorkers is the number of workers for parallel file reading
	// If not set (zero), it will be calculated based on CPU count
	fileReadWorkers int

	// slowReadThreshold is the duration threshold to log slow file reads
	slowReadThreshold time.Duration

	// appendOnlyDirs contains directories where files are append-only (like logs)
	// For these, we'll use incremental reading instead of full re-reads
	appendOnlyDirs []string
}

// fileState holds in-memory data and metadata for a single file or directory
type fileState struct {
	isDir    bool
	content  []byte // might be empty if we skipped reading (e.g. large file)
	modTime  time.Time
	fileMode os.FileMode
	size     int64
	uid      int    // cached user id of owner
	gid      int    // cached group id of owner
	lastSize int64  // tracks last known size for incremental reading
	inode    uint64 // tracks inode number to detect file replacement
}

// fileChange represents a pending user-level change: either an updated content or a removal
type fileChange struct {
	content []byte
	perm    os.FileMode
	removed bool
	wasDir  bool // tracks if this was a directory before removal
}

// Check interface conformance at compile time
var _ Service = (*BufferedService)(nil)

// NewBufferedService creates a buffered service that wraps an existing filesystem service.
// The existing API for backward compatibility - takes a single directory.
func NewBufferedService(base Service, rootDir string, pathsToIgnore []string) *BufferedService {
	return NewBufferedServiceWithDirs(base, []string{rootDir}, pathsToIgnore)
}

// NewBufferedServiceWithDirs creates a buffered service that wraps an existing filesystem service.
// syncDirectories is a list of directories that will be read during SyncFromDisk().
func NewBufferedServiceWithDirs(base Service, syncDirectories []string, pathsToIgnore []string) *BufferedService {
	currentUser, err := user.Current()
	if err != nil {
		// Log the error but continue - we'll use mode bits only for permissions
		logger := logger.For(logger.ComponentFilesystem)
		logger.Warnf("Failed to get current user info: %v", err)
	}

	// Calculate default worker count
	workerCount := runtime.NumCPU() * constants.FilesystemWorkerMultiplier
	if workerCount < constants.FilesystemMinWorkers {
		workerCount = constants.FilesystemMinWorkers
	}
	if workerCount > constants.FilesystemMaxWorkers {
		workerCount = constants.FilesystemMaxWorkers
	}

	return &BufferedService{
		base:              base,
		files:             make(map[string]fileState),
		changed:           make(map[string]fileChange),
		syncDirs:          syncDirectories,
		maxFileSize:       10 * 1024 * 1024, // 10 MB default threshold for demonstration
		pathsToIgnore:     pathsToIgnore,
		verifyPermissions: true, // Default to true for safety
		currentUser:       currentUser,
		fileReadWorkers:   workerCount,
		slowReadThreshold: constants.FilesystemSlowReadThreshold,
		// By default, no append-only directories
		appendOnlyDirs: []string{},
	}
}

// NewBufferedServiceWithDefaultAppendDirs creates a buffered service with predefined append-only directories
// that will use incremental reading for logs or other append-only files.
func NewBufferedServiceWithDefaultAppendDirs(base Service, syncDirectories []string, pathsToIgnore []string) *BufferedService {
	bs := NewBufferedServiceWithDirs(base, syncDirectories, pathsToIgnore)

	// By default, consider log directories as append-only
	bs.appendOnlyDirs = []string{"/data/logs"}

	return bs
}

// Chown changes the owner and group ids of the named file.
func (bs *BufferedService) Chown(ctx context.Context, path string, username string, groupname string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check if file exists
	st, ok := bs.files[path]
	if !ok {
		return os.ErrNotExist
	}

	// Convert username to uid
	var uid int
	if username != "" {
		// Note: This is a call outside of the buffered service
		// We could later cache this information
		u, err := user.Lookup(username)
		if err != nil {
			return fmt.Errorf("failed to lookup user %s: %w", username, err)
		}
		uid, _ = strconv.Atoi(u.Uid)
	} else {
		uid = st.uid // Keep existing
	}

	// Convert groupname to gid
	var gid int
	if groupname != "" {
		// Note: This is a call outside of the buffered service
		// We could later cache this information
		g, err := user.LookupGroup(groupname)
		if err != nil {
			return fmt.Errorf("failed to lookup group %s: %w", groupname, err)
		}
		gid, _ = strconv.Atoi(g.Gid)
	} else {
		gid = st.gid // Keep existing
	}

	// Check permissions - only root can change ownership
	if bs.verifyPermissions && bs.currentUser != nil {
		currentUid, _ := strconv.Atoi(bs.currentUser.Uid)
		if currentUid != 0 {
			return fmt.Errorf("insufficient permissions to change ownership: not root")
		}
	}

	// Update file state in memory
	st.uid = uid
	st.gid = gid
	bs.files[path] = st

	// Mark as changed for SyncToDisk
	chg, inChg := bs.changed[path]
	if !inChg {
		chg = fileChange{}
	}
	// We don't modify chg.content or chg.perm here, just mark it as changed
	bs.changed[path] = chg

	return nil
}

// SetVerifyPermissions configures whether permission checks should be performed
func (bs *BufferedService) SetVerifyPermissions(verify bool) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.verifyPermissions = verify
}

// SetFileReadWorkers configures the number of worker goroutines for file reading
func (bs *BufferedService) SetFileReadWorkers(count int) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if count <= 0 {
		// Calculate default based on CPU count
		count = runtime.NumCPU() * constants.FilesystemWorkerMultiplier
		if count < constants.FilesystemMinWorkers {
			count = constants.FilesystemMinWorkers
		}
		if count > constants.FilesystemMaxWorkers {
			count = constants.FilesystemMaxWorkers
		}
	}
	bs.fileReadWorkers = count
}

// SetSlowReadThreshold configures the threshold for logging slow file reads
func (bs *BufferedService) SetSlowReadThreshold(threshold time.Duration) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.slowReadThreshold = threshold
}

// SetAppendOnlyDirs configures directories that contain append-only files (like logs)
// Files in these directories will be read incrementally (only new content)
func (bs *BufferedService) SetAppendOnlyDirs(dirs []string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.appendOnlyDirs = dirs
}

// isInAppendOnlyDir checks if a path is in one of the append-only directories
func (bs *BufferedService) isInAppendOnlyDir(path string) bool {
	for _, dir := range bs.appendOnlyDirs {
		if strings.HasPrefix(path, dir) {
			return true
		}
	}
	return false
}

// getFileState gets the cached file state, or returns nil if not found
func (bs *BufferedService) getFileState(path string) *fileState {
	state, ok := bs.files[path]
	if !ok {
		return nil
	}
	return &state
}

// canWriteBasedOnMode checks if the current user can write to a file based on its mode
func (bs *BufferedService) canWriteBasedOnMode(state *fileState) bool {
	if state == nil {
		return false
	}

	// If we're root, we can always write
	if bs.currentUser != nil && bs.currentUser.Uid == "0" {
		return true
	}

	// Check if the file is writable by the owner, group, or others
	mode := state.fileMode

	// If we have uid/gid info and current user info
	if bs.currentUser != nil {
		uid, _ := strconv.Atoi(bs.currentUser.Uid)
		gid, _ := strconv.Atoi(bs.currentUser.Gid)

		// If we're the owner
		if uid == state.uid {
			return mode&0200 != 0 // Owner write permission
		}

		// Check if we're in the same group
		if gid == state.gid {
			return mode&0020 != 0 // Group write permission
		}
	}

	// Check other permission
	return mode&0002 != 0 // Others write permission
}

// checkWritePermission verifies if the current process should have write permission for a path
// based on cached file information
func (bs *BufferedService) checkWritePermission(path string) error {
	if !bs.verifyPermissions {
		return nil
	}

	// Get the file state from our cache
	state := bs.getFileState(path)

	// If the file doesn't exist in our cache, check if parent directory is writable
	if state == nil {
		parentDir := filepath.Dir(path)
		return bs.checkDirectoryWritePermission(parentDir)
	}

	// Check if we can write to it based on mode
	if !bs.canWriteBasedOnMode(state) {
		return fmt.Errorf("insufficient permissions to write to %s", path)
	}

	return nil
}

// checkDirectoryWritePermission verifies if we can write to a directory
// based on cached directory information
func (bs *BufferedService) checkDirectoryWritePermission(dir string) error {
	if !bs.verifyPermissions {
		return nil
	}

	// Special case: if dir is empty or ".", check current working directory
	if dir == "" || dir == "." {
		dir = "."
	}

	// Get the directory state from our cache
	state := bs.getFileState(dir)

	// If directory not in cache, return an error
	if state == nil {
		return fmt.Errorf("directory not found in cache: %s", dir)
	}

	// Check if it's actually a directory
	if !state.isDir {
		return fmt.Errorf("path is not a directory: %s", dir)
	}

	// Check if we can write to it based on mode
	if !bs.canWriteBasedOnMode(state) {
		return fmt.Errorf("insufficient permissions to write to directory %s", dir)
	}

	return nil
}

// shouldIgnorePath checks if a path should be ignored during sync
func (bs *BufferedService) shouldIgnorePath(path string) bool {
	for _, ignore := range bs.pathsToIgnore {
		if strings.Contains(path, ignore) {
			return true
		}
	}
	return false
}

// =========================
// SyncFromDisk / SyncToDisk
// =========================

// SyncFromDisk loads the filesystem state into memory, ignoring anything we had before.
// It will read file contents unless they exceed maxFileSize, in which case content is blank.
// TODO: make this function better readable
func (bs *BufferedService) SyncFromDisk(ctx context.Context) error {
	logger := logger.For(logger.ComponentFilesystem)
	startTime := time.Now()
	logger.Debugf("SyncFromDisk started for %d directories", len(bs.syncDirs))
	defer func() {
		logger.Debugf("SyncFromDisk took %dms", time.Since(startTime).Milliseconds())
	}()

	// Create a new map to hold synced files
	newFiles := make(map[string]fileState)
	var filesMutex sync.Mutex

	// Copy existing state for incremental reads
	existingFiles := make(map[string]fileState)
	bs.mu.Lock()
	for k, v := range bs.files {
		existingFiles[k] = v
	}
	bs.mu.Unlock()

	// Sync each directory in the list
	// TODO: "This code could probably refactored to use some of the initial scan code"
	for _, dir := range bs.syncDirs {
		// Check if directory exists
		info, err := bs.base.Stat(ctx, dir)
		if err != nil {
			if os.IsNotExist(err) {
				logger.Debugf("Directory doesn't exist, skipping: %s", dir)
				continue // Skip directories that don't exist
			}
			return fmt.Errorf("failed to stat directory %s: %w", dir, err)
		}

		if !info.IsDir() {
			return fmt.Errorf("path is not a directory: %s", dir)
		}

		// Get owner and group info for the directory
		uid, gid := getFileOwner(dir)

		// Add directory itself
		filesMutex.Lock()
		newFiles[dir] = fileState{
			isDir:    true,
			content:  nil,
			modTime:  info.ModTime(),
			fileMode: info.Mode(),
			size:     0,
			uid:      uid,
			gid:      gid,
		}
		filesMutex.Unlock()

		// Read the directory tree
		dc, err := ReadDirectoryTree(ctx, bs.base, dir)
		if err != nil {
			return fmt.Errorf("failed to read directory tree for %s: %w", dir, err)
		}

		// Process directories first (they're lightweight and don't need parallelization)
		for path, cf := range dc.Files {
			if !cf.IsDir {
				continue
			}

			absPath := filepath.Join(dir, path)
			if bs.shouldIgnorePath(absPath) {
				continue
			}

			uid, gid := getFileOwner(absPath)
			filesMutex.Lock()
			newFiles[absPath] = fileState{
				isDir:    true,
				content:  nil,
				modTime:  cf.Info.ModTime(),
				fileMode: cf.Info.Mode(),
				size:     0,
				uid:      uid,
				gid:      gid,
			}
			filesMutex.Unlock()
		}

		// Setup worker pool for file content reading
		// Define job and result types
		type fileReadJob struct {
			absPath string
			cf      *CachedFile
		}

		type fileReadResult struct {
			absPath string
			state   *fileState
			err     error
		}

		// Count files to properly size channels
		fileCount := countFiles(dc.Files)
		if fileCount == 0 {
			// No files to process in this directory
			continue
		}

		// Create channels
		jobs := make(chan fileReadJob, fileCount)
		results := make(chan fileReadResult, fileCount)
		errChan := make(chan error, 1)
		var wg sync.WaitGroup

		// Determine worker count
		workerCount := bs.fileReadWorkers
		if workerCount <= 0 {
			// Fallback in case it wasn't initialized properly
			workerCount = runtime.NumCPU() * 2
		}
		// Create cancellable context for workers
		workerCtx, cancel := context.WithCancel(ctx)
		defer cancel() // Ensure we cancel workers on exit

		// Start workers
		for w := 0; w < workerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for job := range jobs {
					// Check for cancellation
					select {
					case <-workerCtx.Done():
						return
					default:
						// Continue processing
					}

					// Skip directories (already processed)
					if job.cf.IsDir {
						continue
					}

					// Get file info
					uid, gid := getFileOwner(job.absPath)

					// Get inode number to detect file rotation
					fileInode := getFileInode(job.absPath)

					// Skip large files
					if job.cf.Info.Size() > bs.maxFileSize {
						results <- fileReadResult{
							absPath: job.absPath,
							state: &fileState{
								isDir:    false,
								content:  nil, // Skip content for large files
								modTime:  job.cf.Info.ModTime(),
								fileMode: job.cf.Info.Mode(),
								size:     job.cf.Info.Size(),
								uid:      uid,
								gid:      gid,
								inode:    fileInode,
							},
						}
						continue
					}

					// Check if this is an append-only file that we've seen before
					isIncremental := false
					var previousContent []byte
					var previousSize int64
					var previousInode uint64

					if bs.isInAppendOnlyDir(job.absPath) {
						if prevState, exists := existingFiles[job.absPath]; exists && !prevState.isDir {
							previousContent = prevState.content
							previousSize = prevState.size
							previousInode = prevState.inode

							// Only do incremental if:
							// 1. File has same inode (it's the same file)
							// 2. Current size is >= previous size (no log rotation occurred)
							if previousInode == fileInode && job.cf.Info.Size() >= previousSize {
								isIncremental = true
							} else if previousInode != fileInode || job.cf.Info.Size() < previousSize {
								// Log rotation detected - file replaced or truncated
								logger.Debugf("Log rotation detected for %s (inode: %d->%d, size: %d->%d)",
									job.absPath, previousInode, fileInode, previousSize, job.cf.Info.Size())
								isIncremental = false
							}
						}
					}

					var data []byte
					var err error
					readStart := time.Now()

					if isIncremental && job.cf.Info.Size() > previousSize {
						// Incremental read - only read the new bytes
						data, err = bs.readFileIncrementally(job.absPath, previousSize, previousContent, logger)
						if err != nil {
							results <- fileReadResult{
								absPath: job.absPath,
								err:     err,
							}
							continue
						}
						logger.Debugf("Incremental read: %s (%d bytes new)", job.absPath, len(data)-len(previousContent))
					} else {
						// Normal full read (either first time seeing file or log rotation happened)
						data, err = bs.base.ReadFile(workerCtx, job.absPath)
						if isIncremental {
							logger.Debugf("Full read after log rotation: %s", job.absPath)
						}
					}

					readDuration := time.Since(readStart)

					// Log slow reads
					if readDuration > bs.slowReadThreshold {
						logger.Debugf("Slow file read: %s took %v", job.absPath, readDuration)
					}

					// Handle errors
					if err != nil {
						if os.IsNotExist(err) {
							// File disappeared between directory listing and reading
							results <- fileReadResult{
								absPath: job.absPath,
								err:     os.ErrNotExist,
							}
						} else {
							// Fatal error - propagate it
							select {
							case errChan <- fmt.Errorf("failed to read file %s: %w", job.absPath, err):
								cancel() // Signal other workers to stop
							default:
								// Another error already sent
							}
						}
						continue
					}

					// Success - send result
					results <- fileReadResult{
						absPath: job.absPath,
						state: &fileState{
							isDir:    false,
							content:  data,
							modTime:  job.cf.Info.ModTime(),
							fileMode: job.cf.Info.Mode(),
							size:     job.cf.Info.Size(),
							uid:      uid,
							gid:      gid,
							lastSize: job.cf.Info.Size(), // Track current size for future incremental reads
							inode:    fileInode,          // Track inode for log rotation detection
						},
					}
				}
			}()
		}

		// Start a result collector goroutine
		var resultErr error
		resultDone := make(chan struct{})

		go func() {
			defer close(resultDone)

			// Process results as they arrive
			for result := range results {
				if result.err != nil {
					if errors.Is(result.err, os.ErrNotExist) {
						logger.Debugf("File does not exist, skipping: %s", result.absPath)
						continue
					}

					// Set error and exit
					resultErr = result.err
					cancel() // Signal workers to stop
					return
				}

				// Store result
				filesMutex.Lock()
				newFiles[result.absPath] = *result.state
				filesMutex.Unlock()
			}
		}()

		// Queue all file jobs
		for path, cf := range dc.Files {
			if cf.IsDir {
				continue // Skip directories, already processed
			}

			absPath := filepath.Join(dir, path)
			if bs.shouldIgnorePath(absPath) {
				continue
			}

			select {
			case jobs <- fileReadJob{absPath: absPath, cf: cf}:
				// Job queued successfully
			case <-ctx.Done():
				cancel()
				return ctx.Err()
			}
		}

		// Close jobs channel to signal no more work
		close(jobs)

		// Wait for workers to finish
		wg.Wait()

		// Close results channel
		close(results)

		// Wait for result processor to finish
		<-resultDone

		// Check for errors from workers
		select {
		case err := <-errChan:
			return err
		default:
			// No worker errors
		}

		// Check for errors from result processor
		if resultErr != nil {
			return resultErr
		}
	}

	// Replace the in-memory state atomically
	logger.Debugf("SyncFromDisk: loaded %d files", len(newFiles))
	bs.mu.Lock()
	bs.files = newFiles
	// Clear any pending changes since we are reloading from disk
	clear(bs.changed)
	bs.mu.Unlock()

	logger.Debugf("SyncFromDisk: done")
	return nil
}

// SyncToDisk flushes all changed files to disk, and removes any marked for removal.
func (bs *BufferedService) SyncToDisk(ctx context.Context) error {
	start := time.Now()
	logger := logger.For(logger.ComponentFilesystem)
	logger.Debugf("SyncToDisk started")
	defer func() {
		logger.Debugf("SyncToDisk took %dms", time.Since(start).Milliseconds())
	}()

	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check permissions before performing operations
	if bs.verifyPermissions {
		permissionErrors := make(map[string]error)

		// Check all operations for permission issues
		for path, chg := range bs.changed {
			if !chg.removed {
				// Check write permission for files/directories we're creating or modifying
				if err := bs.checkWritePermission(path); err != nil {
					permissionErrors[path] = err
				}
			}
			// Skip permission checks for items marked for removal
			// We already checked permissions during the Remove/RemoveAll operation
			// And parent directories might have already been removed from the cache
		}

		// If any permission errors, report them
		if len(permissionErrors) > 0 {
			errStrs := make([]string, 0, len(permissionErrors))
			for path, err := range permissionErrors {
				errStrs = append(errStrs, fmt.Sprintf("%s: %v", path, err))
			}
			return fmt.Errorf("permission checks failed for %d paths: %s",
				len(permissionErrors), strings.Join(errStrs, "; "))
		}
	}

	// Process in the following order:
	// 1. Create directories (shallowest first)
	// 2. Write files (ensuring parent directories exist)
	// 3. Remove files (deepest first)
	// 4. Remove directories (deepest first)

	// Step 1: Create directories (shallowest first)
	var dirsToCreate []string
	for path, chg := range bs.changed {
		if _, exists := bs.changed[path]; exists && !chg.removed {
			state, exists := bs.files[path]
			if !exists {
				return fmt.Errorf("file not found in memory: %s", path)
			}
			if state.isDir {
				dirsToCreate = append(dirsToCreate, path)
			}
		}
	}
	// Sort paths by length in ascending order to create shallowest paths first
	sort.Slice(dirsToCreate, func(i, j int) bool {
		return len(dirsToCreate[i]) < len(dirsToCreate[j])
	})

	for _, path := range dirsToCreate {
		if err := bs.base.EnsureDirectory(ctx, path); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		delete(bs.changed, path)
	}

	// Step 2: Write files (ensuring parent directories exist)
	var filesToWrite []string
	for path, chg := range bs.changed {
		if _, exists := bs.changed[path]; exists && !chg.removed {
			state, exists := bs.files[path]
			if !exists {
				return fmt.Errorf("file not found in memory: %s", path)
			}
			if !state.isDir {
				filesToWrite = append(filesToWrite, path)
			}
		}
	}

	for _, path := range filesToWrite {
		// Ensure parent directory exists
		parentDir := filepath.Dir(path)
		if err := bs.base.EnsureDirectory(ctx, parentDir); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Now write the file
		chg, exists := bs.changed[path]
		if !exists {
			return fmt.Errorf("change record missing for path: %s", path)
		}
		if err := bs.base.WriteFile(ctx, path, chg.content, chg.perm); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		delete(bs.changed, path)
	}

	// Step 3 & 4: Handle removals in reverse order (deepest paths first)
	var filesToRemove []string
	var dirsToRemove []string
	for path, chg := range bs.changed {
		if _, exists := bs.changed[path]; exists && chg.removed {
			if chg.wasDir {
				dirsToRemove = append(dirsToRemove, path)
			} else {
				filesToRemove = append(filesToRemove, path)
			}
		}
	}

	// Sort paths by length in descending order
	sort.Slice(filesToRemove, func(i, j int) bool {
		return len(filesToRemove[i]) > len(filesToRemove[j])
	})
	sort.Slice(dirsToRemove, func(i, j int) bool {
		return len(dirsToRemove[i]) > len(dirsToRemove[j])
	})

	// First remove files
	for _, path := range filesToRemove {
		if err := bs.base.Remove(ctx, path); err != nil {
			return fmt.Errorf("failed to remove file: %w", err)
		}
		delete(bs.files, path)
		delete(bs.changed, path)
	}

	// Then remove directories
	for _, path := range dirsToRemove {
		if err := bs.base.RemoveAll(ctx, path); err != nil {
			return fmt.Errorf("failed to remove directory: %w", err)
		}
		delete(bs.files, path)
		delete(bs.changed, path)
	}

	return nil
}

// =========================
// Implementation of Service
// =========================

// EnsureDirectory creates the directory in-memory and marks it for creation on disk.
func (bs *BufferedService) EnsureDirectory(ctx context.Context, path string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check permissions for the parent directory
	if bs.verifyPermissions {
		parentDir := filepath.Dir(path)
		if err := bs.checkDirectoryWritePermission(parentDir); err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
	}

	// If already exists, check if it is a directory.
	if state, ok := bs.files[path]; ok {
		if !state.isDir {
			return fmt.Errorf("%s exists and is not a directory", path)
		}
		return nil
	}

	// Create new directory entry.
	bs.files[path] = fileState{
		isDir:    true,
		modTime:  time.Now(),
		fileMode: os.ModeDir | 0755,
		size:     0,
	}
	// Mark as changed so that SyncToDisk can create it on disk.
	bs.changed[path] = fileChange{
		removed: false,
		perm:    os.ModeDir | 0755,
	}
	return nil
}

// ReadFile returns the content from the in-memory cache only.
// If not present, returns os.ErrNotExist.
func (bs *BufferedService) ReadFile(ctx context.Context, path string) ([]byte, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	st, ok := bs.files[path]
	if !ok || st.isDir {
		return nil, os.ErrNotExist
	}
	// If it's a large file (content is nil but size is set), return not exist
	// TODO: "Would be nice to have a distinct error here."
	if st.content == nil && st.size > bs.maxFileSize {
		return nil, os.ErrNotExist
	}
	// Return empty slice instead of nil if content is nil
	if st.content == nil {
		return []byte{}, nil
	}
	return st.content, nil
}

// WriteFile does not immediately write to disk; it marks the file as changed in memory.
func (bs *BufferedService) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check permissions
	if bs.verifyPermissions {
		// If file exists, check if we can write to it
		if state, exists := bs.files[path]; exists {
			if !bs.canWriteBasedOnMode(&state) {
				return fmt.Errorf("insufficient permissions to write to file: %s", path)
			}
		} else {
			// If file doesn't exist, check parent directory permissions
			parentDir := filepath.Dir(path)
			if err := bs.checkDirectoryWritePermission(parentDir); err != nil {
				return fmt.Errorf("permission check failed: %w", err)
			}
		}
	}

	// If there's an existing fileState, update it so subsequent ReadFile sees new content
	st, exists := bs.files[path]
	if exists {
		st.isDir = false
		st.content = data
		st.fileMode = perm
		st.size = int64(len(data))
		// Update the map with the modified state
		bs.files[path] = st
	} else {
		// Create a new file state
		bs.files[path] = fileState{
			isDir:    false,
			content:  data,
			modTime:  time.Now(),
			fileMode: perm,
			size:     int64(len(data)),
		}
	}

	// Mark changed
	bs.changed[path] = fileChange{
		content: data,
		perm:    perm,
		removed: false,
	}
	return nil
}

// PathExists checks if a path (file or directory) exists in the in-memory map
func (bs *BufferedService) PathExists(ctx context.Context, path string) (bool, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	_, ok := bs.files[path]
	if !ok {
		return false, nil
	}
	// If it's in changed as removed, treat it as not existing
	chg, inChanged := bs.changed[path]
	if inChanged && chg.removed {
		return false, nil
	}
	// We have the file entry and it's not marked for removal, so it exists
	return true, nil
}

// FileExists checks the in-memory map only.
// Deprecated: use PathExists instead
func (bs *BufferedService) FileExists(ctx context.Context, path string) (bool, error) {
	return bs.PathExists(ctx, path)
}

// Remove marks the file for removal in memory.
func (bs *BufferedService) Remove(ctx context.Context, path string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check permissions
	if bs.verifyPermissions {
		// For removal, need write permission on parent directory
		parentDir := filepath.Dir(path)
		if err := bs.checkDirectoryWritePermission(parentDir); err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
	}

	// If we have a fileState, check if it's a directory
	if st, ok := bs.files[path]; ok {
		if st.isDir {
			return fmt.Errorf("cannot remove: is directory")
		}
		// Delete from files map immediately
		delete(bs.files, path)
	}

	bs.changed[path] = fileChange{
		removed: true,
	}
	return nil
}

// RemoveAll recursively marks all files and directories under the given path as removed in-memory.
func (bs *BufferedService) RemoveAll(ctx context.Context, path string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check permissions
	if bs.verifyPermissions {
		// For removal, need write permission on parent directory
		parentDir := filepath.Dir(path)
		if err := bs.checkDirectoryWritePermission(parentDir); err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
	}

	// First check if the target itself is a directory
	targetIsDir := false
	if st, ok := bs.files[path]; ok {
		targetIsDir = st.isDir
	}

	// Mark the target and all its children for removal
	for key := range bs.files {
		if key == path || hasPrefix(key, path+string(os.PathSeparator)) {
			state, ok := bs.files[key]
			if !ok {
				continue
			}
			bs.changed[key] = fileChange{removed: true, wasDir: state.isDir}
			// Also mark it as removed in the files map
			delete(bs.files, key)
		}
	}

	// Also mark the target itself for removal even if it wasn't in our files map
	bs.changed[path] = fileChange{removed: true, wasDir: targetIsDir}
	return nil
}

// Stat returns an os.FileInfo-like object if it is in the in-memory map. Otherwise os.ErrNotExist.
func (bs *BufferedService) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	st, ok := bs.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	if st.isDir {
		// Return a synthetic fileInfo for a directory
		return &memFileInfo{
			name:  filepathBase(path),
			size:  0,
			mode:  st.fileMode,
			mtime: st.modTime,
			dir:   true,
		}, nil
	}
	return &memFileInfo{
		name:  filepathBase(path),
		size:  st.size,
		mode:  st.fileMode,
		mtime: st.modTime,
		dir:   false,
	}, nil
}

// CreateFile creates a file in-memory and marks it for creation on disk during SyncToDisk.
// It returns nil, nil because the actual file doesn't exist on disk yet.
func (bs *BufferedService) CreateFile(ctx context.Context, path string, perm os.FileMode) (*os.File, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check permissions
	if bs.verifyPermissions {
		parentDir := filepath.Dir(path)
		if err := bs.checkDirectoryWritePermission(parentDir); err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
	}

	// If already exists, check if it is a file
	if state, ok := bs.files[path]; ok {
		if state.isDir {
			return nil, fmt.Errorf("%s exists and is not a file", path)
		}
		// It exists and is a file, so we're good
		return nil, nil
	}

	// Create new file entry in memory
	bs.files[path] = fileState{
		isDir:    false,
		content:  []byte{},
		modTime:  time.Now(),
		fileMode: perm,
		size:     0,
	}

	// Mark as changed so that SyncToDisk will create it on disk
	bs.changed[path] = fileChange{
		content: []byte{},
		perm:    perm,
		removed: false,
	}

	// Return nil, nil since the file isn't actually on disk yet
	return nil, nil
}

// Chmod updates the fileMode in memory (and later flush).
func (bs *BufferedService) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check permissions - we must be the owner to change permissions
	if bs.verifyPermissions {
		state, ok := bs.files[path]
		if !ok {
			return os.ErrNotExist
		}

		// Can only change mode if we're the owner or root
		if bs.currentUser != nil {
			uid, _ := strconv.Atoi(bs.currentUser.Uid)
			if uid != 0 && uid != state.uid {
				return fmt.Errorf("insufficient permissions to change mode: not owner")
			}
		}
	}

	st, ok := bs.files[path]
	if !ok {
		return os.ErrNotExist
	}
	st.fileMode = mode

	// If the file wasn't removed, mark changed
	chg, inChg := bs.changed[path]
	if !inChg {
		chg = fileChange{}
	}
	chg.perm = mode
	bs.changed[path] = chg

	return nil
}

// ReadDir returns directories only from the in-memory map.
func (bs *BufferedService) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check if it's a directory in memory
	st, ok := bs.files[path]
	if !ok || !st.isDir {
		return nil, fmt.Errorf("not a directory: %s", path)
	}

	// Gather immediate children (1 level)
	var entries []os.DirEntry
	prefix := path
	if prefix != "" && !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}

	for filePath, fileState := range bs.files {
		// If it has the prefix and does not contain another separator after prefix, it's an immediate child
		if filePath != path && strings.HasPrefix(filePath, prefix) {
			// Get the relative path from the prefix
			remainder := filePath[len(prefix):]
			// Check if remainder has no further slash => direct child
			if !strings.Contains(remainder, string(os.PathSeparator)) {
				// Check if not removed
				if chg, inChg := bs.changed[filePath]; inChg && chg.removed {
					continue
				}
				// Build a synthetic dirEntry
				entries = append(entries, &memDirEntry{
					name:  filepath.Base(filePath),
					isDir: fileState.isDir,
					info: &memFileInfo{
						name:  filepath.Base(filePath),
						size:  fileState.size,
						mode:  fileState.fileMode,
						mtime: fileState.modTime,
						dir:   fileState.isDir,
					},
				})
			}
		}
	}
	return entries, nil
}

// ExecuteCommand delegates to the base service
func (bs *BufferedService) ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return bs.base.ExecuteCommand(ctx, name, args...)
}

// GetFiles returns a copy of the in-memory files map
func (bs *BufferedService) GetFiles() map[string]fileState {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	files := make(map[string]fileState)
	for k, v := range bs.files {
		files[k] = v
	}
	return files
}

// ======================
// Helpers and Mini-Types
// ======================

func filepathBase(path string) string {
	return filepath.Base(path)
}

// hasPrefix is a small helper to check prefix
func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

// memFileInfo is a trivial in-memory file info
type memFileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	mtime time.Time
	dir   bool
}

func (m *memFileInfo) Name() string       { return m.name }
func (m *memFileInfo) Size() int64        { return m.size }
func (m *memFileInfo) Mode() os.FileMode  { return m.mode }
func (m *memFileInfo) ModTime() time.Time { return m.mtime }
func (m *memFileInfo) IsDir() bool        { return m.dir }
func (m *memFileInfo) Sys() interface{}   { return nil }

// memDirEntry is a trivial in-memory dir entry
type memDirEntry struct {
	name  string
	isDir bool
	info  os.FileInfo
}

func (m *memDirEntry) Name() string               { return m.name }
func (m *memDirEntry) IsDir() bool                { return m.isDir }
func (m *memDirEntry) Type() os.FileMode          { return m.info.Mode().Type() }
func (m *memDirEntry) Info() (os.FileInfo, error) { return m.info, nil }

// getFileOwner gets the UID and GID of a file
func getFileOwner(path string) (uid, gid int) {
	info, err := os.Stat(path)
	if err != nil {
		return -1, -1 // Couldn't determine
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(stat.Uid), int(stat.Gid)
	}

	return -1, -1 // Not available on this platform
}

// countFiles counts the number of regular files (not directories) in a map of CachedFile
func countFiles(files map[string]*CachedFile) int {
	count := 0
	for _, cf := range files {
		if !cf.IsDir {
			count++
		}
	}
	return count
}

// getFileInode returns the inode number of a file to detect if the file has been replaced
func getFileInode(path string) uint64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}

	return 0
}

// readFileIncrementally reads only the new content from a file starting at the previousSize position.
func (bs *BufferedService) readFileIncrementally(absPath string, previousSize int64, previousContent []byte, logger *zap.SugaredLogger) ([]byte, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for incremental read: %w", err)
	}
	defer func() {
		closeErr := file.Close()
		if closeErr != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, logger, "failed to close file for incremental read: %v", closeErr)
		}
	}()

	// Seek to the position where we left off
	_, err = file.Seek(previousSize, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to seek in file: %w", err)
	}

	// Get the current file info to determine how much to read
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Read the new content
	newContent := make([]byte, fileInfo.Size()-previousSize)
	_, err = file.Read(newContent)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to read new content: %w", err)
	}

	// Concatenate old and new content
	return append(previousContent, newContent...), nil
}
