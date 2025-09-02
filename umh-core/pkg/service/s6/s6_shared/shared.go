package s6_shared

import (
	"time"
)

// ServiceStatus represents the status of an S6 service.
type ServiceStatus string

const (
	// ServiceUnknown indicates the service status cannot be determined.
	ServiceUnknown ServiceStatus = "unknown"
	// ServiceUp indicates the service is running.
	ServiceUp ServiceStatus = "up"
	// ServiceDown indicates the service is stopped.
	ServiceDown ServiceStatus = "down"
	// ServiceRestarting indicates the service is restarting.
	ServiceRestarting ServiceStatus = "restarting"
)


// ServiceInfo contains information about an S6 service.
type ServiceInfo struct {
	LastChangedAt      time.Time     // Timestamp when the service status last changed
	LastReadyAt        time.Time     // Timestamp when the service was last ready
	LastDeploymentTime time.Time     // Timestamp when the service config last changed
	Status             ServiceStatus // Current status of the service
	ExitHistory        []ExitEvent   // History of exit codes
	Uptime             int64         // Seconds the service has been up
	DownTime           int64         // Seconds the service has been down
	ReadyTime          int64         // Seconds the service has been ready
	Pid                int           // Process ID if service is up
	Pgid               int           // Process group ID if service is up
	ExitCode           int           // Exit code if service is down
	WantUp             bool          // Whether the service wants to be up (based on existence of down file)
	IsPaused           bool          // Whether the service is paused
	IsFinishing        bool          // Whether the service is shutting down
	IsWantingUp        bool          // Whether the service wants to be up (based on flags)
	IsReady            bool          // Whether the service is ready
}

// ExitEvent represents a service exit event.
type ExitEvent struct {
	Timestamp time.Time // timestamp of the exit event
	ExitCode  int       // exit code of the service
	Signal    int       // signal number of the exit event
}

// LogEntry represents a parsed log entry from the S6 logs.
type LogEntry struct {
	// Timestamp in UTC time
	Timestamp time.Time `json:"timestamp"`
	// Content of the log entry
	Content string `json:"content"`
}
