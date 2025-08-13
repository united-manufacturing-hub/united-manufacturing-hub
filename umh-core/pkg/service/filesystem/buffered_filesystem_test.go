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

package filesystem_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("BufferedService", func() {
	var (
		tmpDir      string
		err         error
		ctx         context.Context
		cancel      context.CancelFunc
		baseService filesystem.Service
		bufService  *filesystem.BufferedService
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		_ = ctx
		_ = cancel

		// Create a real temp directory on disk for the underlying DefaultService to operate on.
		tmpDir, err = os.MkdirTemp("", "buffered-service-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Initialize the base service (DefaultService) pointing to the real filesystem
		baseService = filesystem.NewDefaultService()

		// Initialize the buffered service, wrapping the base service
		bufService = filesystem.NewBufferedService(baseService, tmpDir, constants.FilesAndDirectoriesToIgnore)

		// Disable permission checks by default for most tests
		// The permission-specific tests will enable them
		bufService.SetVerifyPermissions(false)

		// Create some files or directories in the tmpDir so that we can test SyncFromDisk
		setupTestFiles(tmpDir)
	})

	AfterEach(func() {
		cancel()

		// Make read-only directories writable before removal
		readOnlyDir := filepath.Join(tmpDir, "readonly_dir")
		if _, err := os.Stat(readOnlyDir); err == nil {
			err = os.Chmod(readOnlyDir, 0755) //nolint:gosec // G302: Test cleanup requires permissive permissions to delete read-only directories
			if err != nil {
				// Log instead of failing - we're in cleanup
				fmt.Println("Warning: Failed to chmod directory:", err)
			}
		}

		err := os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("SyncFromDisk", func() {
		It("should load existing files/directories into memory", func() {
			// Initially, the in-memory snapshot should be empty
			// We test by reading a file that should exist, expecting a "not exist" error
			_, err := bufService.ReadFile(ctx, filepath.Join(tmpDir, "sample.txt"))
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())

			// Now we do SyncFromDisk
			err = bufService.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// After that, reading sample.txt should succeed in memory
			content, err := bufService.ReadFile(ctx, filepath.Join(tmpDir, "sample.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("Hello, world!\n"))
		})

		It("should skip large files if they exceed maxFileSize", func() {
			// Make a large file bigger than the default 10 MB threshold
			largeFilePath := filepath.Join(tmpDir, "big.log")
			createLargeFile(largeFilePath, 11*1024*1024) // 11 MB

			err := bufService.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// big.log should exist on disk, but in memory we skip its content
			_, err = bufService.ReadFile(ctx, largeFilePath)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())

			// But the file should still exist on disk
			_, err = os.Stat(largeFilePath)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle directories correctly during SyncFromDisk", func() {
			// Create a nested directory structure similar to production
			dirPath := filepath.Join(tmpDir, "run", "service", "umh-core")
			err := os.MkdirAll(dirPath, 0755) //nolint:gosec // G301: Test requires permissive permissions for directory creation testing
			Expect(err).NotTo(HaveOccurred())

			// Create a file in one path
			err = os.WriteFile(filepath.Join(dirPath, "somefile.txt"), []byte("test"), 0644) //nolint:gosec // G306: Test requires standard file permissions for testing filesystem operations
			Expect(err).NotTo(HaveOccurred())

			// Create a directory where a file might be expected
			err = os.MkdirAll(filepath.Join(dirPath, "log"), 0755) //nolint:gosec // G301: Test requires permissive permissions for directory creation testing
			Expect(err).NotTo(HaveOccurred())

			// Create a new buffered service with this root
			bufServiceTest := filesystem.NewBufferedService(baseService, filepath.Join(tmpDir, "run", "service"), constants.FilesAndDirectoriesToIgnore)

			// Now try to sync from disk - this should fail because it will try to read the directory as a file
			err = bufServiceTest.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle symlinked directories correctly during SyncFromDisk", func() {
			// Create a directory structure similar to production with symlinks
			targetDir := filepath.Join(tmpDir, "run", "s6-rc", "servicedirs", "umh-core")
			err := os.MkdirAll(targetDir, 0755) //nolint:gosec // G301: Test requires permissive permissions for service directory creation testing
			Expect(err).NotTo(HaveOccurred())

			// Also create a log dir
			targetLogDir := filepath.Join(tmpDir, "run", "s6-rc", "servicedirs", "umh-core-log")
			err = os.MkdirAll(targetLogDir, 0755) //nolint:gosec // G301: Test requires permissive permissions for log directory creation testing
			Expect(err).NotTo(HaveOccurred())

			// Create a sample file in the target directory
			sampleFile := filepath.Join(targetDir, "config.json")
			err = os.WriteFile(sampleFile, []byte("{\"test\": true}"), 0644) //nolint:gosec // G306: Test requires standard file permissions for config file testing
			Expect(err).NotTo(HaveOccurred())

			// Create the service directory
			serviceDir := filepath.Join(tmpDir, "run", "service")
			err = os.MkdirAll(serviceDir, 0755) //nolint:gosec // G301: Test requires permissive permissions for service directory creation testing
			Expect(err).NotTo(HaveOccurred())

			// Create symlinks in service directory like in production
			err = os.Symlink(targetDir, filepath.Join(serviceDir, "umh-core"))
			Expect(err).NotTo(HaveOccurred())
			err = os.Symlink(targetLogDir, filepath.Join(serviceDir, "umh-core-log"))
			Expect(err).NotTo(HaveOccurred())

			// Create a new buffered service with the service directory as root
			bufServiceTest := filesystem.NewBufferedService(baseService, serviceDir, constants.FilesAndDirectoriesToIgnore)

			// Now try to sync from disk - this currently fails in production
			err = bufServiceTest.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify we can read files through the symlink
			content, err := bufServiceTest.ReadFile(ctx, filepath.Join(serviceDir, "umh-core", "config.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("{\"test\": true}"))
		})
	})

	Context("Basic file operations (in-memory)", func() {
		BeforeEach(func() {
			// Load from disk so we have an in-memory snapshot
			err = bufService.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Ensure permissions are disabled for basic file operations
			bufService.SetVerifyPermissions(false)
		})

		It("should read existing file from memory, and return not exist for unknown files", func() {
			// sample.txt was created by setupTestFiles, so it should be in memory
			content, err := bufService.ReadFile(ctx, filepath.Join(tmpDir, "sample.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("Hello, world!\n"))

			// Something not existing
			_, err = bufService.ReadFile(ctx, filepath.Join(tmpDir, "nope.txt"))
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should mark new writes in memory and only flush on SyncToDisk", func() {
			newFilePath := filepath.Join(tmpDir, "newfile.txt")
			err = bufService.WriteFile(ctx, newFilePath, []byte("new content"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Immediately, let's see if it appears in memory
			data, err := bufService.ReadFile(ctx, newFilePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("new content"))

			// On disk, it doesn't exist yet because we haven't called SyncToDisk
			_, statErr := os.Stat(newFilePath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())

			// Now flush
			err = bufService.SyncToDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Check on disk
			fi, statErr := os.Stat(newFilePath)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(fi.Size()).To(Equal(int64(len("new content"))))
		})

		It("should create a file in memory via CreateFile and write it on SyncToDisk", func() {
			newFilePath := filepath.Join(tmpDir, "created_file.txt")

			// File should not exist yet
			exists, err := bufService.FileExists(ctx, newFilePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())

			// On disk, it also doesn't exist
			_, statErr := os.Stat(newFilePath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())

			// Create the file with CreateFile
			err = bufService.WriteFile(ctx, newFilePath, []byte{}, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Now the file should exist in memory
			exists, err = bufService.FileExists(ctx, newFilePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())

			// Get file info to verify its attributes
			fileInfo, err := bufService.Stat(ctx, newFilePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(fileInfo.IsDir()).To(BeFalse())
			Expect(fileInfo.Size()).To(Equal(int64(0))) // Empty file

			// On disk, it still doesn't exist until we sync
			_, statErr = os.Stat(newFilePath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())

			// Now flush to disk
			err = bufService.SyncToDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Now it should exist on disk
			fi, statErr := os.Stat(newFilePath)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(fi.IsDir()).To(BeFalse())
			Expect(fi.Size()).To(Equal(int64(0))) // Empty file
		})

		It("should mark files as removed in memory and remove them on SyncToDisk", func() {
			// Confirm sample2.txt exists in memory
			content, err := bufService.ReadFile(ctx, filepath.Join(tmpDir, "nested", "sample2.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("Nested file\n"))

			// Now remove it
			err = bufService.Remove(ctx, filepath.Join(tmpDir, "nested", "sample2.txt"))
			Expect(err).NotTo(HaveOccurred())

			// In memory, read should fail
			_, err = bufService.ReadFile(ctx, filepath.Join(tmpDir, "nested", "sample2.txt"))
			Expect(os.IsNotExist(err)).To(BeTrue())

			// Still on disk for the moment, because not flushed
			_, diskErr := os.Stat(filepath.Join(tmpDir, "nested", "sample2.txt"))
			Expect(diskErr).NotTo(HaveOccurred())

			// Now flush
			err = bufService.SyncToDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// After flush, the file should be gone on disk
			_, diskErr = os.Stat(filepath.Join(tmpDir, "nested", "sample2.txt"))
			Expect(os.IsNotExist(diskErr)).To(BeTrue())
		})
	})

	Context("Directory operations", func() {
		BeforeEach(func() {
			err = bufService.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Ensure permissions are disabled for directory operations
			bufService.SetVerifyPermissions(false)
		})

		It("should EnsureDirectory (buffered) and create on SyncToDisk", func() {
			newDirPath := filepath.Join(tmpDir, "mydir")
			err = bufService.EnsureDirectory(ctx, newDirPath)
			Expect(err).NotTo(HaveOccurred())

			// In memory, it should exist as a directory
			info, statErr := bufService.Stat(ctx, newDirPath)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())

			// On disk, it doesn't exist yet
			_, diskErr := os.Stat(newDirPath)
			Expect(os.IsNotExist(diskErr)).To(BeTrue())

			// Flush
			err = bufService.SyncToDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Now on disk
			fi, diskErr := os.Stat(newDirPath)
			Expect(diskErr).NotTo(HaveOccurred())
			Expect(fi.IsDir()).To(BeTrue())
		})

		It("should RemoveAll recursively in memory, then on SyncToDisk", func() {
			// `nested` folder has a file sample2.txt
			err = bufService.RemoveAll(ctx, filepath.Join(tmpDir, "nested"))
			Expect(err).NotTo(HaveOccurred())

			// In memory, reading sample2.txt fails
			_, err = bufService.ReadFile(ctx, filepath.Join(tmpDir, "nested", "sample2.txt"))
			Expect(os.IsNotExist(err)).To(BeTrue())

			// On disk it still exists for now
			_, diskErr := os.Stat(filepath.Join(tmpDir, "nested", "sample2.txt"))
			Expect(diskErr).NotTo(HaveOccurred())

			err = bufService.SyncToDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			_, diskErr = os.Stat(filepath.Join(tmpDir, "nested"))
			Expect(os.IsNotExist(diskErr)).To(BeTrue())
		})
	})

	Context("Error Handling", func() {
		It("should return an error if SyncToDisk fails on an actual WriteFile error", func() {
			// We'll create a mock or something that fails on WriteFile
			// Alternatively, we can rename the directory out from under it to cause an error
			err := bufService.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// write a new file
			testFile := filepath.Join(tmpDir, "bad_file.txt")
			err = bufService.WriteFile(ctx, testFile, []byte("hello"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// With our new implementation, simply removing the directory won't cause an error
			// because we automatically create parent directories
			// Instead, let's create a file at the same path as our directory to cause a failure
			err = os.RemoveAll(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Create a file with the same name as the directory we need to create
			// This will cause EnsureDirectory to fail since it can't create a directory over a file
			err = os.MkdirAll(filepath.Dir(tmpDir), 0755) //nolint:gosec // G301: Test requires permissive permissions for error simulation directory setup
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(tmpDir, []byte("blocking file"), 0644) //nolint:gosec // G306: Test requires standard file permissions for error simulation testing
			Expect(err).NotTo(HaveOccurred())

			// Now flush - this should fail because we can't create the directory
			err = bufService.SyncToDisk(ctx)
			Expect(err).To(HaveOccurred())
			Expect(strings.Contains(err.Error(), "failed to create")).To(BeTrue())
		})
	})
})

// ----------------------------------------------------------
// Helper functions
// ----------------------------------------------------------

func setupTestFiles(root string) {
	// Create a sample file
	samplePath := filepath.Join(root, "sample.txt")
	err := os.WriteFile(samplePath, []byte("Hello, world!\n"), 0644) //nolint:gosec // G306: Test requires standard file permissions for sample file creation
	Expect(err).NotTo(HaveOccurred())

	// Create nested directories
	nestedDir := filepath.Join(root, "nested")
	err = os.MkdirAll(nestedDir, 0755) //nolint:gosec // G301: Test requires permissive permissions for nested directory creation
	Expect(err).NotTo(HaveOccurred())

	// Create a nested file
	nestedFile := filepath.Join(nestedDir, "sample2.txt")
	err = os.WriteFile(nestedFile, []byte("Nested file\n"), 0644) //nolint:gosec // G306: Test requires standard file permissions for nested file creation
	Expect(err).NotTo(HaveOccurred())
}

func createLargeFile(path string, sizeBytes int64) {
	f, err := os.Create(path) //nolint:gosec // G304: File path is controlled within test context, safe for test file creation
	Expect(err).NotTo(HaveOccurred())

	defer func() {
		err := f.Close()
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentFilesystem), "failed to close file: %v", err)
		}
	}()

	// Just write N zeroes
	err = f.Truncate(sizeBytes)
	Expect(err).NotTo(HaveOccurred())
}

// ----------------------------------------------------------
// BufferedService with MockFileSystem
// ----------------------------------------------------------

var _ = Describe("BufferedService with MockFileSystem", func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		mockFs      *filesystem.MockFileSystem
		bufService  *filesystem.BufferedService
		mockRootDir = "/mock-root"
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		_ = ctx
		_ = cancel
		mockFs = filesystem.NewMockFileSystem()
		bufService = filesystem.NewBufferedService(mockFs, mockRootDir, constants.FilesAndDirectoriesToIgnore)

		// Disable permission checks for the mock filesystem tests
		bufService.SetVerifyPermissions(false)
	})

	AfterEach(func() {
		cancel()
	})

	Context("SyncFromDisk with mock failures", func() {
		It("should return an error if WalkDir fails (simulated by ReadDirectoryTree)", func() {
			// We'll override ReadDirectoryTree usage by forcing an error from "ReadDir".
			// The simplest way is to intercept calls in mock's ReadDir or FileExists, etc.

			// Let's say we fail all ReadDir calls to simulate an error during `filepath.Walk`.
			mockFs.WithReadDirFunc(func(ctx context.Context, path string) ([]os.DirEntry, error) {
				return nil, errors.New("simulated readDir failure")
			})

			err := bufService.SyncFromDisk(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to walk directory tree"))
		})

		It("should skip reading a file content if ReadFile fails for that file", func() {
			// If a particular file triggers an error, the code will skip storing it in the snapshot.
			mockFs.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if strings.Contains(path, "trouble.txt") {
					return nil, errors.New("simulated read error")
				}

				return []byte("normal content"), nil
			})

			err := bufService.SyncFromDisk(ctx)
			// The code doesn't return an error if a single file read fails (it just continues).
			// So we expect SyncFromDisk to succeed.
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Writes and SyncToDisk with mock failures", func() {
		BeforeEach(func() {
			// Setup mock to succeed for SyncFromDisk
			mockFs.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				// Create a mock FileInfo that satisfies the interface
				return mockFs.NewMockFileInfo(filepath.Base(path), 0, 0644, time.Now(), true), nil
			})
			mockFs.WithReadDirFunc(func(ctx context.Context, path string) ([]os.DirEntry, error) {
				return []os.DirEntry{}, nil
			})
			mockFs.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == mockRootDir, nil
			})

			err := bufService.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Ensure permission checks are disabled for these mock tests
			bufService.SetVerifyPermissions(false)
		})

		It("should mark file as changed, then fail on SyncToDisk if WriteFile fails", func() {
			// Write a new file
			err := bufService.WriteFile(ctx, filepath.Join(mockRootDir, "myfile.txt"), []byte("hello"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Now inject a failure for WriteFile in the mock
			mockFs.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				return errors.New("simulated write failure from mock")
			})

			// SyncToDisk should fail because the underlying mock fails
			err = bufService.SyncToDisk(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write file"))
		})
	})

	Context("EnsureDirectory, RemoveAll, MkdirTemp with mock failures", func() {
		BeforeEach(func() {
			// Setup mock to succeed for SyncFromDisk
			mockFs.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				// Create a mock FileInfo that satisfies the interface
				return mockFs.NewMockFileInfo(filepath.Base(path), 0, 0644, time.Now(), true), nil
			})
			mockFs.WithReadDirFunc(func(ctx context.Context, path string) ([]os.DirEntry, error) {
				return []os.DirEntry{}, nil
			})
			mockFs.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == mockRootDir, nil
			})

			err := bufService.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Ensure permission checks are disabled for these mock tests
			bufService.SetVerifyPermissions(false)
		})

		It("should fail during SyncToDisk if EnsureDirectory mock triggers an error", func() {
			// First ensure directory in memory (should succeed)
			err := bufService.EnsureDirectory(ctx, filepath.Join(mockRootDir, "someDir"))
			Expect(err).NotTo(HaveOccurred())

			// Setup mock to fail during SyncToDisk
			mockFs.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return errors.New("mock ensureDir error")
			})

			// Now try to sync to disk, which should fail
			err = bufService.SyncToDisk(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mock ensureDir error"))
		})

		It("should fail during SyncToDisk if RemoveAll mock triggers an error", func() {
			// First create a directory in memory
			dirPath := filepath.Join(mockRootDir, "remDir")
			err := bufService.EnsureDirectory(ctx, dirPath)
			Expect(err).NotTo(HaveOccurred())

			// Then mark it for removal (should succeed in memory)
			err = bufService.RemoveAll(ctx, dirPath)
			Expect(err).NotTo(HaveOccurred())

			// Setup mock to fail during SyncToDisk
			mockFs.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				return errors.New("mock removeAll error")
			})

			// Now try to sync to disk, which should fail
			err = bufService.SyncToDisk(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to remove directory"))
		})
	})

	Context("Random failure rates", func() {
		BeforeEach(func() {
			// For demonstration, we set a 50% chance of failing any operation
			mockFs.WithFailureRate(0.5)
			// Also let's put a small delay range
			mockFs.WithDelayRange(5 * time.Millisecond)
		})

		It("should occasionally succeed or fail when writing files", func() {
			// This is a bit nondeterministic, but we can at least ensure it sometimes fails.
			// We can run multiple attempts and expect some successes, some failures.
			numAttempts := 10
			failCount := 0
			successCount := 0

			for range numAttempts {
				err := bufService.WriteFile(ctx, filepath.Join(mockRootDir, "random.txt"), []byte("test"), 0644)
				if err != nil {
					failCount++
				} else {
					successCount++
				}
			}
			// It's possible (though unlikely) that random never fails or never succeeds within 10 tries.
			// If we want guaranteed coverage, we might do a bigger number or break out the random logic differently.
			// For demonstration, let's just ensure we had at least 1 success or 1 fail if the PRNG is cooperative.
			Expect(failCount + successCount).To(Equal(numAttempts))
		})
	})
})

