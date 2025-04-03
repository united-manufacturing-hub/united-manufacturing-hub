package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
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
		fullPath := filepathJoin(root, entry.Name())
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

		dc.Files[relPath] = &CachedFile{
			Info:  info,
			IsDir: info.IsDir(),
		}

		// If it's a directory, recurse
		if info.IsDir() {
			subDc, err := ReadDirectoryTree(ctx, service, fullPath)
			if err != nil {
				return nil, err
			}
			// Add all files from subdirectory
			for subPath, subFile := range subDc.Files {
				dc.Files[filepathJoin(relPath, subPath)] = subFile
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
	// Key: full path on disk (absolute or relative, your choice).
	// Value: fileState describing content, modTime when we read it, etc.
	files map[string]*fileState

	// changed tracks which files the reconcile loop has modified in memory.
	// If a file is in changed[], we plan to write it out in SyncToDisk(). If removed==true, we plan to remove it.
	changed map[string]*fileChange

	// rootDir is the base directory if you want to store an absolute root for references.
	rootDir string

	// maxFileSize is a threshold for reading big logs. If a file is bigger, we skip it. It should not happen as logs have a 1MB limit.
	maxFileSize int64
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
// rootDir indicates the path that SyncFromDisk() will read from.
func NewBufferedService(base Service, rootDir string) *BufferedService {
	return &BufferedService{
		base:        base,
		files:       make(map[string]*fileState),
		changed:     make(map[string]*fileChange),
		rootDir:     rootDir,
		maxFileSize: 10 * 1024 * 1024, // 10 MB default threshold for demonstration
	}
}

// =========================
// SyncFromDisk / SyncToDisk
// =========================

// SyncFromDisk loads the filesystem state into memory, ignoring anything we had before.
// It will read file contents unless they exceed maxFileSize, in which case content is blank.
func (bs *BufferedService) SyncFromDisk(ctx context.Context) error {
	// We'll do a fresh walk of the directory.
	dc, err := ReadDirectoryTree(ctx, bs.base, bs.rootDir)
	if err != nil {
		return err
	}

	// We'll rebuild the entire in-memory map.
	// This is "atomic" under a lock.
	newFiles := make(map[string]*fileState)
	for relPath, cf := range dc.Files {
		// Skip directories or store them as "isDir"
		if cf.IsDir {
			newFiles[relPath] = &fileState{
				isDir:    true,
				content:  nil,
				modTime:  cf.Info.ModTime(),
				fileMode: cf.Info.Mode(),
				size:     0,
			}
			continue
		}

		// If file is bigger than maxFileSize, skip reading content
		if cf.Info.Size() > bs.maxFileSize {
			newFiles[relPath] = &fileState{
				isDir:    false,
				content:  nil, // big file => skip content
				modTime:  cf.Info.ModTime(),
				fileMode: cf.Info.Mode(),
				size:     cf.Info.Size(),
			}
			continue
		}

		// Otherwise, read content from disk once
		absolutePath := filepathJoin(bs.rootDir, relPath)
		data, err := bs.base.ReadFile(ctx, absolutePath)
		if err != nil {
			// If we can't read, skip this file
			continue
		}

		newFiles[relPath] = &fileState{
			isDir:    false,
			content:  data,
			modTime:  cf.Info.ModTime(),
			fileMode: cf.Info.Mode(),
			size:     cf.Info.Size(),
		}
	}

	bs.mu.Lock()
	bs.files = newFiles
	// Clear any pending changes since we are reloading from disk
	bs.changed = make(map[string]*fileChange)
	bs.mu.Unlock()

	return nil
}

// SyncToDisk flushes all changed files to disk, and removes any marked for removal.
//
// Note: this does not check if the file has changed on disk.
func (bs *BufferedService) SyncToDisk(ctx context.Context) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// First, handle removals in reverse order (deepest paths first)
	var toRemove []string
	for relPath, chg := range bs.changed {
		if chg.removed {
			toRemove = append(toRemove, relPath)
		}
	}
	// Sort paths by length in descending order to remove deepest paths first
	sort.Slice(toRemove, func(i, j int) bool {
		return len(toRemove[i]) > len(toRemove[j])
	})

	for _, relPath := range toRemove {
		fullPath := filepathJoin(bs.rootDir, relPath)
		// Check if this was a directory in our original state
		chg := bs.changed[relPath]
		if chg.wasDir {
			if err := bs.base.RemoveAll(ctx, fullPath); err != nil {
				return fmt.Errorf("failed to remove directory: %w", err)
			}
		} else {
			if err := bs.base.Remove(ctx, fullPath); err != nil {
				return fmt.Errorf("failed to remove file: %w", err)
			}
		}
		// Only remove from our in-memory maps after successful removal
		delete(bs.files, relPath)
		delete(bs.changed, relPath)
	}

	// Then handle writes and directory creation
	for relPath, chg := range bs.changed {
		if !chg.removed {
			// If it's not in our map for some reason, skip
			state, exists := bs.files[relPath]
			if !exists {
				return fmt.Errorf("file not found in memory: %s", relPath)
			}

			// Handle directories differently from files
			if state.isDir {
				fullPath := filepathJoin(bs.rootDir, relPath)
				if err := bs.base.EnsureDirectory(ctx, fullPath); err != nil {
					return fmt.Errorf("failed to create directory: %w", err)
				}
			} else {
				// Do the write for regular files
				absolutePath := filepathJoin(bs.rootDir, relPath)
				if err := bs.base.WriteFile(ctx, absolutePath, chg.content, chg.perm); err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}
			}
			delete(bs.changed, relPath)
		}
	}

	return nil
}

