package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

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
}

// fileState holds in-memory data and metadata for a single file or directory
type fileState struct {
	isDir    bool
	content  []byte // might be empty if we skipped reading (e.g. large file)
	modTime  time.Time
	fileMode os.FileMode
	size     int64
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
	return &BufferedService{
		base:          base,
		files:         make(map[string]fileState),
		changed:       make(map[string]fileChange),
		syncDirs:      syncDirectories,
		maxFileSize:   10 * 1024 * 1024, // 10 MB default threshold for demonstration
		pathsToIgnore: pathsToIgnore,
	}
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
func (bs *BufferedService) SyncFromDisk(ctx context.Context) error {
	logger := logger.For(logger.ComponentFilesystem)
	startTime := time.Now()
	logger.Debugf("SyncFromDisk started for %d directories", len(bs.syncDirs))
	defer func() {
		logger.Debugf("SyncFromDisk took %dms", time.Since(startTime).Milliseconds())
	}()

	// Create a new map to hold synced files
	newFiles := make(map[string]fileState)

	// Sync each directory in the list
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

		// Read the directory tree
		dc, err := ReadDirectoryTree(ctx, bs.base, dir)
		if err != nil {
			return fmt.Errorf("failed to read directory tree for %s: %w", dir, err)
		}

		// Add directory itself
		newFiles[dir] = fileState{
			isDir:    true,
			content:  nil,
			modTime:  info.ModTime(),
			fileMode: info.Mode(),
			size:     0,
		}

		// Process all files in this directory
		for path, cf := range dc.Files {
			// Get absolute path
			absPath := filepath.Join(dir, path)

			// Check if path should be ignored
			if bs.shouldIgnorePath(absPath) {
				continue
			}

			// Process the file or directory
			if cf.IsDir {
				newFiles[absPath] = fileState{
					isDir:    true,
					content:  nil,
					modTime:  cf.Info.ModTime(),
					fileMode: cf.Info.Mode(),
					size:     0,
				}
			} else {
				// Skip large files
				if cf.Info.Size() > bs.maxFileSize {
					newFiles[absPath] = fileState{
						isDir:    false,
						content:  nil, // big file => skip content
						modTime:  cf.Info.ModTime(),
						fileMode: cf.Info.Mode(),
						size:     cf.Info.Size(),
					}
					continue
				}

				// Read file content
				data, err := bs.base.ReadFile(ctx, absPath)
				if err != nil {
					// if the file does not exist anymore, then remove it from the map
					// Likely just a temporary file between reading the directory and then here reading the file
					if os.IsNotExist(err) {
						logger.Debugf("File does not exist, removing: %s", absPath)
						delete(newFiles, absPath)
						continue
					} else {
						// If we can't read, throw an error
						logger.Warnf("ReadFile for %s failed: %v", absPath, err)
						return fmt.Errorf("failed to read file: %w", err)
					}
				}

				newFiles[absPath] = fileState{
					isDir:    false,
					content:  data,
					modTime:  cf.Info.ModTime(),
					fileMode: cf.Info.Mode(),
					size:     cf.Info.Size(),
				}
			}
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
//
// Note: this does not check if the file has changed on disk.
func (bs *BufferedService) SyncToDisk(ctx context.Context) error {
	start := time.Now()
	logger := logger.For(logger.ComponentFilesystem)
	logger.Debugf("SyncToDisk started")
	defer func() {
		logger.Debugf("SyncToDisk took %dms", time.Since(start).Milliseconds())
	}()

	bs.mu.Lock()
	defer bs.mu.Unlock()

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

	// If there's an existing fileState, update it so subsequent ReadFile sees new content
	st, exists := bs.files[path]
	if !exists {
		st = fileState{
			isDir:    false,
			content:  data,
			modTime:  time.Time{}, // unknown until we flush
			fileMode: perm,
			size:     int64(len(data)),
		}
		bs.files[path] = st
	} else {
		st.isDir = false
		st.content = data
		st.fileMode = perm
		st.size = int64(len(data))
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

	st, ok := bs.files[path]
	if !ok {
		return false, nil
	}
	// If it's in changed as removed, treat it as not existing
	chg, inChanged := bs.changed[path]
	if inChanged && chg.removed {
		return false, nil
	}
	// We have the file entry and it's not marked for removal, so it exists
	_ = st // Using st to avoid unused variable warning
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

// containsSeparator checks if there's any os.PathSeparator in s
func containsSeparator(s string) bool {
	return strings.Contains(s, string(os.PathSeparator))
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
