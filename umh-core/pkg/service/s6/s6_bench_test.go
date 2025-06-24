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
	"bufio"
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cactus/tai64"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// S6 Log Rotation Performance Benchmarks
// ======================================
//
// This file contains benchmarks comparing different approaches for finding the latest
// rotated log file in S6's TAI64N timestamp-based naming scheme.
//
// BENCHMARK RESULTS (Apple M3 Pro, 2025-01-24):
// =============================================
//
// Two approaches were compared:
// 1. **Timestamp Parsing**: Parse TAI64N timestamps and find the latest chronologically
// 2. **Lexicographic Sorting**: Sort filenames lexicographically (TAI64N is designed for this)
//
// Results Summary:
// ---------------
// | File Count | Timestamp Parsing | Lexicographic Sort | Winner | Performance Gain |
// |------------|-------------------|-------------------|--------|------------------|
// | 1 file     | 19,738 ns/op     | 19,684 ns/op      | Lex    | +0.3%           |
// | 5 files    | 23,758 ns/op     | 23,462 ns/op      | Lex    | +1.2%           |
// | 10 files   | 29,096 ns/op     | 28,699 ns/op      | Lex    | +1.4%           |
// | 20 files   | 39,263 ns/op     | 53,672 ns/op      | Parse  | -36.7%          |
// | 50 files   | 64,360 ns/op     | 62,304 ns/op      | Lex    | +3.2%           |
// | 100 files  | 113,206 ns/op    | 110,608 ns/op     | Lex    | +2.3%           |
// | Realistic  | 35,449 ns/op     | 33,914 ns/op      | Lex    | +4.3%           |
// | (15 files) |                  |                   |        |                 |
//
// Key Findings:
// ------------
// - **Lexicographic sorting wins in most scenarios** (except one outlier at 20 files)
// - **4.3% performance improvement** for realistic workloads (15 rotated files)
// - **Simpler implementation** - leverages TAI64N's designed lexicographic sortability
// - **Better maintainability** - no timestamp parsing complexity or error handling
// - **Consistent memory usage** - similar allocations between approaches
//
// DECISION: Use lexicographic sorting with slices.Sort()
// =====================================================
//
// Rationale:
// 1. **Performance**: Generally faster, especially for realistic file counts
// 2. **Simplicity**: TAI64N timestamps are explicitly designed to be lexicographically sortable
// 3. **Reliability**: No parsing errors to handle - filenames either sort correctly or are ignored
// 4. **Go optimization**: slices.Sort() uses pattern-defeating quicksort, highly optimized
//
// Alternative Approaches Considered:
// ---------------------------------
// - **Radix Sort**: Theoretical O(n) performance, but 24-pass overhead made it slower for
//   realistic file counts (15-50 files). Lexicographic sorting was 35% faster.
// - **Inode-based tracking**: More complex state management, abandoned for simpler timestamp approach
//
// Test Environment:
// ----------------
// - CPU: Apple M3 Pro
// - Go version: 1.21+
// - Date: January 2025
// - Realistic scenario: 15 rotated files (typical S6 log rotation pattern)
//

// parseLogLineOptimized is an optimized version of parseLogLine that:
// 1. Avoids allocations for simple cases
// 2. Uses a more efficient string scanning strategy
// 3. Checks for minimum length before trying to parse
func parseLogLineOptimized(line string) LogEntry {
	// Quick check for empty strings or too short lines
	if len(line) < 29 { // Minimum length for "YYYY-MM-DD HH:MM:SS  content"
		return LogEntry{Content: line}
	}

	// Check if we have the double space separator
	sepIdx := strings.Index(line, "  ")
	if sepIdx == -1 || sepIdx > 28 {
		return LogEntry{Content: line}
	}

	// Extract timestamp part
	timestampStr := line[:sepIdx]

	// Extract content part (after the double space)
	content := ""
	if sepIdx+2 < len(line) {
		content = line[sepIdx+2:]
	}

	// Try to parse the timestamp
	// We are using ParseNano over time.Parse because it is faster for our specific time format
	timestamp, err := ParseNano(timestampStr)
	if err != nil {
		return LogEntry{Content: line}
	}

	return LogEntry{
		Timestamp: timestamp,
		Content:   content,
	}
}

