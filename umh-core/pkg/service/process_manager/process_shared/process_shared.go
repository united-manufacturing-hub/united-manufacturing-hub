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

package process_shared

import (
	"bytes"
	"strings"
	"time"
)

// ServiceStatus represents the status of an S6 service
type ServiceStatus string

const (
	// ServiceUnknown indicates the service status cannot be determined
	ServiceUnknown ServiceStatus = "unknown"
	// ServiceUp indicates the service is running
	ServiceUp ServiceStatus = "up"
	// ServiceDown indicates the service is stopped
	ServiceDown ServiceStatus = "down"
	// ServiceRestarting indicates the service is restarting
	ServiceRestarting ServiceStatus = "restarting"
)

// HealthStatus represents the health state of an S6 service
type HealthStatus int

const (
	// HealthUnknown indicates the health check failed due to I/O errors, timeouts, etc.
	// This should not trigger service recreation, just retry next tick
	HealthUnknown HealthStatus = iota
	// HealthOK indicates the service directory is healthy and complete
	HealthOK
	// HealthBad indicates the service directory is definitely broken and needs recreation
	HealthBad
)

// String returns a string representation of the health status
func (h HealthStatus) String() string {
	switch h {
	case HealthUnknown:
		return "unknown"
	case HealthOK:
		return "ok"
	case HealthBad:
		return "bad"
	default:
		return "invalid"
	}
}

// ServiceInfo contains information about an S6 service
type ServiceInfo struct {
	Status        ServiceStatus // Current status of the service
	Uptime        int64         // Seconds the service has been up
	DownTime      int64         // Seconds the service has been down
	ReadyTime     int64         // Seconds the service has been ready
	Pid           int           // Process ID if service is up
	Pgid          int           // Process group ID if service is up
	ExitCode      int           // Exit code if service is down
	WantUp        bool          // Whether the service wants to be up (based on existence of down file)
	IsPaused      bool          // Whether the service is paused
	IsFinishing   bool          // Whether the service is shutting down
	IsWantingUp   bool          // Whether the service wants to be up (based on flags)
	IsReady       bool          // Whether the service is ready
	ExitHistory   []ExitEvent   // History of exit codes
	LastChangedAt time.Time     // Timestamp when the service status last changed
	LastReadyAt   time.Time     // Timestamp when the service was last ready
}

// ExitEvent represents a service exit event
type ExitEvent struct {
	Timestamp time.Time // timestamp of the exit event
	ExitCode  int       // exit code of the service
	Signal    int       // signal number of the exit event
}

// LogEntry represents a parsed log entry from the S6 logs
type LogEntry struct {
	// Timestamp in UTC time
	Timestamp time.Time `json:"timestamp"`
	// Content of the log entry
	Content string `json:"content"`
}

// ParseLogsFromBytes is a zero-allocation* parser for an s6 "current"
// file.  It scans the buffer **once**, pre-allocates the result slice
// and never calls strings.Split/Index, so the costly
// runtime.growslice/strings.* nodes vanish from the profile.
//
//	*apart from the unavoidable string↔[]byte conversions needed for the
//	LogEntry struct – those are just header copies, no heap memcopy.
func ParseLogsFromBytes(buf []byte) ([]LogEntry, error) {
	// Trim one trailing newline that is always present in rotated logs.
	buf = bytes.TrimSuffix(buf, []byte{'\n'})

	// 1) -------- pre-allocation --------------------------------------
	nLines := bytes.Count(buf, []byte{'\n'}) + 1
	entries := make([]LogEntry, 0, nLines) // avoids  runtime.growslice

	// 2) -------- single pass over the buffer -------------------------
	for start := 0; start < len(buf); {
		// find next '\n'
		nl := bytes.IndexByte(buf[start:], '\n')
		var line []byte
		if nl == -1 {
			line = buf[start:]
			start = len(buf)
		} else {
			line = buf[start : start+nl]
			start += nl + 1
		}
		if len(line) == 0 { // empty line – rotate artefact
			continue
		}

		// 3) -------- parse one line ----------------------------------
		// format: 2025-04-20 13:01:02.123456789␠␠payload
		sep := bytes.Index(line, []byte("  "))
		if sep == -1 || sep < 29 { // malformed – keep raw
			entries = append(entries, LogEntry{Content: string(line)})
			continue
		}

		ts, err := ParseNano(string(line[:sep])) // ParseNano is already fast
		if err != nil {
			entries = append(entries, LogEntry{Content: string(line)})
			continue
		}

		entries = append(entries, LogEntry{
			Timestamp: ts,
			Content:   string(line[sep+2:]),
		})
	}
	return entries, nil
}

// ParseLogsFromBytes_Unoptimized is the more readable not optimized version of ParseLogsFromBytes
func ParseLogsFromBytes_Unoptimized(content []byte) ([]LogEntry, error) {
	// Split logs by newline
	logs := strings.Split(strings.TrimSpace(string(content)), "\n")

	// Parse each log line into structured entries
	var entries []LogEntry
	for _, line := range logs {
		if line == "" {
			continue
		}

		entry := parseLogLine(line)
		if !entry.Timestamp.IsZero() {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// parseLogLine parses a log line from S6 format and returns a LogEntry
func parseLogLine(line string) LogEntry {
	// Quick check for empty strings or too short lines
	if len(line) < 28 { // Minimum length for "YYYY-MM-DD HH:MM:SS.<9 digit nanoseconds>  content"
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
