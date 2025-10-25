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

package s6

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/cactus/tai64"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// GetLogsSince Test Coverage
// ═══════════════════════════
//
// This test suite provides comprehensive coverage for the new GetLogsSince() method,
// which returns only logs newer than a provided timestamp. This functionality enables
// incremental log collection for data freshness monitoring.
//
// Test Categories:
// ───────────────
//
// 1. **Basic Functionality**: Returns only logs after the given timestamp
// 2. **Edge Cases**:
//    - Empty log files
//    - No logs newer than timestamp (returns empty slice)
//    - All logs newer than timestamp (returns all)
//    - Exact timestamp match behavior (timestamp should be excluded)
//    - Very old timestamp (should return all logs)
//    - Future timestamp (should return no logs)
// 3. **Log Rotation Scenarios**: Handles rotated files correctly
// 4. **Error Handling**: File not found, permission errors, service not existing
// 5. **TAI64N Timestamp Parsing**: Validates correct timestamp parsing and filtering
//
// Expected Behavior:
// ─────────────────
// - Returns LogEntry array with only entries where entry.Timestamp > since
// - Maintains chronological order (oldest to newest)
// - Handles log rotation transparently (reads from rotated + current files)
// - Returns empty slice (not error) when no logs match criteria
// - Returns ErrServiceNotExist when service doesn't exist
// - Returns ErrLogFileNotFound when log file doesn't exist
var _ = Describe("GetLogsSince", func() {
	var (
		service     *DefaultService
		fsService   filesystem.Service
		tempDir     string
		serviceName string
		servicePath string
		logDir      string
		currentFile string
		ctx         context.Context
	)

	BeforeEach(func() {
		service = NewDefaultService().(*DefaultService)
		fsService = filesystem.NewMockFileSystem()
		ctx = context.Background()

		tempDir = GinkgoT().TempDir()
		serviceName = "test-service"

		serviceBaseDir := filepath.Join(tempDir, "run", "service")
		servicePath = filepath.Join(serviceBaseDir, serviceName)

		logDir = filepath.Join(constants.S6LogBaseDir, serviceName)
		currentFile = filepath.Join(logDir, "current")
	})

	// ══════════════════════════════════════════════════════════════════
	// Basic Functionality
	// ══════════════════════════════════════════════════════════════════

	Describe("Basic Functionality", func() {
		It("should return only logs after the given timestamp", func() {
			baseTime := time.Date(2025, 1, 20, 10, 0, 0, 0, time.UTC)
			sinceTime := time.Date(2025, 1, 20, 10, 0, 5, 0, time.UTC)

			logContent := "2025-01-20 10:00:00.000000000  message 1\n" +
				"2025-01-20 10:00:03.000000000  message 2\n" +
				"2025-01-20 10:00:05.000000000  message 3 (at boundary)\n" +
				"2025-01-20 10:00:07.000000000  message 4\n" +
				"2025-01-20 10:00:10.000000000  message 5\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(2))
			Expect(entries[0].Content).To(Equal("message 4"))
			Expect(entries[0].Timestamp).To(Equal(baseTime.Add(7 * time.Second)))
			Expect(entries[1].Content).To(Equal("message 5"))
			Expect(entries[1].Timestamp).To(Equal(baseTime.Add(10 * time.Second)))
		})

		It("should maintain chronological order", func() {
			baseTime := time.Date(2025, 1, 20, 10, 0, 0, 0, time.UTC)
			sinceTime := time.Date(2025, 1, 20, 9, 55, 0, 0, time.UTC)

			logContent := "2025-01-20 10:00:00.000000000  first\n" +
				"2025-01-20 10:00:01.000000000  second\n" +
				"2025-01-20 10:00:02.000000000  third\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(3))
			Expect(entries[0].Content).To(Equal("first"))
			Expect(entries[0].Timestamp).To(Equal(baseTime))
			Expect(entries[1].Content).To(Equal("second"))
			Expect(entries[1].Timestamp).To(Equal(baseTime.Add(1 * time.Second)))
			Expect(entries[2].Content).To(Equal("third"))
			Expect(entries[2].Timestamp).To(Equal(baseTime.Add(2 * time.Second)))
		})
	})

	// ══════════════════════════════════════════════════════════════════
	// Edge Cases
	// ══════════════════════════════════════════════════════════════════

	Describe("Edge Cases", func() {
		It("should return empty slice for empty log file", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 0, 0, time.UTC)
			logContent := ""

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})

		It("should return empty slice when no logs are newer than timestamp", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 10, 0, time.UTC)

			logContent := "2025-01-20 10:00:00.000000000  message 1\n" +
				"2025-01-20 10:00:05.000000000  message 2\n" +
				"2025-01-20 10:00:09.000000000  message 3\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})

		It("should return all logs when all are newer than timestamp", func() {
			sinceTime := time.Date(2025, 1, 20, 9, 0, 0, 0, time.UTC)

			logContent := "2025-01-20 10:00:00.000000000  message 1\n" +
				"2025-01-20 10:00:05.000000000  message 2\n" +
				"2025-01-20 10:00:10.000000000  message 3\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(3))
		})

		It("should exclude logs with exact timestamp match", func() {
			exactTime := time.Date(2025, 1, 20, 10, 0, 5, 0, time.UTC)

			logContent := "2025-01-20 10:00:00.000000000  before\n" +
				"2025-01-20 10:00:05.000000000  exact match\n" +
				"2025-01-20 10:00:10.000000000  after\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, exactTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("after"))
		})

		It("should return all logs when timestamp is very old", func() {
			veryOldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

			logContent := "2025-01-20 10:00:00.000000000  message 1\n" +
				"2025-01-20 10:00:05.000000000  message 2\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, veryOldTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(2))
		})

		It("should return empty slice when timestamp is in the future", func() {
			futureTime := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

			logContent := "2025-01-20 10:00:00.000000000  message 1\n" +
				"2025-01-20 10:00:05.000000000  message 2\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, futureTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})

		It("should handle zero timestamp (return all logs)", func() {
			zeroTime := time.Time{}

			logContent := "2025-01-20 10:00:00.000000000  message 1\n" +
				"2025-01-20 10:00:05.000000000  message 2\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, zeroTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(2))
		})
	})

	// ══════════════════════════════════════════════════════════════════
	// Log Rotation Scenarios
	// ══════════════════════════════════════════════════════════════════

	Describe("Log Rotation Scenarios", func() {
		It("should filter logs from both rotated and current files", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 5, 0, time.UTC)

			rotatedTime := time.Now().Add(-1 * time.Minute)
			rotatedFileName := tai64.FormatNano(rotatedTime) + ".s"
			rotatedFile := filepath.Join(logDir, rotatedFileName)

			rotatedContent := "2025-01-20 10:00:00.000000000  rotated 1\n" +
				"2025-01-20 10:00:03.000000000  rotated 2\n" +
				"2025-01-20 10:00:06.000000000  rotated 3\n"

			currentContent := "2025-01-20 10:00:08.000000000  current 1\n" +
				"2025-01-20 10:00:10.000000000  current 2\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile || path == rotatedFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(currentContent)),
						inode: 54321,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(currentContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				if path == rotatedFile {
					content := []byte(rotatedContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{rotatedFile}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(3))
			Expect(entries[0].Content).To(Equal("rotated 3"))
			Expect(entries[1].Content).To(Equal("current 1"))
			Expect(entries[2].Content).To(Equal("current 2"))
		})

		It("should handle rotation at timestamp boundary", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 5, 0, time.UTC)

			rotatedTime := time.Now().Add(-1 * time.Minute)
			rotatedFileName := tai64.FormatNano(rotatedTime) + ".s"
			rotatedFile := filepath.Join(logDir, rotatedFileName)

			rotatedContent := "2025-01-20 10:00:00.000000000  before rotation\n" +
				"2025-01-20 10:00:05.000000000  at boundary\n"

			currentContent := "2025-01-20 10:00:06.000000000  after rotation\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile || path == rotatedFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(currentContent)),
						inode: 54321,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(currentContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				if path == rotatedFile {
					content := []byte(rotatedContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{rotatedFile}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("after rotation"))
		})

		It("should handle multiple rotated files", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 5, 0, time.UTC)

			baseRotatedTime := time.Now().Add(-2 * time.Minute)
			rotatedFileName1 := tai64.FormatNano(baseRotatedTime) + ".s"
			rotatedFileName2 := tai64.FormatNano(baseRotatedTime.Add(1 * time.Minute)) + ".s"
			rotatedFile1 := filepath.Join(logDir, rotatedFileName1)
			rotatedFile2 := filepath.Join(logDir, rotatedFileName2)

			rotatedContent1 := "2025-01-20 10:00:01.000000000  rotated 1\n"
			rotatedContent2 := "2025-01-20 10:00:04.000000000  rotated 2\n" +
				"2025-01-20 10:00:06.000000000  rotated 3\n"

			currentContent := "2025-01-20 10:00:08.000000000  current\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile || path == rotatedFile1 || path == rotatedFile2, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(currentContent)),
						inode: 54321,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(currentContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				if path == rotatedFile2 {
					content := []byte(rotatedContent2)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				if path == rotatedFile1 {
					content := []byte(rotatedContent1)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{rotatedFile1, rotatedFile2}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(2))
			Expect(entries[0].Content).To(Equal("rotated 3"))
			Expect(entries[1].Content).To(Equal("current"))
		})
	})

	// ══════════════════════════════════════════════════════════════════
	// Error Handling
	// ══════════════════════════════════════════════════════════════════

	Describe("Error Handling", func() {
		It("should return ErrServiceNotExist when service doesn't exist", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 0, 0, time.UTC)

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return false, nil
			})

			_, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).To(Equal(ErrServiceNotExist))
		})

		It("should return ErrLogFileNotFound when log file doesn't exist", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 0, 0, time.UTC)

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if path == servicePath {
					return true, nil
				}
				return false, nil
			})

			_, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).To(Equal(ErrLogFileNotFound))
		})

		It("should handle context cancellation", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 0, 0, time.UTC)
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return false, context.Canceled
			})

			_, err := service.GetLogsSince(cancelledCtx, servicePath, fsService, sinceTime)
			Expect(err).To(HaveOccurred())
		})

		It("should handle permission errors gracefully", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 0, 0, time.UTC)

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if path == servicePath {
					return true, nil
				}
				if path == currentFile {
					return true, nil
				}
				return false, nil
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				return nil, 0, os.ErrPermission
			})

			_, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).To(HaveOccurred())
		})
	})

	// ══════════════════════════════════════════════════════════════════
	// TAI64N Timestamp Parsing
	// ══════════════════════════════════════════════════════════════════

	Describe("TAI64N Timestamp Parsing", func() {
		It("should correctly parse and filter nanosecond precision timestamps", func() {
			baseTime := time.Date(2025, 1, 20, 10, 0, 5, 0, time.UTC)
			sinceTime := time.Date(2025, 1, 20, 10, 0, 5, 500000000, time.UTC)

			logContent := "2025-01-20 10:00:05.000000000  before nanos\n" +
				"2025-01-20 10:00:05.500000000  at boundary\n" +
				"2025-01-20 10:00:05.750000000  after nanos\n" +
				"2025-01-20 10:00:06.000000000  next second\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(2))
			Expect(entries[0].Content).To(Equal("after nanos"))
			Expect(entries[0].Timestamp).To(Equal(baseTime.Add(750 * time.Millisecond)))
			Expect(entries[1].Content).To(Equal("next second"))
			Expect(entries[1].Timestamp).To(Equal(baseTime.Add(1 * time.Second)))
		})

		It("should handle logs without valid timestamps", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 0, 0, time.UTC)

			logContent := "2025-01-20 10:00:01.000000000  valid log\n" +
				"invalid log entry without timestamp\n" +
				"2025-01-20 10:00:05.000000000  another valid log\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(2))
			Expect(entries[0].Content).To(Equal("valid log"))
			Expect(entries[1].Content).To(Equal("another valid log"))
		})

		It("should handle timezone differences correctly (all times in UTC)", func() {
			sinceTime := time.Date(2025, 1, 20, 10, 0, 5, 0, time.UTC)

			logContent := "2025-01-20 10:00:00.000000000  message 1\n" +
				"2025-01-20 10:00:07.000000000  message 2\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entries, err := service.GetLogsSince(ctx, servicePath, fsService, sinceTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("message 2"))
			Expect(entries[0].Timestamp.Location()).To(Equal(time.UTC))
		})
	})

	// ══════════════════════════════════════════════════════════════════
	// Backward Compatibility
	// ══════════════════════════════════════════════════════════════════

	Describe("Backward Compatibility", func() {
		It("should not affect existing GetLogs() behavior", func() {
			logContent := "2025-01-20 10:00:00.000000000  message 1\n" +
				"2025-01-20 10:00:05.000000000  message 2\n"

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == servicePath || path == currentFile, nil
			})
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == currentFile {
					return &mockFileInfoWithSys{
						name:  "current",
						size:  int64(len(logContent)),
						inode: 12345,
					}, nil
				}
				return nil, os.ErrNotExist
			})
			mockFS.WithReadFileRangeFunc(func(ctx context.Context, path string, from int64) ([]byte, int64, error) {
				if path == currentFile {
					content := []byte(logContent)
					if from >= int64(len(content)) {
						return nil, int64(len(content)), nil
					}
					return content[from:], int64(len(content)), nil
				}
				return nil, 0, os.ErrNotExist
			})
			mockFS.WithGlobFunc(func(ctx context.Context, pattern string) ([]string, error) {
				return []string{}, nil
			})

			entriesOld, errOld := service.GetLogs(ctx, servicePath, fsService)
			Expect(errOld).ToNot(HaveOccurred())
			Expect(entriesOld).To(HaveLen(2))
		})
	})
})