// =========================
// Implementation of Service
// =========================

// EnsureDirectory creates the directory in-memory and marks it for creation on disk.
func (bs *BufferedService) EnsureDirectory(ctx context.Context, path string) error {
	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return err
	}

	bs.mu.Lock()
	defer bs.mu.Unlock()

	// If already exists, check if it is a directory.
	if state, ok := bs.files[relPath]; ok {
		if !state.isDir {
			return fmt.Errorf("%s exists and is not a directory", path)
		}
		return nil
	}

	// Create new directory entry.
	bs.files[relPath] = &fileState{
		isDir:    true,
		modTime:  time.Now(),
		fileMode: os.ModeDir | 0755,
		size:     0,
	}
	// Mark as changed so that SyncToDisk can create it on disk.
	bs.changed[relPath] = &fileChange{
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

	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return nil, err
	}
	st, ok := bs.files[relPath]
	if !ok || st.isDir {
		return nil, os.ErrNotExist
	}
	// If it's a large file (content is nil but size is set), return not exist
	if st.content == nil && st.size > bs.maxFileSize {
		return nil, os.ErrNotExist
	}
	return st.content, nil
}

// WriteFile does not immediately write to disk; it marks the file as changed in memory.
func (bs *BufferedService) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return err
	}

	// If there's an existing fileState, update it so subsequent ReadFile sees new content
	st, exists := bs.files[relPath]
	if !exists {
		st = &fileState{
			isDir:    false,
			content:  data,
			modTime:  time.Time{}, // unknown until we flush
			fileMode: perm,
			size:     int64(len(data)),
		}
		bs.files[relPath] = st
	} else {
		st.isDir = false
		st.content = data
		st.fileMode = perm
		st.size = int64(len(data))
	}

	// Mark changed
	bs.changed[relPath] = &fileChange{
		content: data,
		perm:    perm,
		removed: false,
	}
	return nil
}

// FileExists checks the in-memory map only.
func (bs *BufferedService) FileExists(ctx context.Context, path string) (bool, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return false, err
	}
	st, ok := bs.files[relPath]
	if !ok {
		return false, nil
	}
	// If it's in changed as removed, treat it as not existing
	chg, inChanged := bs.changed[relPath]
	if inChanged && chg.removed {
		return false, nil
	}
	return !st.isDir, nil
}

// Remove marks the file for removal in memory.
func (bs *BufferedService) Remove(ctx context.Context, path string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return err
	}

	// If we have a fileState, check if it's a directory
	if st, ok := bs.files[relPath]; ok {
		if st.isDir {
			return fmt.Errorf("cannot remove: is directory")
		}
		// Delete from files map immediately
		delete(bs.files, relPath)
	}

	bs.changed[relPath] = &fileChange{
		removed: true,
	}
	return nil
}

// RemoveAll recursively marks all files and directories under the given path as removed in-memory.
func (bs *BufferedService) RemoveAll(ctx context.Context, path string) error {
	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return err
	}

	bs.mu.Lock()
	defer bs.mu.Unlock()

	// First check if the target itself is a directory
	targetIsDir := false
	if st, ok := bs.files[relPath]; ok {
		targetIsDir = st.isDir
	}

	// Mark the target and all its children for removal
	for key := range bs.files {
		if key == relPath || hasPrefix(key, relPath+string(os.PathSeparator)) {
			isDir := bs.files[key].isDir
			bs.changed[key] = &fileChange{removed: true, wasDir: isDir}
			// Also mark it as removed in the files map
			delete(bs.files, key)
		}
	}

	// Also mark the target itself for removal even if it wasn't in our files map
	bs.changed[relPath] = &fileChange{removed: true, wasDir: targetIsDir}
	return nil
}