// parseLogLine parses a log line from S6 format and returns a LogEntry
func parseLogLineOriginal(line string) LogEntry {
	// S6 log format with T flag: YYYY-MM-DD HH:MM:SS.NNNNNNNNN  content
	parts := strings.SplitN(line, "  ", 2)
	if len(parts) != 2 {
		return LogEntry{Content: line}
	}

	// We are using ParseNano over time.Parse because it is faster for our specific time format
	timestamp, err := ParseNano(parts[0])
	if err != nil {
		return LogEntry{Content: line}
	}

	return LogEntry{
		Timestamp: timestamp,
		Content:   parts[1],
	}
}

// findLatestRotatedFileByParsing is the old implementation that parses TAI64N timestamps (for comparison)
func findLatestRotatedFileByParsing(entries []string) string {
	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		// Extract just the filename part for parsing
		filename := filepath.Base(entry)
		// Remove the .s extension to get just the TAI64N timestamp
		if strings.HasSuffix(filename, ".s") {
			timestamp := strings.TrimSuffix(filename, ".s")
			parsedTime, err := tai64.Parse(timestamp)
			if err != nil {
				continue
			}

			if parsedTime.After(latestTime) {
				latestTime = parsedTime
				latestFile = entry
			}
		}
	}

	return latestFile
}

// findLatestRotatedFileByMaxFunc is the implementation that uses slices.MaxFunc
func findLatestRotatedFileByMaxFunc(entries []string) string {
	if len(entries) == 0 {
		return ""
	}

	// Use slices.MaxFunc to find the latest file
	latestFile := slices.MaxFunc(entries, cmp.Compare[string])
	return latestFile
}

// findLatestRotatedFileBySort is the implementation that uses slices.Sort
func findLatestRotatedFile(entries []string) string {
	if len(entries) == 0 {
		return ""
	}

	slices.Sort(entries)

	return entries[len(entries)-1]
}

// Now we use the main implementation from s6.go via the service instance

func BenchmarkParseLogLine(b *testing.B) {
	// Test cases with different log line formats
	testCases := []struct {
		name     string
		logLine  string
		expected LogEntry
	}{
		{
			name:    "Valid log line",
			logLine: "2023-01-02 15:04:05.123456789  This is a valid log entry",
			expected: LogEntry{
				Timestamp: time.Date(2023, 1, 2, 15, 4, 5, 123456789, time.UTC),
				Content:   "This is a valid log entry",
			},
		},
		{
			name:    "Invalid timestamp format",
			logLine: "2023/01/02 15:04:05  Invalid timestamp format",
			expected: LogEntry{
				Content: "2023/01/02 15:04:05  Invalid timestamp format",
			},
		},
		{
			name:    "No timestamp",
			logLine: "Just a log message without timestamp",
			expected: LogEntry{
				Content: "Just a log message without timestamp",
			},
		},
		{
			name:    "Empty string",
			logLine: "",
			expected: LogEntry{
				Content: "",
			},
		},
		{
			name:    "Missing content",
			logLine: "2023-01-02 15:04:05.123456789  ",
			expected: LogEntry{
				Timestamp: time.Date(2023, 1, 2, 15, 4, 5, 123456789, time.UTC),
				Content:   "",
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				entry := parseLogLineOriginal(tc.logLine)
				// Prevent compiler optimizations by using the result
				if entry.Content == "" && tc.name != "Empty string" && tc.name != "Missing content" {
					b.Fatalf("Got empty content for: %s", tc.name)
				}
			}
		})
	}
}

