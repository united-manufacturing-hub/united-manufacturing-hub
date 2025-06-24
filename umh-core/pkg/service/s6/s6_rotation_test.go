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
	"fmt"
	"path/filepath"
	"time"

	"github.com/cactus/tai64"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// S6 Log Rotation Test Coverage
// ═════════════════════════════
//
// This test suite covers the simplified log rotation functionality that prevents
// protobuf message loss during topic browser output parsing. The tests verify
// the streamlined rotation approach where rotated file content is immediately
// read and combined with current file content.
//
// Test Categories:
// ───────────────
//
// 1. **findLatestRotatedFile**: Discovery of the most recent rotated file by TAI64N timestamp
//   - Normal: Finding the latest rotated file among multiple candidates
//   - Edge: No rotated files, malformed timestamps, empty directory
//
// 2. **appendToRingBuffer**: Ring buffer management for log entries
//   - Normal: Appending to empty buffer, buffer growth
//   - Edge: Buffer overflow/wrapping, multiple wrapping cycles
//
// 3. **GetLogs Integration**: Full rotation scenario testing
//   - Normal: Reading combined rotated + current content
//   - Edge: Rotation without rotated file, chronological ordering
//
// Simplified Approach Benefits:
// ────────────────────────────
// - No complex inode tracking across calls
// - Immediate handling of rotated content using timestamps
// - Guaranteed chronological order
// - Robust error handling with graceful fallbacks
var _ = Describe("S6 Log Rotation", func() {
	var (
		service     *DefaultService
		fsService   filesystem.Service
		tempDir     string
		logDir      string
		serviceName string
		ctx         context.Context
	)

	BeforeEach(func() {
		service = NewDefaultService().(*DefaultService)
		fsService = filesystem.NewDefaultService()
		ctx = context.Background()

		// Create temporary directory structure that matches S6 expectations
		tempDir = GinkgoT().TempDir()
		serviceName = "test-service"

		// GetLogs uses constants.S6LogBaseDir to find logs, so we need to set up the correct structure
		// For tests, we'll use the temp directory structure
		logDir = filepath.Join(tempDir, "data", "logs", serviceName)

		err := fsService.EnsureDirectory(ctx, logDir)
		Expect(err).ToNot(HaveOccurred())
	})

	// ══════════════════════════════════════════════════════════════════
	// findLatestRotatedFile: TAI64N Timestamp-based Rotated File Discovery
	// ══════════════════════════════════════════════════════════════════
	//
	// Tests the discovery of the most recent rotated file by parsing TAI64N
	// timestamps in filenames. This approach is more reliable than inode
	// matching and leverages S6's existing naming convention.
	//
	// Normal Cases:
	// • Finding the latest rotated file among multiple candidates
	// • Parsing TAI64N timestamps correctly from filenames
	//
	// Edge Cases:
	// • No rotated files exist - returns empty string gracefully
	// • Malformed filenames - skips them and continues
	// • Directory read errors - returns empty string
	//
	Describe("findLatestRotatedFile", func() {
		It("should return empty string when no rotated files exist", func() {
			result := service.findLatestRotatedFile(ctx, logDir, fsService)
			Expect(result).To(BeEmpty())
		})

		It("should return empty string when no valid TAI64N files exist", func() {
			// Create files that don't match the @TAI64N pattern
			invalidFiles := []string{"current", "lock", "invalid@file", "@invalid"}
			for _, fileName := range invalidFiles {
				filePath := filepath.Join(logDir, fileName)
				err := fsService.WriteFile(ctx, filePath, []byte("content"), 0644)
				Expect(err).ToNot(HaveOccurred())
			}

			result := service.findLatestRotatedFile(ctx, logDir, fsService)
			Expect(result).To(BeEmpty())
		})

		It("should find the latest rotated file among multiple candidates", func() {
			// Create multiple rotated files with different timestamps
			baseTime := time.Now().Add(-1 * time.Hour)

			// Create files at different times (older to newer)
			times := []time.Time{
				baseTime.Add(-30 * time.Minute), // Oldest
				baseTime.Add(-20 * time.Minute),
				baseTime.Add(-10 * time.Minute), // Newest
			}

			var files []string
			for i, t := range times {
				fileName := tai64.FormatNano(t) + ".s"
				filePath := filepath.Join(logDir, fileName)
				content := fmt.Sprintf("content_%d", i)
				err := fsService.WriteFile(ctx, filePath, []byte(content), 0644)
				Expect(err).ToNot(HaveOccurred())
				files = append(files, filePath)
			}

			// Should return the newest file (last one created)
			result := service.findLatestRotatedFile(ctx, logDir, fsService)
			Expect(result).To(Equal(files[2])) // The newest file
		})

		It("should handle mix of valid and invalid files", func() {
			// Create some invalid files
			invalidFiles := []string{"current", "@invalid", "regular.log"}
			for _, fileName := range invalidFiles {
				filePath := filepath.Join(logDir, fileName)
				err := fsService.WriteFile(ctx, filePath, []byte("content"), 0644)
				Expect(err).ToNot(HaveOccurred())
			}

			// Create one valid rotated file
			validTime := time.Now().Add(-5 * time.Minute)
			validFileName := tai64.FormatNano(validTime) + ".s"
			validFilePath := filepath.Join(logDir, validFileName)
			err := fsService.WriteFile(ctx, validFilePath, []byte("valid content"), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Should find the valid file despite invalid ones
			result := service.findLatestRotatedFile(ctx, logDir, fsService)
			Expect(result).To(Equal(validFilePath))
		})

		It("should handle directory read errors gracefully", func() {
			// Try to read from non-existent directory
			nonExistentDir := filepath.Join(tempDir, "non-existent")
			result := service.findLatestRotatedFile(ctx, nonExistentDir, fsService)
			Expect(result).To(BeEmpty())
		})
	})

	// ══════════════════════════════════════════════════════════════════
	// appendToRingBuffer: Ring Buffer Management
	// ══════════════════════════════════════════════════════════════════
	//
	// Tests the ring buffer logic that maintains the last N log entries
	// efficiently without memory reallocation or data shifting.
	//
	// Normal Cases:
	// • Empty buffer initialization and growth
	// • Sequential entry appending with correct head pointer management
	//
	// Edge Cases (tested implicitly in GetLogs integration):
	// • Buffer overflow/wrapping when reaching S6MaxLines limit
	// • Chronological ordering preservation across rotations
	// • Memory efficiency (single allocation per log file)
	//
	// Note: Full ring buffer wrapping is tested in GetLogs integration
	// scenarios, not isolated unit tests due to the large S6MaxLines constant.
	//
	Describe("appendToRingBuffer", func() {
		It("should append entries to empty ring buffer", func() {
			st := &logState{}
			entries := []LogEntry{
				{Timestamp: time.Now(), Content: "test1"},
				{Timestamp: time.Now(), Content: "test2"},
			}

			service.appendToRingBuffer(entries, st)

			Expect(st.logs).To(HaveLen(2))
			Expect(st.logs[0].Content).To(Equal("test1"))
			Expect(st.logs[1].Content).To(Equal("test2"))
			Expect(st.full).To(BeFalse())
		})

		It("should handle ring buffer wrapping correctly", func() {
			st := &logState{}

			// Use the actual S6MaxLines constant (10000)
			maxLines := constants.S6MaxLines

			// Create more entries than the ring buffer can hold
			totalEntries := maxLines + 50 // Exceed capacity by 50
			entries := make([]LogEntry, totalEntries)

			// Create entries with sequential timestamps and identifiable content
			baseTime := time.Now()
			for i := 0; i < totalEntries; i++ {
				entries[i] = LogEntry{
					Timestamp: baseTime.Add(time.Duration(i) * time.Second),
					Content:   fmt.Sprintf("entry_%d", i),
				}
			}

			// Append all entries to trigger wrapping
			service.appendToRingBuffer(entries, st)

			// Verify ring buffer is marked as full
			Expect(st.full).To(BeTrue(), "Ring buffer should be marked as full after wrapping")

			// Verify we only kept the maximum number of entries
			Expect(len(st.logs)).To(Equal(maxLines), "Ring buffer should contain exactly maxLines entries")

			// Verify head pointer wrapped correctly
			expectedHead := totalEntries % maxLines // Should be 50 for our test
			Expect(st.head).To(Equal(expectedHead), "Head pointer should wrap correctly")

			// The ring buffer should contain the LAST maxLines entries
			// Due to wrapping, the oldest entry in the buffer should be at index st.head
			// and the newest should be at index (st.head - 1 + maxLines) % maxLines

			// Find the oldest entry that should be in the buffer
			oldestKeptIndex := totalEntries - maxLines // Should be 50
			newestKeptIndex := totalEntries - 1        // Should be maxLines + 49

			// The entry at st.head should be the oldest kept entry
			oldestInBuffer := st.logs[st.head]
			Expect(oldestInBuffer.Content).To(Equal(fmt.Sprintf("entry_%d", oldestKeptIndex)),
				"Oldest entry in ring buffer should be correct")

			// The entry just before st.head should be the newest entry
			newestIndex := (st.head - 1 + maxLines) % maxLines
			newestInBuffer := st.logs[newestIndex]
			Expect(newestInBuffer.Content).To(Equal(fmt.Sprintf("entry_%d", newestKeptIndex)),
				"Newest entry in ring buffer should be correct")

			// Verify chronological ordering is maintained in the buffer
			// All entries should be sequential when read in ring order
			for i := 0; i < maxLines; i++ {
				bufferIndex := (st.head + i) % maxLines
				expectedEntryIndex := oldestKeptIndex + i
				expectedContent := fmt.Sprintf("entry_%d", expectedEntryIndex)

				Expect(st.logs[bufferIndex].Content).To(Equal(expectedContent),
					"Entry at buffer position %d should have correct content", bufferIndex)
			}
		})

		It("should handle exact ring buffer capacity without wrapping", func() {
			st := &logState{}
			maxLines := constants.S6MaxLines

			// Create exactly maxLines entries (should fill but not wrap)
			entries := make([]LogEntry, maxLines)
			for i := 0; i < maxLines; i++ {
				entries[i] = LogEntry{
					Timestamp: time.Now().Add(time.Duration(i) * time.Second),
					Content:   fmt.Sprintf("exact_entry_%d", i),
				}
			}

			service.appendToRingBuffer(entries, st)

			// Should be full but head should point to 0 (ready for next write)
			Expect(st.full).To(BeTrue(), "Buffer should be full at exact capacity")
			Expect(st.head).To(Equal(0), "Head should be at 0 when exactly full")
			Expect(len(st.logs)).To(Equal(maxLines), "Should contain exactly maxLines entries")

			// All entries should be in order from 0 to maxLines-1
			for i := 0; i < maxLines; i++ {
				Expect(st.logs[i].Content).To(Equal(fmt.Sprintf("exact_entry_%d", i)),
					"Entry at position %d should be correct", i)
			}
		})

		It("should handle multiple wrapping cycles", func() {
			st := &logState{}
			maxLines := constants.S6MaxLines

			// First fill: exactly maxLines entries
			firstBatch := make([]LogEntry, maxLines)
			for i := 0; i < maxLines; i++ {
				firstBatch[i] = LogEntry{Content: fmt.Sprintf("first_%d", i)}
			}
			service.appendToRingBuffer(firstBatch, st)

			// Second fill: another maxLines entries (complete wrap)
			secondBatch := make([]LogEntry, maxLines)
			for i := 0; i < maxLines; i++ {
				secondBatch[i] = LogEntry{Content: fmt.Sprintf("second_%d", i)}
			}
			service.appendToRingBuffer(secondBatch, st)

			// Should still be full, head should be back to 0
			Expect(st.full).To(BeTrue())
			Expect(st.head).To(Equal(0))
			Expect(len(st.logs)).To(Equal(maxLines))

			// Buffer should contain only second batch entries
			for i := 0; i < maxLines; i++ {
				Expect(st.logs[i].Content).To(Equal(fmt.Sprintf("second_%d", i)),
					"After complete wrap, should contain second batch at position %d", i)
			}
		})
	})

	// ══════════════════════════════════════════════════════════════════
	// GetLogs Integration: Simplified Rotation Handling
	// ══════════════════════════════════════════════════════════════════
	//
	// Tests the integrated rotation behavior where rotated file content is
	// immediately read and combined with current file content to maintain
	// chronological order without complex state tracking.
	//
	// Normal Cases:
	// • Reading combined rotated + current content in correct order
	// • Graceful handling when no rotated file is found
	// • Proper inode and offset tracking across calls
	//
	// Edge Cases:
	// • No matching rotated file (fallback to current only)
	// • Empty rotated or current files
	// • Invalid log content parsing
	//
	// Benefits of Simplified Approach:
	// • No persistent state tracking between calls
	// • Immediate data recovery on rotation
	// • Guaranteed chronological ordering
	// • Robust error handling with graceful fallbacks
	//
	Describe("GetLogs Rotation Integration", func() {
		var servicePath string

		BeforeEach(func() {
			// GetLogs expects the service to exist in the constants.S6BaseDir structure
			serviceBaseDir := filepath.Join(tempDir, "run", "service")
			servicePath = filepath.Join(serviceBaseDir, serviceName)
			err := fsService.EnsureDirectory(ctx, servicePath)
			Expect(err).ToNot(HaveOccurred())

			// Also need to set up the log directory where GetLogs will look
			// Since we can't easily change constants.S6LogBaseDir, we'll create the expected structure
			actualLogDir := filepath.Join(constants.S6LogBaseDir, serviceName)
			err = fsService.EnsureDirectory(ctx, actualLogDir)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle first call with no rotation", func() {
			// Create initial log file in the actual log directory that GetLogs will check
			actualLogDir := filepath.Join(constants.S6LogBaseDir, serviceName)
			currentFile := filepath.Join(actualLogDir, "current")
			logContent := "2025-01-20 10:00:00.000000000  initial message\n"
			err := fsService.WriteFile(ctx, currentFile, []byte(logContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			entries, err := service.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("initial message"))
		})

		It("should handle subsequent calls without rotation", func() {
			// Create initial log file in the actual log directory that GetLogs will check
			actualLogDir := filepath.Join(constants.S6LogBaseDir, serviceName)
			currentFile := filepath.Join(actualLogDir, "current")
			initialContent := "2025-01-20 10:00:00.000000000  initial message\n"
			err := fsService.WriteFile(ctx, currentFile, []byte(initialContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			// First call
			entries, err := service.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))

			// Append more content by reading existing and writing combined
			existingContent, err := fsService.ReadFile(ctx, currentFile)
			Expect(err).ToNot(HaveOccurred())

			additionalContent := "2025-01-20 10:00:01.000000000  additional message\n"
			combinedContent := string(existingContent) + additionalContent
			err = fsService.WriteFile(ctx, currentFile, []byte(combinedContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Second call should only get the new content
			entries, err = service.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(2)) // Both messages in ring buffer
			Expect(entries[0].Content).To(Equal("initial message"))
			Expect(entries[1].Content).To(Equal("additional message"))
		})

		It("should gracefully handle missing rotated file on rotation", func() {
			// Create a current file in the actual log directory that GetLogs will check
			actualLogDir := filepath.Join(constants.S6LogBaseDir, serviceName)
			currentFile := filepath.Join(actualLogDir, "current")
			initialContent := "2025-01-20 10:00:00.000000000  initial message\n"
			err := fsService.WriteFile(ctx, currentFile, []byte(initialContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			// First call to establish state
			entries, err := service.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))

			// Simulate rotation by creating a new current file (different inode)
			// No rotated file exists, so should fallback gracefully
			newContent := "2025-01-20 10:00:02.000000000  after rotation\n"
			err = fsService.RemoveAll(ctx, currentFile)
			Expect(err).ToNot(HaveOccurred())
			err = fsService.WriteFile(ctx, currentFile, []byte(newContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Should handle missing rotated file gracefully and read current file
			entries, err = service.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("after rotation"))
		})

		It("should read and combine rotated file with current file", func() {
			// Create initial current file in the actual log directory that GetLogs will check
			actualLogDir := filepath.Join(constants.S6LogBaseDir, serviceName)
			currentFile := filepath.Join(actualLogDir, "current")
			initialContent := "2025-01-20 10:00:00.000000000  initial message\n"
			err := fsService.WriteFile(ctx, currentFile, []byte(initialContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			// First call to establish state
			entries, err := service.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))

			// Create a rotated file with some content that would have been read
			rotatedTime := time.Now().Add(-1 * time.Minute)
			rotatedFileName := tai64.FormatNano(rotatedTime) + ".s"
			rotatedFile := filepath.Join(actualLogDir, rotatedFileName)
			rotatedContent := "2025-01-20 10:00:01.000000000  rotated message\n"
			err = fsService.WriteFile(ctx, rotatedFile, []byte(rotatedContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Simulate rotation by creating new current file
			newContent := "2025-01-20 10:00:02.000000000  new current message\n"
			err = fsService.RemoveAll(ctx, currentFile)
			Expect(err).ToNot(HaveOccurred())
			err = fsService.WriteFile(ctx, currentFile, []byte(newContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Should find rotated file and combine with current
			entries, err = service.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(2))

			// Should be in chronological order: rotated content first, then current
			Expect(entries[0].Content).To(Equal("rotated message"))
			Expect(entries[1].Content).To(Equal("new current message"))
		})
	})
})