// MkdirTemp creates a temporary directory in-memory and marks it for creation on disk.
func (bs *BufferedService) MkdirTemp(ctx context.Context, dir, pattern string) (string, error) {
	// First try to create the directory using the base service
	tempDir, err := bs.base.MkdirTemp(ctx, dir, pattern)
	if err != nil {
		return "", err
	}

	bs.mu.Lock()
	defer bs.mu.Unlock()

	relPath, err := makeRelative(bs.rootDir, tempDir)
	if err != nil {
		return "", err
	}

	bs.files[relPath] = &fileState{
		isDir:    true,
		modTime:  time.Now(),
		fileMode: os.ModeDir | 0755,
		size:     0,
	}
	bs.changed[relPath] = &fileChange{
		removed: false,
		perm:    os.ModeDir | 0755,
	}
	return tempDir, nil
}

// Stat returns an os.FileInfo-like object if it is in the in-memory map. Otherwise os.ErrNotExist.
func (bs *BufferedService) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return nil, err
	}

	st, ok := bs.files[relPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	if st.isDir {
		// Return a synthetic fileInfo for a directory
		return &memFileInfo{
			name:  filepathBase(relPath),
			size:  0,
			mode:  st.fileMode,
			mtime: st.modTime,
			dir:   true,
		}, nil
	}
	return &memFileInfo{
		name:  filepathBase(relPath),
		size:  st.size,
		mode:  st.fileMode,
		mtime: st.modTime,
		dir:   false,
	}, nil
}

// CreateFile calls WriteFile with empty content (or you can pass through).
func (bs *BufferedService) CreateFile(ctx context.Context, path string, perm os.FileMode) (*os.File, error) {
	err := bs.WriteFile(ctx, path, nil, perm)
	if err != nil {
		return nil, err
	}
	// We don't actually have a real *os.File, so returning nil.
	return nil, errors.New("BufferedService.CreateFile: not supported returning *os.File (in-memory only)")
}

// Chmod updates the fileMode in memory (and later flush).
func (bs *BufferedService) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return err
	}

	st, ok := bs.files[relPath]
	if !ok {
		return os.ErrNotExist
	}
	st.fileMode = mode

	// If the file wasn't removed, mark changed
	chg, inChg := bs.changed[relPath]
	if !inChg {
		chg = &fileChange{}
	}
	chg.perm = mode
	bs.changed[relPath] = chg

	return nil
}

// ReadDir returns directories only from the in-memory map.
func (bs *BufferedService) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	relPath, err := makeRelative(bs.rootDir, path)
	if err != nil {
		return nil, err
	}

	// Check if it's a directory in memory
	st, ok := bs.files[relPath]
	if !ok || !st.isDir {
		return nil, fmt.Errorf("not a directory: %s", path)
	}

	// Gather immediate children (1 level)
	var entries []os.DirEntry
	prefix := relPath
	if prefix != "" {
		prefix += string(os.PathSeparator)
	}
	for filePath, fileState := range bs.files {
		// If it has the prefix and does not contain another separator after prefix, it's an immediate child
		if len(filePath) > len(prefix) && hasPrefix(filePath, prefix) {
			remainder := filePath[len(prefix):]
			// Check if remainder has no further slash => direct child
			if !containsSeparator(remainder) {
				// Check if not removed
				if chg, inChg := bs.changed[filePath]; inChg && chg.removed {
					continue
				}
				// Build a synthetic dirEntry
				entries = append(entries, &memDirEntry{
					name:  filepathBase(filePath),
					isDir: fileState.isDir,
					info: &memFileInfo{
						name:  filepathBase(filePath),
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

// ======================
// Helpers and Mini-Types
// ======================

// If you don't have a built-in join function, define something small:
func filepathJoin(base, rel string) string {
	if base == "" {
		return rel
	}
	return filepath.Join(base, rel)
}

func filepathBase(path string) string {
	return filepath.Base(path)
}

// makeRelative ensures `path` is relative to `rootDir`, or returns an error if it can't.
func makeRelative(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return rel, nil
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
	for i := 0; i < len(s); i++ {
		if s[i] == os.PathSeparator {
			return true
		}
	}
	return false
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