// BenchmarkParseLogLineOptimized benchmarks the optimized implementation
func BenchmarkParseLogLineOptimized(b *testing.B) {
	// Test cases with different log line formats
	testCases := []struct {
		name     string
		logLine  string
		expected LogEntry
	}{
		{
			name:    "Valid log line",
			logLine: "2023-01-02 15:04:05.123456789  This is a valid log entry",
			expected: LogEntry{
				Timestamp: time.Date(2023, 1, 2, 15, 4, 5, 123456789, time.UTC),
				Content:   "This is a valid log entry",
			},
		},
		{
			name:    "Invalid timestamp format",
			logLine: "2023/01/02 15:04:05  Invalid timestamp format",
			expected: LogEntry{
				Content: "2023/01/02 15:04:05  Invalid timestamp format",
			},
		},
		{
			name:    "No timestamp",
			logLine: "Just a log message without timestamp",
			expected: LogEntry{
				Content: "Just a log message without timestamp",
			},
		},
		{
			name:    "Empty string",
			logLine: "",
			expected: LogEntry{
				Content: "",
			},
		},
		{
			name:    "Missing content",
			logLine: "2023-01-02 15:04:05.123456789  ",
			expected: LogEntry{
				Timestamp: time.Date(2023, 1, 2, 15, 4, 5, 123456789, time.UTC),
				Content:   "",
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				entry := parseLogLineOptimized(tc.logLine)
				// Prevent compiler optimizations by using the result
				if entry.Content == "" && tc.name != "Empty string" && tc.name != "Missing content" {
					b.Fatalf("Got empty content for: %s", tc.name)
				}
			}
		})
	}
}

// BenchmarkParseLogLineCombined benchmarks the function with all test cases in a single run
func BenchmarkParseLogLineCombined(b *testing.B) {
	logLines := []string{
		"2023-01-02 15:04:05.123456789  This is a valid log entry",
		"2023/01/02 15:04:05  Invalid timestamp format",
		"Just a log message without timestamp",
		"",
		"2023-01-02 15:04:05.123456789  ",
		"2023-01-02 15:04:05.000000000  Log with zero nanoseconds",
		"2023-01-02 15:04:05  Log with no nanoseconds",
		"2023-01-02 15:04:05.123456789  Log with multiple  spaces  in content",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range logLines {
			_ = parseLogLineOriginal(line)
		}
	}
}

// BenchmarkParseLogLineCombinedOptimized benchmarks the optimized function with all test cases
func BenchmarkParseLogLineCombinedOptimized(b *testing.B) {
	logLines := []string{
		"2023-01-02 15:04:05.123456789  This is a valid log entry",
		"2023/01/02 15:04:05  Invalid timestamp format",
		"Just a log message without timestamp",
		"",
		"2023-01-02 15:04:05.123456789  ",
		"2023-01-02 15:04:05.000000000  Log with zero nanoseconds",
		"2023-01-02 15:04:05  Log with no nanoseconds",
		"2023-01-02 15:04:05.123456789  Log with multiple  spaces  in content",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range logLines {
			_ = parseLogLineOptimized(line)
		}
	}
}

// BenchmarkParseRealLogData benchmarks parsing real log data from the test file
func BenchmarkParseRealLogData(b *testing.B) {
	lines, err := readLogLines("s6_test_data.txt")
	if err != nil {
		b.Fatalf("Failed to read test data: %v", err)
	}

	if len(lines) == 0 {
		b.Fatal("No lines read from test data file")
	}

	b.Logf("Read %d lines from test data file", len(lines))

	b.Run("Original", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Use i % len(lines) to cycle through all lines
			idx := i % len(lines)
			_ = parseLogLineOriginal(lines[idx])
		}
	})

	b.Run("Optimized", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Use i % len(lines) to cycle through all lines
			idx := i % len(lines)
			_ = parseLogLineOptimized(lines[idx])
		}
	})

	// Also benchmark processing 100 lines in sequence to simulate batch processing
	if len(lines) >= 100 {
		testLines := lines[:100]

		b.Run("Original_Batch100", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				for _, line := range testLines {
					_ = parseLogLineOriginal(line)
				}
			}
		})

		b.Run("Optimized_Batch100", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				for _, line := range testLines {
					_ = parseLogLineOptimized(line)
				}
			}
		})
	}
}

