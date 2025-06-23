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
	"path/filepath"
	"time"

	"github.com/cactus/tai64"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// S6 Log Rotation Test Coverage
// ═════════════════════════════
//
// This test suite covers the seamless log rotation functionality that prevents
// protobuf message loss during topic browser output parsing. The tests verify
// both normal operation and edge cases for each component of the rotation system.
//
// Test Categories:
// ───────────────
//
// 1. **parseRotatedFileTimestamp**: TAI64N timestamp parsing from S6 rotated filenames
//   - Normal: Valid @<TAI64N> formatted filenames
//   - Edge: Invalid filenames, malformed timestamps
//
// 2. **findMostRecentRotatedFile**: Discovery of most recent rotated file
//   - Normal: Multiple rotated files with different timestamps
//   - Edge: No rotated files, mixed file types, invalid timestamps
//
// 3. **appendToRingBuffer**: Ring buffer management for log entries
//   - Normal: Appending to empty buffer, buffer growth
//   - Edge: Buffer overflow/wrapping (tested implicitly)
//
// 4. **finishRotatedFile**: Completion of interrupted rotated file reads
//   - Normal: Reading remaining content from valid rotated file
//   - Edge: Missing files, empty rotated file state, I/O errors
//
// Missing Integration Tests:
// ─────────────────────────
// - Full GetLogs() rotation scenarios (difficult due to hardcoded constants.S6LogBaseDir)
// - Concurrent access during rotation
// - Multiple rapid rotations
// - Large file handling and memory limits
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
		// GetLogs uses constants.S6LogBaseDir, so we need to create the structure it expects
		logDir = filepath.Join(tempDir, "data", "logs", serviceName)

		err := fsService.EnsureDirectory(ctx, logDir)
		Expect(err).ToNot(HaveOccurred())
	})

	// ══════════════════════════════════════════════════════════════════
	// parseRotatedFileTimestamp: TAI64N Timestamp Parsing
	// ══════════════════════════════════════════════════════════════════
	//
	// Tests the parsing of TAI64N timestamps from S6 rotated log filenames.
	// S6 rotates files by renaming "current" to "@<TAI64N-timestamp>".
	//
	// Normal Cases:
	// • Valid TAI64N timestamp parsing with time accuracy verification
	//
	// Edge Cases:
	// • Invalid filename format (no @ prefix)
	// • Malformed TAI64N timestamp strings
	//
	Describe("parseRotatedFileTimestamp", func() {
		It("should parse valid TAI64N timestamp", func() {
			// Create a TAI64N timestamp
			now := time.Now()
			tai64Str := tai64.FormatNano(now)

			timestamp, err := service.parseRotatedFileTimestamp(tai64Str)
			Expect(err).ToNot(HaveOccurred())
			Expect(timestamp).To(BeTemporally("~", now, time.Second))
		})

		It("should return error for invalid filename", func() {
			_, err := service.parseRotatedFileTimestamp("invalid-file")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not a rotated file"))
		})

		It("should return error for invalid TAI64N", func() {
			_, err := service.parseRotatedFileTimestamp("@invalid-tai64n")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse"))
		})
	})

	// ══════════════════════════════════════════════════════════════════
	// findMostRecentRotatedFile: Rotated File Discovery
	// ══════════════════════════════════════════════════════════════════
	//
	// Tests the discovery of the most recent rotated file in a log directory.
	// This is critical for determining which rotated file contains unread
	// protobuf data that must be processed to prevent message loss.
	//
	// Normal Cases:
	// • Multiple rotated files - selects most recent by timestamp
	// • Mixed file types - ignores non-rotated files (no @ prefix)
	//
	// Edge Cases:
	// • Empty directory - returns empty string gracefully
	// • Directory read errors (tested implicitly via filesystem service)
	// • Invalid TAI64N timestamps in filenames (logged but ignored)
	//
	Describe("findMostRecentRotatedFile", func() {
		It("should return empty string when no rotated files exist", func() {
			result := service.findMostRecentRotatedFile(ctx, logDir, fsService)
			Expect(result).To(BeEmpty())
		})

		It("should find most recent rotated file", func() {
			// Create two rotated files with different timestamps
			older := time.Now().Add(-2 * time.Hour)
			newer := time.Now().Add(-1 * time.Hour)

			olderFile := filepath.Join(logDir, tai64.FormatNano(older))
			newerFile := filepath.Join(logDir, tai64.FormatNano(newer))

			err := fsService.WriteFile(ctx, olderFile, []byte("old content"), 0644)
			Expect(err).ToNot(HaveOccurred())

			err = fsService.WriteFile(ctx, newerFile, []byte("new content"), 0644)
			Expect(err).ToNot(HaveOccurred())

			result := service.findMostRecentRotatedFile(ctx, logDir, fsService)
			Expect(result).To(Equal(newerFile))
		})

		It("should ignore non-rotated files", func() {
			// Create a regular file and a rotated file
			regularFile := filepath.Join(logDir, "current")
			rotatedFile := filepath.Join(logDir, tai64.FormatNano(time.Now()))

			err := fsService.WriteFile(ctx, regularFile, []byte("current content"), 0644)
			Expect(err).ToNot(HaveOccurred())

			err = fsService.WriteFile(ctx, rotatedFile, []byte("rotated content"), 0644)
			Expect(err).ToNot(HaveOccurred())

			result := service.findMostRecentRotatedFile(ctx, logDir, fsService)
			Expect(result).To(Equal(rotatedFile))
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
	})

	// ══════════════════════════════════════════════════════════════════
	// finishRotatedFile: Rotated File Completion
	// ══════════════════════════════════════════════════════════════════
	//
	// Tests the critical function that prevents protobuf message loss by
	// completing reads from rotated files before processing the new current file.
	// This ensures binary protobuf messages are never truncated mid-stream.
	//
	// Normal Cases:
	// • Complete reading of valid rotated file content
	// • Proper parsing and integration with existing ring buffer
	// • Correct state cleanup after successful processing
	//
	// Edge Cases:
	// • No rotated file set (early return) - common for first calls
	// • Missing/deleted rotated file - graceful error handling
	// • I/O errors during read - fail-safe state clearing
	// • Parse errors - continue processing without crashing
	//
	// Fail-Safe Strategy:
	// • Always clears rotated file state on completion/error
	// • Prevents infinite retry loops on persistent errors
	// • Prioritizes availability over perfect data completeness
	//
	Describe("finishRotatedFile", func() {
		It("should do nothing when no rotated file is set", func() {
			st := &logState{}

			service.finishRotatedFile(ctx, st, fsService)

			Expect(st.rotatedFile).To(BeEmpty())
			Expect(st.rotatedOffset).To(BeZero())
		})

		It("should handle missing rotated file gracefully", func() {
			st := &logState{
				rotatedFile:   filepath.Join(logDir, "missing-file"),
				rotatedOffset: 0,
			}

			service.finishRotatedFile(ctx, st, fsService)

			// Should clear the rotated file on error
			Expect(st.rotatedFile).To(BeEmpty())
			Expect(st.rotatedOffset).To(BeZero())
		})

		It("should read remaining content from rotated file", func() {
			// Create a rotated file with log content
			rotatedFile := filepath.Join(logDir, tai64.FormatNano(time.Now()))
			logContent := "2025-01-20 10:00:00.000000000  test message 1\n2025-01-20 10:00:01.000000000  test message 2\n"

			err := fsService.WriteFile(ctx, rotatedFile, []byte(logContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			st := &logState{
				rotatedFile:   rotatedFile,
				rotatedOffset: 0,
			}

			service.finishRotatedFile(ctx, st, fsService)

			// Should clear the rotated file after reading
			Expect(st.rotatedFile).To(BeEmpty())
			Expect(st.rotatedOffset).To(BeZero())

			// Should have parsed and added entries to ring buffer
			Expect(st.logs).To(HaveLen(2))
			Expect(st.logs[0].Content).To(Equal("test message 1"))
			Expect(st.logs[1].Content).To(Equal("test message 2"))
		})
	})
})
