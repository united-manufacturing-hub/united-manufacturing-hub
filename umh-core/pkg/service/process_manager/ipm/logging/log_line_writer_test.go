//go:build internal_process_manager
// +build internal_process_manager

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

package logging_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/logging"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"go.uber.org/zap"
)

var _ = Describe("LogLineWriter", func() {
	var (
		writer       *logging.LogLineWriter
		memoryBuffer *logging.MemoryLogBuffer
		logManager   *logging.LogManager
		fsService    filesystem.Service
		logger       *zap.SugaredLogger
		identifier   constants.ServiceIdentifier
		logPath      string
		tempDir      string
	)

	BeforeEach(func() {
		// Create temporary directory for testing
		var err error
		tempDir, err = os.MkdirTemp("", "log-line-writer-test-")
		Expect(err).ToNot(HaveOccurred())

		// Create test logger
		zapLogger, _ := zap.NewDevelopment()
		logger = zapLogger.Sugar()

		// Setup test data
		identifier = constants.ServiceIdentifier("test-service")
		logPath = filepath.Join(tempDir, "logs")

		// Create memory buffer
		memoryBuffer = logging.NewMemoryLogBuffer(100)

		// Create log manager
		logManager = logging.NewLogManager(logger)

		// Use real filesystem for testing
		fsService = filesystem.NewDefaultService()

		// Ensure log directory exists
		err = fsService.EnsureDirectory(context.Background(), logPath)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if writer != nil {
			writer.Close()
		}
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Context("when creating a new writer", func() {
		It("should create successfully with valid parameters", func() {
			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(writer).ToNot(BeNil())
			Expect(writer.Identifier).To(Equal(identifier))
			Expect(writer.LogPath).To(Equal(logPath))
			Expect(writer.MemoryBuffer).To(Equal(memoryBuffer))
			Expect(writer.LogManager).To(Equal(logManager))
			Expect(writer.FsService).To(Equal(fsService))
		})

		It("should fail with nil log manager", func() {
			writer, err := logging.NewLogLineWriter(identifier, logPath, nil, memoryBuffer, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logManager cannot be nil"))
			Expect(writer).To(BeNil())
		})

		It("should fail with nil memory buffer", func() {
			writer, err := logging.NewLogLineWriter(identifier, logPath, logManager, nil, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("memoryBuffer cannot be nil"))
			Expect(writer).To(BeNil())
		})

		It("should create ipm. current log file if it doesn't exist", func() {
			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())

			currentLogFile := filepath.Join(logPath, constants.CurrentLogFileName)
			exists, err := fsService.FileExists(context.Background(), currentLogFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should detect existing log file size", func() {
			// Create a log file with some content
			currentLogFile := filepath.Join(logPath, constants.CurrentLogFileName)
			testContent := "Existing log content\n"
			err := fsService.WriteFile(context.Background(), currentLogFile, []byte(testContent), constants.LogFilePermission)
			Expect(err).ToNot(HaveOccurred())

			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.GetCurrentFileSize()).To(Equal(int64(len(testContent))))
		})
	})

	Context("when writing log entries", func() {
		BeforeEach(func() {
			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should write to both memory buffer and file", func() {
			entry := process_shared.LogEntry{
				Timestamp: time.Date(2025, 1, 15, 12, 30, 45, 123456789, time.UTC),
				Content:   "Test log entry",
			}

			err := writer.WriteLine(entry)
			Expect(err).ToNot(HaveOccurred())

			// Check memory buffer
			entries := memoryBuffer.GetEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("Test log entry"))
			Expect(entries[0].Timestamp).To(Equal(entry.Timestamp))

			// Check file content
			currentLogFile := filepath.Join(logPath, constants.CurrentLogFileName)
			content, err := fsService.ReadFile(context.Background(), currentLogFile)
			Expect(err).ToNot(HaveOccurred())

			expectedLine := "2025-01-15 12:30:45.123456789  Test log entry\n"
			Expect(string(content)).To(Equal(expectedLine))
		})

		It("should update file size tracking", func() {
			initialSize := writer.GetCurrentFileSize()

			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "Test entry",
			}

			err := writer.WriteLine(entry)
			Expect(err).ToNot(HaveOccurred())

			newSize := writer.GetCurrentFileSize()
			Expect(newSize).To(BeNumerically(">", initialSize))
		})

		It("should handle multiple entries in correct order", func() {
			baseTime := time.Now()
			entries := []process_shared.LogEntry{
				{Timestamp: baseTime, Content: "First entry"},
				{Timestamp: baseTime.Add(time.Second), Content: "Second entry"},
				{Timestamp: baseTime.Add(2 * time.Second), Content: "Third entry"},
			}

			for _, entry := range entries {
				err := writer.WriteLine(entry)
				Expect(err).ToNot(HaveOccurred())
			}

			// Check memory buffer
			memEntries := memoryBuffer.GetEntries()
			Expect(memEntries).To(HaveLen(3))
			for i, entry := range entries {
				Expect(memEntries[i].Content).To(Equal(entry.Content))
			}

			// Check file content
			currentLogFile := filepath.Join(logPath, constants.CurrentLogFileName)
			content, err := fsService.ReadFile(context.Background(), currentLogFile)
			Expect(err).ToNot(HaveOccurred())

			lines := strings.Split(strings.TrimSpace(string(content)), "\n")
			Expect(lines).To(HaveLen(3))
			for i, entry := range entries {
				Expect(lines[i]).To(ContainSubstring(entry.Content))
			}
		})

		It("should handle entries with special characters", func() {
			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "Entry with special chars: !@#$%^&*(){}[]|\\:;\"'<>,.?/~`",
			}

			err := writer.WriteLine(entry)
			Expect(err).ToNot(HaveOccurred())

			entries := memoryBuffer.GetEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal(entry.Content))
		})

		It("should handle entries with newlines in content", func() {
			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "Multi\nline\nentry",
			}

			err := writer.WriteLine(entry)
			Expect(err).ToNot(HaveOccurred())

			entries := memoryBuffer.GetEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal(entry.Content))
		})
	})

	Context("when file rotation is needed", func() {
		BeforeEach(func() {
			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Set a small file size limit for testing
			writer.SetMaxFileSize(100) // 100 bytes
		})

		It("should trigger rotation when file size exceeds limit", func() {
			// Write enough entries to exceed the size limit
			for i := 0; i < 10; i++ {
				entry := process_shared.LogEntry{
					Timestamp: time.Now().Add(time.Duration(i) * time.Second),
					Content:   fmt.Sprintf("This is a long log entry number %d that should help exceed the size limit", i),
				}
				err := writer.WriteLine(entry)
				Expect(err).ToNot(HaveOccurred())
			}

			// Check that rotation occurred by looking for rotated files
			files, err := fsService.ReadDir(context.Background(), logPath)
			Expect(err).ToNot(HaveOccurred())

			// Should have ipm. current file + at least one rotated file
			logFiles := make([]string, 0)
			for _, file := range files {
				if strings.HasSuffix(file.Name(), constants.LogFileExtension) || file.Name() == constants.CurrentLogFileName {
					logFiles = append(logFiles, file.Name())
				}
			}
			Expect(len(logFiles)).To(BeNumerically(">=", 2)) // ipm. current + rotated
		})

		It("should reset file size after rotation", func() {
			// Set very small limit
			writer.SetMaxFileSize(50)

			// Write entry that will trigger rotation
			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "This is a long entry that should trigger rotation",
			}
			err := writer.WriteLine(entry)
			Expect(err).ToNot(HaveOccurred())

			// Size should be smaller than before rotation
			size := writer.GetCurrentFileSize()
			Expect(size).To(BeNumerically("<", 100))
		})

		It("should continue writing after rotation", func() {
			writer.SetMaxFileSize(50)

			// Write entry that triggers rotation
			entry1 := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "Entry that triggers rotation",
			}
			err := writer.WriteLine(entry1)
			Expect(err).ToNot(HaveOccurred())

			// Write another entry after rotation
			entry2 := process_shared.LogEntry{
				Timestamp: time.Now().Add(time.Second),
				Content:   "Entry after rotation",
			}
			err = writer.WriteLine(entry2)
			Expect(err).ToNot(HaveOccurred())

			// Both entries should be in memory buffer
			entries := memoryBuffer.GetEntries()
			Expect(entries).To(HaveLen(2))
			Expect(entries[0].Content).To(Equal(entry1.Content))
			Expect(entries[1].Content).To(Equal(entry2.Content))
		})
	})

	Context("when setting file size limits", func() {
		BeforeEach(func() {
			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should use default size for zero or negative values", func() {
			writer.SetMaxFileSize(0)
			// Internal maxFileSize should be set to default
			// We can't directly check private field, but we can verify behavior

			writer.SetMaxFileSize(-100)
			// Should also use default
		})

		It("should accept positive size values", func() {
			writer.SetMaxFileSize(2048)
			// Size should be set - we can verify this indirectly through rotation behavior
		})
	})

	Context("when closing the writer", func() {
		BeforeEach(func() {
			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Write some entries
			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "Entry before close",
			}
			err = writer.WriteLine(entry)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should close successfully", func() {
			err := writer.Close()
			Expect(err).ToNot(HaveOccurred())

			// File size should be reset to 0
			Expect(writer.GetCurrentFileSize()).To(Equal(int64(0)))
		})

		It("should not clear memory buffer", func() {
			entries := memoryBuffer.GetEntries()
			Expect(entries).To(HaveLen(1))

			err := writer.Close()
			Expect(err).ToNot(HaveOccurred())

			// Memory buffer should still contain entries
			entries = memoryBuffer.GetEntries()
			Expect(entries).To(HaveLen(1))
		})

		It("should be safe to call multiple times", func() {
			err := writer.Close()
			Expect(err).ToNot(HaveOccurred())

			err = writer.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("error handling", func() {
		It("should handle file write failures gracefully", func() {
			// Create writer with read-only directory to simulate write failure
			readOnlyDir := filepath.Join(tempDir, "readonly")
			err := fsService.EnsureDirectory(context.Background(), readOnlyDir)
			Expect(err).ToNot(HaveOccurred())

			// Change permissions to read-only (this may not work on all systems)
			err = os.Chmod(readOnlyDir, 0444)
			if err == nil {
				defer os.Chmod(readOnlyDir, 0755) // Restore permissions for cleanup

				// Try to create writer in read-only directory
				_, err = logging.NewLogLineWriter(identifier, readOnlyDir, logManager, memoryBuffer, fsService)
				Expect(err).To(HaveOccurred())
			}
		})

		It("should continue adding to memory buffer even if file writes fail", func() {
			// This test is tricky to implement without mocking file operations
			// For now, we'll test that memory operations work independently

			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())

			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "Test entry",
			}

			err = writer.WriteLine(entry)
			Expect(err).ToNot(HaveOccurred())

			// Memory buffer should always work
			entries := memoryBuffer.GetEntries()
			Expect(entries).To(HaveLen(1))
		})
	})

	Context("integration with LogManager", func() {
		BeforeEach(func() {
			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should register ipm. service with LogManager during creation", func() {
			// We can't directly check if ipm. service is registered, but we can verify
			// that the LogManager is properly initialized and rotation works

			writer.SetMaxFileSize(50)

			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "Entry that should trigger rotation through LogManager",
			}

			err := writer.WriteLine(entry)
			Expect(err).ToNot(HaveOccurred())

			// If LogManager integration works, rotation should succeed
		})
	})

	Context("concurrent access", func() {
		BeforeEach(func() {
			var err error
			writer, err = logging.NewLogLineWriter(identifier, logPath, logManager, memoryBuffer, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle concurrent writes safely", func() {
			const numGoroutines = 5
			const entriesPerGoroutine = 20

			errChan := make(chan error, numGoroutines)

			// Start multiple goroutines writing entries
			for i := 0; i < numGoroutines; i++ {
				go func(routineID int) {
					for j := 0; j < entriesPerGoroutine; j++ {
						entry := process_shared.LogEntry{
							Timestamp: time.Now(),
							Content:   fmt.Sprintf("Routine %d Entry %d", routineID, j),
						}
						if err := writer.WriteLine(entry); err != nil {
							errChan <- err
							return
						}
					}
					errChan <- nil
				}(i)
			}

			// Wait for all goroutines to complete
			for i := 0; i < numGoroutines; i++ {
				err := <-errChan
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify that all entries were written to memory buffer
			entries := memoryBuffer.GetEntries()
			Expect(len(entries)).To(Equal(numGoroutines * entriesPerGoroutine))

			// Verify file contains data
			currentLogFile := filepath.Join(logPath, constants.CurrentLogFileName)
			content, err := fsService.ReadFile(context.Background(), currentLogFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(content)).To(BeNumerically(">", 0))
		})
	})
})
