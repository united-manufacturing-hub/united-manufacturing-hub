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

package process_manager

import (
	"context"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
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

// Service defines the interface for interacting with S6 services
type Service interface {
	// Create creates the service with specific configuration
	Create(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) error
	// Remove removes the service directory structure
	Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Start starts the service
	Start(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Stop stops the service
	Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Restart restarts the service
	Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Status gets the current status of the service
	Status(ctx context.Context, servicePath string, fsService filesystem.Service) (ServiceInfo, error)
	// ExitHistory gets the exit history of the service
	ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]ExitEvent, error)
	// ServiceExists checks if the service directory exists
	ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error)
	// GetConfig gets the actual service config from s6
	GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (s6serviceconfig.S6ServiceConfig, error)
	// CleanS6ServiceDirectory cleans the S6 service directory, removing non-standard services
	CleanS6ServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error
	// GetS6ConfigFile retrieves a config file for a service
	GetS6ConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error)
	// ForceRemove removes a service from the S6 manager
	ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// GetLogs gets the logs of the service
	GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]LogEntry, error)
	// EnsureSupervision checks if the supervise directory exists for a service and notifies
	// s6-svscan if it doesn't, to trigger supervision setup.
	// Returns true if supervise directory exists (ready for supervision), false otherwise.
	EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error)
}