// BenchmarkFindLatestRotatedFile benchmarks different approaches to finding the latest rotated file
func BenchmarkFindLatestRotatedFile(b *testing.B) {
	// Test with different numbers of rotated files
	fileCounts := []int{1, 5, 10, 20, 50, 100}

	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("Files_%d", fileCount), func(b *testing.B) {
			// Create temporary directory and files
			tempDir := b.TempDir()
			logDir := filepath.Join(tempDir, "logs")
			err := os.MkdirAll(logDir, 0755)
			if err != nil {
				b.Fatalf("Failed to create log directory: %v", err)
			}

			fsService := filesystem.NewDefaultService()
			ctx := context.Background()

			// Create rotated files with incrementing timestamps
			baseTime := time.Now().Add(-1 * time.Hour)
			var expectedLatest string

			for i := 0; i < fileCount; i++ {
				timestamp := baseTime.Add(time.Duration(i) * time.Minute)
				filename := tai64.FormatNano(timestamp) + ".s"
				filepath := filepath.Join(logDir, filename)

				err := os.WriteFile(filepath, []byte(fmt.Sprintf("log content %d", i)), 0644)
				if err != nil {
					b.Fatalf("Failed to create test file: %v", err)
				}

				if i == fileCount-1 {
					expectedLatest = filepath
				}
			}

			pattern := filepath.Join(logDir, "@*.s")
			entries, err := fsService.Glob(ctx, pattern)
			if err != nil {
				b.Fatalf("Failed to read log directory %s: %v", logDir, err)
			}

			// Benchmark timestamp parsing approach
			b.Run("TimestampParsing", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					result := findLatestRotatedFileByParsing(entries)
					if result != expectedLatest {
						b.Fatalf("Expected %s, got %s", expectedLatest, result)
					}
				}
			})

			// Benchmark lexicographic sorting approach
			b.Run("LexicographicSorting", func(b *testing.B) {
				service := NewDefaultService().(*DefaultService)
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					result := service.findLatestRotatedFile(entries)
					if result != expectedLatest {
						b.Fatalf("Expected %s, got %s", expectedLatest, result)
					}
				}
			})

			b.Run("MaxFunc", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					result := findLatestRotatedFileByMaxFunc(entries)
					if result != expectedLatest {
						b.Fatalf("Expected %s, got %s", expectedLatest, result)
					}
				}
			})
		})
	}
}

// BenchmarkFindLatestRotatedFileRealistic benchmarks with realistic file patterns
func BenchmarkFindLatestRotatedFileRealistic(b *testing.B) {
	// Create temporary directory
	tempDir := b.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		b.Fatalf("Failed to create log directory: %v", err)
	}

	fsService := filesystem.NewDefaultService()
	ctx := context.Background()

	// Create a realistic scenario with rotated files (@timestamp.s)
	// In practice, only current, lock, state and rotated TAI64N files exist

	baseTime := time.Now().Add(-2 * time.Hour)
	var expectedLatest string

	// Create 15 rotated files over 2 hours
	for i := 0; i < 15; i++ {
		timestamp := baseTime.Add(time.Duration(i*8) * time.Minute) // Every 8 minutes
		filename := tai64.FormatNano(timestamp) + ".s"
		filepath := filepath.Join(logDir, filename)

		err := os.WriteFile(filepath, []byte(fmt.Sprintf("rotated log %d", i)), 0644)
		if err != nil {
			b.Fatalf("Failed to create rotated file: %v", err)
		}

		if i == 14 {
			expectedLatest = filepath
		}
	}

	// Create the standard S6 files that exist but should be ignored by glob
	standardFiles := []string{"current", "lock", "state"}
	for _, filename := range standardFiles {
		filepath := filepath.Join(logDir, filename)
		err := os.WriteFile(filepath, []byte("standard s6 file content"), 0644)
		if err != nil {
			b.Fatalf("Failed to create standard file: %v", err)
		}
	}

	pattern := filepath.Join(logDir, "@*.s")
	entries, err := fsService.Glob(ctx, pattern)
	if err != nil {
		b.Fatalf("Failed to read log directory %s: %v", logDir, err)
	}

	b.Run("TimestampParsing_Realistic", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			result := findLatestRotatedFileByParsing(entries)
			if result != expectedLatest {
				b.Fatalf("Expected %s, got %s", expectedLatest, result)
			}
		}
	})

	b.Run("LexicographicSorting_Realistic", func(b *testing.B) {
		service := NewDefaultService().(*DefaultService)
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			result := service.findLatestRotatedFile(entries)
			if result != expectedLatest {
				b.Fatalf("Expected %s, got %s", expectedLatest, result)
			}
		}
	})

}

// readLogLines reads lines from the test data file
func readLogLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = closeErr
		}
	}()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}