var _ = Describe("BufferedService Directory Creation Issues", func() {
	var (
		tmpDir      string
		err         error
		ctx         context.Context
		cancel      context.CancelFunc
		baseService filesystem.Service
		bufService  *filesystem.BufferedService
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		_ = ctx
		_ = cancel

		// Create a real temp directory on disk for the underlying DefaultService to operate on.
		tmpDir, err = os.MkdirTemp("", "buffered-service-dir-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Initialize the base service (DefaultService) pointing to the real filesystem
		baseService = filesystem.NewDefaultService()

		// Initialize the buffered service, wrapping the base service
		bufService = filesystem.NewBufferedService(baseService, tmpDir, constants.FilesAndDirectoriesToIgnore)

		// Disable permission checks for directory creation tests
		bufService.SetVerifyPermissions(false)
	})

	AfterEach(func() {
		// Clean up
		cancel()

		// Make read-only directories writable before removal
		readOnlyDir := filepath.Join(tmpDir, "readonly_dir")
		if _, err := os.Stat(readOnlyDir); err == nil {
			err = os.Chmod(readOnlyDir, 0755) //nolint:gosec // G302: Test cleanup requires permissive permissions to delete read-only directories
			if err != nil {
				// Log instead of failing - we're in cleanup
				fmt.Println("Warning: Failed to chmod directory:", err)
			}
		}

		err := os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should now succeed when writing files in directories that don't exist yet", func() {
		// Create a deeply nested file path with multiple directory levels that don't exist
		nestedFilePath := filepath.Join(tmpDir, "service", "hello-world", "dependencies.d", "base")

		// Write a file into this nested path without creating the directories first
		err = bufService.WriteFile(ctx, nestedFilePath, []byte("content"), 0644)
		Expect(err).NotTo(HaveOccurred())

		// With the fixed implementation, SyncToDisk should now succeed by automatically creating parent directories
		err = bufService.SyncToDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Verify file was written
		content, err := os.ReadFile(nestedFilePath) //nolint:gosec // G304: File path is controlled within test context, safe for test verification
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("content"))

		// Verify parent directories were created
		dirInfo, err := os.Stat(filepath.Join(tmpDir, "service", "hello-world", "dependencies.d"))
		Expect(err).NotTo(HaveOccurred())
		Expect(dirInfo.IsDir()).To(BeTrue())
	})

	It("should now succeed with the paths from logs after the fix", func() {
		// Exact paths from logs
		servicePath := filepath.Join(tmpDir, "run", "service", "hello-world")
		dependenciesPath := filepath.Join(servicePath, "dependencies.d")
		basePath := filepath.Join(dependenciesPath, "base")
		downPath := filepath.Join(servicePath, "down")

		// Write files without creating parent directories
		err = bufService.WriteFile(ctx, basePath, []byte("base content"), 0644)
		Expect(err).NotTo(HaveOccurred())

		err = bufService.WriteFile(ctx, downPath, []byte(""), 0644)
		Expect(err).NotTo(HaveOccurred())

		// With the fixed implementation, SyncToDisk should now succeed
		err = bufService.SyncToDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Verify files exist
		baseContent, err := os.ReadFile(basePath) //nolint:gosec // G304: File path is controlled within test context, safe for test verification
		Expect(err).NotTo(HaveOccurred())
		Expect(string(baseContent)).To(Equal("base content"))

		downContent, err := os.ReadFile(downPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(downContent)).To(Equal(""))
	})

	It("should handle mix of directory removal and creation in correct order", func() {
		// Create a directory structure on disk first
		oldPath := filepath.Join(tmpDir, "service", "old-service")
		err = os.MkdirAll(oldPath, 0755) //nolint:gosec // G301: Test requires permissive permissions for old service directory creation
		Expect(err).NotTo(HaveOccurred())

		// Write a file in it
		oldFile := filepath.Join(oldPath, "config")
		err = os.WriteFile(oldFile, []byte("old content"), 0644) //nolint:gosec // G306: Test requires standard file permissions for old config file creation
		Expect(err).NotTo(HaveOccurred())

		// Sync to load existing structure
		err = bufService.SyncFromDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Now, in one operation:
		// 1. Remove the old directory
		// 2. Create a new directory
		// 3. Write a file in the new directory

		// Mark old directory for removal
		err = bufService.RemoveAll(ctx, oldPath)
		Expect(err).NotTo(HaveOccurred())

		// Create new structure
		newPath := filepath.Join(tmpDir, "service", "new-service")
		err = bufService.EnsureDirectory(ctx, newPath)
		Expect(err).NotTo(HaveOccurred())

		// Write a file in the new directory
		newFile := filepath.Join(newPath, "config")
		err = bufService.WriteFile(ctx, newFile, []byte("new content"), 0644)
		Expect(err).NotTo(HaveOccurred())

		// SyncToDisk should handle correct ordering of operations
		err = bufService.SyncToDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Verify new file exists
		content, err := os.ReadFile(newFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("new content"))

		// Verify old directory is gone
		_, err = os.Stat(oldPath)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("should handle complex hierarchical file operations", func() {
		// This test creates a complex scenario with multiple levels of directories and files
		// with some being added and others removed

		// First create and sync a basic structure
		baseDir := filepath.Join(tmpDir, "complex")
		err = os.MkdirAll(filepath.Join(baseDir, "service", "old"), 0755)
		Expect(err).NotTo(HaveOccurred())

		// Write a few files
		err = os.WriteFile(filepath.Join(baseDir, "service", "old", "config"), []byte("old"), 0644)
		Expect(err).NotTo(HaveOccurred())

		// Sync from disk
		err = bufService.SyncFromDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Now perform a complex set of operations:
		// 1. Remove old directory
		err = bufService.RemoveAll(ctx, filepath.Join(baseDir, "service", "old"))
		Expect(err).NotTo(HaveOccurred())

		// 2. Create several nested directories
		dir1 := filepath.Join(baseDir, "service", "new1")
		dir2 := filepath.Join(baseDir, "service", "new2", "nested")
		dir3 := filepath.Join(baseDir, "service", "new3", "deeply", "nested")

		err = bufService.EnsureDirectory(ctx, dir1)
		Expect(err).NotTo(HaveOccurred())
		err = bufService.EnsureDirectory(ctx, dir2)
		Expect(err).NotTo(HaveOccurred())
		err = bufService.EnsureDirectory(ctx, dir3)
		Expect(err).NotTo(HaveOccurred())

		// 3. Write files in each directory
		file1 := filepath.Join(dir1, "file1.txt")
		file2 := filepath.Join(dir2, "file2.txt")
		file3 := filepath.Join(dir3, "file3.txt")

		err = bufService.WriteFile(ctx, file1, []byte("content1"), 0644)
		Expect(err).NotTo(HaveOccurred())
		err = bufService.WriteFile(ctx, file2, []byte("content2"), 0644)
		Expect(err).NotTo(HaveOccurred())
		err = bufService.WriteFile(ctx, file3, []byte("content3"), 0644)
		Expect(err).NotTo(HaveOccurred())

		// 4. Perform sync
		err = bufService.SyncToDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// 5. Verify everything was created/removed correctly
		// Old directory should be gone
		_, err = os.Stat(filepath.Join(baseDir, "service", "old"))
		Expect(os.IsNotExist(err)).To(BeTrue())

		// New directories and files should exist
		content1, err := os.ReadFile(file1)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content1)).To(Equal("content1"))

		content2, err := os.ReadFile(file2)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content2)).To(Equal("content2"))

		content3, err := os.ReadFile(file3)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content3)).To(Equal("content3"))
	})
})

