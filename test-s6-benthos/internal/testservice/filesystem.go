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

package testservice

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// FilesystemService implements basic filesystem operations for the test
type FilesystemService struct{}

// NewFilesystemService creates a new filesystem service
func NewFilesystemService() *FilesystemService {
	return &FilesystemService{}
}

// EnsureDirectory creates a directory if it doesn't exist
func (fs *FilesystemService) EnsureDirectory(ctx context.Context, path string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return os.MkdirAll(path, 0755)
}

// ReadFile reads a file's contents respecting the context
func (fs *FilesystemService) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return os.ReadFile(path)
}

// ReadFileRange reads the file starting at byte offset "from"
func (fs *FilesystemService) ReadFileRange(ctx context.Context, path string, from int64) ([]byte, int64, error) {
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}

	size := stat.Size()
	if from >= size {
		return nil, size, nil
	}

	_, err = file.Seek(from, 0)
	if err != nil {
		return nil, 0, err
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, 0, err
	}

	return data, size, nil
}

// WriteFile writes data to a file respecting the context
func (fs *FilesystemService) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := fs.EnsureDirectory(ctx, dir); err != nil {
		return err
	}

	return os.WriteFile(path, data, perm)
}

// PathExists checks if a file or directory exists at the given path
func (fs *FilesystemService) PathExists(ctx context.Context, path string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// FileExists checks if a file exists (same as PathExists for this implementation)
func (fs *FilesystemService) FileExists(ctx context.Context, path string) (bool, error) {
	return fs.PathExists(ctx, path)
}

// Remove removes a file or directory
func (fs *FilesystemService) Remove(ctx context.Context, path string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return os.Remove(path)
}

// RemoveAll removes a directory and all its contents
func (fs *FilesystemService) RemoveAll(ctx context.Context, path string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return os.RemoveAll(path)
}

// Stat returns file info
func (fs *FilesystemService) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return os.Stat(path)
}

// Chmod changes the mode of the named file
func (fs *FilesystemService) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return os.Chmod(path, mode)
}

// ReadDir reads a directory, returning all its directory entries
func (fs *FilesystemService) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return os.ReadDir(path)
}

// ExecuteCommand executes a command with context
func (fs *FilesystemService) ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// Chown changes the owner and group of the named file
func (fs *FilesystemService) Chown(ctx context.Context, path string, user string, group string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// For simplicity in this test, we'll skip actual chown operations
	// In a real implementation, you'd need to lookup user/group IDs
	return nil
}

// Glob is a wrapper around filepath.Glob that respects the context
func (fs *FilesystemService) Glob(ctx context.Context, pattern string) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return filepath.Glob(pattern)
}

// Rename renames (moves) a file or directory from oldPath to newPath
func (fs *FilesystemService) Rename(ctx context.Context, oldPath, newPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return os.Rename(oldPath, newPath)
}

// Symlink creates a symbolic link from linkPath to target
func (fs *FilesystemService) Symlink(ctx context.Context, target, linkPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Ensure parent directory exists
	dir := filepath.Dir(linkPath)
	if err := fs.EnsureDirectory(ctx, dir); err != nil {
		return err
	}

	return os.Symlink(target, linkPath)
}

// Helper function to get inode (used by s6 log handling)
func getInodeFromStat(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}

// parseUID converts a user name or ID to numeric UID
func parseUID(user string) (int, error) {
	if uid, err := strconv.Atoi(user); err == nil {
		return uid, nil
	}

	// For simplicity, return a default UID for named users
	switch strings.ToLower(user) {
	case "root":
		return 0, nil
	case "nobody":
		return 65534, nil
	default:
		return 1000, nil // Default user
	}
}

// parseGID converts a group name or ID to numeric GID
func parseGID(group string) (int, error) {
	if gid, err := strconv.Atoi(group); err == nil {
		return gid, nil
	}

	// For simplicity, return a default GID for named groups
	switch strings.ToLower(group) {
	case "root":
		return 0, nil
	case "nogroup":
		return 65534, nil
	default:
		return 1000, nil // Default group
	}
}
