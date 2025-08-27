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
	"os"
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