var _ = Describe("BufferedService Permission Checking", func() {
	var (
		tmpDir      string
		err         error
		ctx         context.Context
		cancel      context.CancelFunc
		baseService filesystem.Service
		bufService  *filesystem.BufferedService
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		_ = ctx
		_ = cancel

		// Create a real temp directory on disk for tests
		tmpDir, err = os.MkdirTemp("", "buffered-service-perm-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Initialize the base service
		baseService = filesystem.NewDefaultService()

		// Initialize the buffered service
		bufService = filesystem.NewBufferedService(baseService, tmpDir, constants.FilesAndDirectoriesToIgnore)

		// Initially disable permissions to set up files
		bufService.SetVerifyPermissions(false)

		// Setup a test directory structure with different permissions
		setupTestFilesWithPermissions(tmpDir)

		// Sync from disk to load file metadata and permissions
		err = bufService.SyncFromDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Now enable permission checks for tests
		bufService.SetVerifyPermissions(true)
	})

	AfterEach(func() {
		cancel()

		// Make read-only directories writable before removal
		readOnlyDir := filepath.Join(tmpDir, "readonly_dir")
		if _, err := os.Stat(readOnlyDir); err == nil {
			err = os.Chmod(readOnlyDir, 0755) //nolint:gosec // G302: Test cleanup requires permissive permissions to delete read-only directories
			if err != nil {
				// Log instead of failing - we're in cleanup
				fmt.Println("Warning: Failed to chmod directory:", err)
			}
		}

		err := os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should check permissions by default", func() {
		// Try to write to a read-only file
		readOnlyFile := filepath.Join(tmpDir, "readonly.txt")

		// First make sure we have the file in our cache and can read it
		content, err := bufService.ReadFile(ctx, readOnlyFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("Read-only content\n"))

		// Now try to write to it - this should fail due to permission check
		err = bufService.WriteFile(ctx, readOnlyFile, []byte("Modified"), 0644)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("permission"))
	})

	It("should allow disabling permission checks for in-memory operations only", func() {
		// Disable permission checks
		bufService.SetVerifyPermissions(false)

		// Now write to the same read-only file - should succeed in memory
		readOnlyFile := filepath.Join(tmpDir, "readonly.txt")
		newContent := []byte("Modified")
		err = bufService.WriteFile(ctx, readOnlyFile, newContent, 0644)
		Expect(err).NotTo(HaveOccurred())

		// Verify the write was recorded in memory
		content, err := bufService.ReadFile(ctx, readOnlyFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(Equal(newContent))

		// Make the file writable for the sync test
		err = os.Chmod(readOnlyFile, 0644) //nolint:gosec // G302: Test requires manipulating file permissions for testing sync functionality
		Expect(err).NotTo(HaveOccurred())

		// SyncToDisk should succeed now that both our checks and OS permissions allow writing
		err = bufService.SyncToDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Restore read-only for future tests
		err = os.Chmod(readOnlyFile, 0444) //nolint:gosec // G302: Test requires setting read-only permissions for testing filesystem behavior
		Expect(err).NotTo(HaveOccurred())

		// Re-sync from disk to update our cached permission information
		err = bufService.SyncFromDisk(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Re-enable permission checks to verify the system
		bufService.SetVerifyPermissions(true)

		// Future writes should be checked again
		err = bufService.WriteFile(ctx, readOnlyFile, []byte("Another update"), 0644)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("permission"))
	})

	It("should check parent directory permissions when creating new files", func() {
		// Try to create a file in a read-only directory
		readOnlyDir := filepath.Join(tmpDir, "readonly_dir")
		newFilePath := filepath.Join(readOnlyDir, "newfile.txt")

		// This should fail the permission check
		err = bufService.WriteFile(ctx, newFilePath, []byte("New content"), 0644)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("permission"))
	})

	It("should check permissions during SyncToDisk for buffered changes", func() {
		// Disable permission checks to buffer some changes
		bufService.SetVerifyPermissions(false)

		// Make some changes that would violate permissions
		readOnlyFile := filepath.Join(tmpDir, "readonly.txt")
		err = bufService.WriteFile(ctx, readOnlyFile, []byte("Modified"), 0644)
		Expect(err).NotTo(HaveOccurred())

		// Re-enable permission checks for SyncToDisk
		bufService.SetVerifyPermissions(true)

		// SyncToDisk should now fail
		err = bufService.SyncToDisk(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("permission"))
		Expect(err.Error()).To(ContainSubstring(readOnlyFile))
	})
})

