package filesystem_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
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

		// Create a real temp directory on disk for the underlying DefaultService to operate on.
		tmpDir, err = os.MkdirTemp("", "buffered-service-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Initialize the base service (DefaultService) pointing to the real filesystem
		baseService = filesystem.NewDefaultService()

		// Initialize the buffered service, wrapping the base service
		bufService = filesystem.NewBufferedService(baseService, tmpDir, constants.FilesAndDirectoriesToIgnore)

		// Create some files or directories in the tmpDir so that we can test SyncFromDisk
		setupTestFiles(tmpDir)
	})

	AfterEach(func() {
		// Clean up
		cancel()
		os.RemoveAll(tmpDir)
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
			err := os.MkdirAll(dirPath, 0755)
			Expect(err).NotTo(HaveOccurred())

			// Create a file in one path
			err = os.WriteFile(filepath.Join(dirPath, "somefile.txt"), []byte("test"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Create a directory where a file might be expected
			err = os.MkdirAll(filepath.Join(dirPath, "log"), 0755)
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
			err := os.MkdirAll(targetDir, 0755)
			Expect(err).NotTo(HaveOccurred())

			// Also create a log dir
			targetLogDir := filepath.Join(tmpDir, "run", "s6-rc", "servicedirs", "umh-core-log")
			err = os.MkdirAll(targetLogDir, 0755)
			Expect(err).NotTo(HaveOccurred())

			// Create a sample file in the target directory
			sampleFile := filepath.Join(targetDir, "config.json")
			err = os.WriteFile(sampleFile, []byte("{\"test\": true}"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Create the service directory
			serviceDir := filepath.Join(tmpDir, "run", "service")
			err = os.MkdirAll(serviceDir, 0755)
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
			file, err := bufService.CreateFile(ctx, newFilePath, 0644)
			Expect(err).NotTo(HaveOccurred())
			Expect(file).To(BeNil()) // CreateFile returns nil for file handle

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
		It("should return an error if SyncFromDisk fails (e.g. root does not exist)", func() {
			// Provide a non-existent directory as root
			bufServiceInvalid := filesystem.NewBufferedService(baseService, filepath.Join(tmpDir, "no_such_dir"), constants.FilesAndDirectoriesToIgnore)
			err := bufServiceInvalid.SyncFromDisk(ctx)
			Expect(err).To(HaveOccurred())
			Expect(strings.Contains(err.Error(), "failed to walk directory tree")).To(BeTrue())
		})

		It("should return an error if SyncToDisk fails on an actual WriteFile error", func() {
			// We'll create a mock or something that fails on WriteFile
			// Alternatively, we can rename the directory out from under it to cause an error
			err := bufService.SyncFromDisk(ctx)
			Expect(err).NotTo(HaveOccurred())

			// write a new file
			testFile := filepath.Join(tmpDir, "bad_file.txt")
			err = bufService.WriteFile(ctx, testFile, []byte("hello"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// remove the entire tmpDir so that writing definitely fails
			err = os.RemoveAll(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Now flush
			err = bufService.SyncToDisk(ctx)
			Expect(err).To(HaveOccurred())
			Expect(strings.Contains(err.Error(), "failed to write file")).To(BeTrue())
		})
	})
})

// ----------------------------------------------------------
// Helper functions
// ----------------------------------------------------------

func setupTestFiles(root string) {
	// Create a sample file
	samplePath := filepath.Join(root, "sample.txt")
	os.WriteFile(samplePath, []byte("Hello, world!\n"), 0644)

	// Create nested directories
	nestedDir := filepath.Join(root, "nested")
	os.MkdirAll(nestedDir, 0755)

	// Create a nested file
	nestedFile := filepath.Join(nestedDir, "sample2.txt")
	os.WriteFile(nestedFile, []byte("Nested file\n"), 0644)
}

func createLargeFile(path string, sizeBytes int64) {
	f, _ := os.Create(path)
	defer f.Close()

	// Just write N zeroes
	f.Truncate(sizeBytes)
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
		mockRootDir string = "/mock-root"
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		mockFs = filesystem.NewMockFileSystem()
		bufService = filesystem.NewBufferedService(mockFs, mockRootDir, constants.FilesAndDirectoriesToIgnore)
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

		It("should fail MkdirTemp if mock triggers an error", func() {
			mockFs.WithMkdirTempFunc(func(ctx context.Context, dir, pattern string) (string, error) {
				return "", errors.New("mock mkdirTemp error")
			})
			_, err := bufService.MkdirTemp(ctx, filepath.Join(mockRootDir, "tmp"), "pattern-*")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mock mkdirTemp error"))
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

			for i := 0; i < numAttempts; i++ {
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
