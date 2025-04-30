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
	"os"
	"strings"
	"testing"
	"time"
)

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
	if sepIdx == -1 || sepIdx > 29 {
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
	timestamp, err := time.Parse("2006-01-02 15:04:05.999999999", timestampStr)
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

	timestamp, err := time.Parse("2006-01-02 15:04:05.999999999", parts[0])
	if err != nil {
		return LogEntry{Content: line}
	}

	return LogEntry{
		Timestamp: timestamp,
		Content:   parts[1],
	}
}

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
				// Ensure that expected equals result
				if entry != tc.expected {
					b.Fatalf("Expected %v, got %v", tc.expected, entry)
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
				// Ensure that expected equals result
				if entry != tc.expected {
					b.Fatalf("Expected %v, got %v", tc.expected, entry)
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
			le := parseLogLineOriginal(lines[idx])
			// Ensure that the result does not have an leading space
			if le.Content != "" && le.Content[0] == ' ' {
				b.Fatalf("Leading space found in content: %s", le.Content)
			}
		}
	})

	b.Run("Optimized", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Use i % len(lines) to cycle through all lines
			idx := i % len(lines)
			le := parseLogLineOptimized(lines[idx])
			// Ensure that the result does not have an leading space
			if le.Content != "" && le.Content[0] == ' ' {
				b.Fatalf("Leading space found in content: %s", le.Content)
			}
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