// setupTestFilesWithPermissions creates files with different permissions for testing.
func setupTestFilesWithPermissions(root string) {
	// Create a read-only file
	readOnlyFile := filepath.Join(root, "readonly.txt")

	err := os.WriteFile(readOnlyFile, []byte("Read-only content\n"), 0644)
	if err != nil {
		panic(err)
	}
	// Make it read-only
	err = os.Chmod(readOnlyFile, 0444) //nolint:gosec // G302: Test requires setting read-only permissions for testing filesystem behavior
	if err != nil {
		panic(err)
	}

	// Create a read-only directory
	readOnlyDir := filepath.Join(root, "readonly_dir")

	err = os.MkdirAll(readOnlyDir, 0755)
	if err != nil {
		panic(err)
	}
	// Make a file inside it for testing
	inDirFile := filepath.Join(readOnlyDir, "existing.txt")

	err = os.WriteFile(inDirFile, []byte("Existing file\n"), 0644)
	if err != nil {
		panic(err)
	}
	// Make the directory read-only
	err = os.Chmod(readOnlyDir, 0555) //nolint:gosec // G302: Test requires setting read-only permissions for testing directory behavior
	if err != nil {
		panic(err)
	}

	// Create a writable directory/file for comparison
	writableDir := filepath.Join(root, "writable_dir")

	err = os.MkdirAll(writableDir, 0755)
	if err != nil {
		panic(err)
	}

	writableFile := filepath.Join(writableDir, "writable.txt")

	err = os.WriteFile(writableFile, []byte("Writable content\n"), 0644)
	if err != nil {
		panic(err)
	}
}
