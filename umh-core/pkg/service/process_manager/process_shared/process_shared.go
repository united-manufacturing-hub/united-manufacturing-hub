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

import "time"

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
