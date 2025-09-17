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

package constants

import (
	"os"
	"sync"
	"time"
)

const (
	S6BaseDir       = "/run/service" // Scan directory (contains symlinks)
	S6ConfigDirName = "config"
	S6LogBaseDir    = "/data/logs"
)

var (
	// s6RepositoryBaseDir holds the computed repository directory path
	s6RepositoryBaseDir string
	// s6RepositoryBaseDirOnce ensures the directory is computed only once
	s6RepositoryBaseDirOnce sync.Once
)

// GetS6RepositoryBaseDir returns the S6 repository directory path.
// By default, it uses /tmp/umh-core-services for fresh state on container restart.
// When S6_PERSIST_DIRECTORY environment variable is set to "true", it uses /data/services
// for persistent storage across restarts (useful for debugging).
// GetS6RepositoryBaseDir returns the repository base directory used for s6-managed services.
// 
// On first call it computes and caches the value for the process lifetime. If the environment
// variable S6_PERSIST_DIRECTORY is set to "true" the persistent path "/data/services" is used;
// otherwise the default temporary path "/tmp/umh-core-services" is returned.
func GetS6RepositoryBaseDir() string {
	s6RepositoryBaseDirOnce.Do(func() {
		if os.Getenv("S6_PERSIST_DIRECTORY") == "true" {
			s6RepositoryBaseDir = "/data/services" // Persistent directory for debugging
		} else {
			s6RepositoryBaseDir = "/tmp/umh-core-services" // Temporary directory (default)
		}
	})
	return s6RepositoryBaseDir
}

var (
	// Set by build process via ldflags using -ldflags="-X github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants.S6OverlayVersion=${S6_OVERLAY_VERSION}"
	// This injects the version at build time from the environment, eliminating the need for hard-coded values.
	S6OverlayVersion = "unknown"
)

const (
	// S6 Operation Timeouts - Foundation Service (Level 0)
	// S6 is the foundation service with no dependencies
	// Increased from 6ms to 15ms to handle scaling test load (benthos-scaling integration test).
	// Further increased to 20ms to handle production workloads with 25+ bridges.
	S6UpdateObservedStateTimeout = 20 * time.Millisecond
	// Increased from 10ms to 20ms to handle scaling test load.
	S6RemoveTimeout = 20 * time.Millisecond
	S6MaxLines      = 10000

	// S6FileReadTimeBuffer is the minimum time buffer required before attempting to read a file chunk
	// This is half of DefaultTickerTime to ensure graceful early exit from file operations
	// WHY HALF: Provides safety margin to complete current chunk + cleanup before context deadline
	// BUSINESS LOGIC: Prevents timeout failures by returning partial success instead of total failure.
	S6FileReadTimeBuffer = time.Millisecond * 1

	// S6FileReadChunkSize is the buffer size used for reading files in chunks
	// Set to 1MB for optimal I/O performance while maintaining reasonable memory usage.
	S6FileReadChunkSize = 1024 * 1024
	
	// S6DownAndReadyRestartDelay is the minimum time to wait before attempting to restart
	// a service that is in the S6 "down and ready" edge case state (PID=0 with flagready=1).
	// This delay prevents rapid restart loops when services transition quickly between states,
	// particularly during chaos testing or when stop->start happens in quick succession.
	// See ServiceInfo.IsDownAndReady in pkg/service/s6/s6.go for detailed explanation of this state.
	S6DownAndReadyRestartDelay = 3 * time.Second
)

const (
	// S6 Command Timeout Configuration
	// These constants control timeout behavior for S6 commands with -T and -t parameters.

	// S6_TIMEOUT_PERCENTAGE defines what percentage of remaining context time to allocate to S6 commands
	// Using 50% as recommended by skarnet documentation for safe retry behavior.
	S6_TIMEOUT_PERCENTAGE = 0.5
)
