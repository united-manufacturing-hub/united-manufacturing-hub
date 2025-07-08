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

import "time"

const (
	S6BaseDir       = "/run/service"
	S6ConfigDirName = "config"
	S6LogBaseDir    = "/data/logs"
)

var (
	// Set by build process via ldflags using -ldflags="-X github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants.S6OverlayVersion=${S6_OVERLAY_VERSION}"
	// This injects the version at build time from the environment, eliminating the need for hard-coded values.
	S6OverlayVersion = "unknown"
)

const (
	S6UpdateObservedStateTimeout = time.Millisecond * 3
	S6RemoveTimeout              = time.Millisecond * 3
	S6MaxLines                   = 10000

	// S6FileReadTimeBuffer is the minimum time buffer required before attempting to read a file chunk
	// This is half of DefaultTickerTime to ensure graceful early exit from file operations
	// WHY HALF: Provides safety margin to complete current chunk + cleanup before context deadline
	// BUSINESS LOGIC: Prevents timeout failures by returning partial success instead of total failure
	S6FileReadTimeBuffer = time.Millisecond * 1

	// S6FileReadChunkSize is the buffer size used for reading files in chunks
	// Set to 1MB for optimal I/O performance while maintaining memory efficiency
	// WHY 1MB: Balance between I/O throughput (fewer syscalls) and memory usage (bounded allocation)
	// PERFORMANCE: Large enough to amortize syscall overhead, small enough to avoid memory pressure
	S6FileReadChunkSize = 1024 * 1024
)

const (
	// S6ExpectedMaxP95ExecutionTimePerInstance reserves execution time for S6 service instance reconciliation.
	//
	// WHY: Ensures S6 instances don't start reconciliation without sufficient time to complete,
	// preventing partial state updates and timeout failures. S6 is the foundational service
	// manager, so its stability is critical for overall system reliability.
	//
	// BUSINESS LOGIC: S6 manages the lifecycle of all other services (Benthos, Redpanda, etc.).
	// If S6 reconciliation fails due to timeouts, dependent services may be left in
	// inconsistent states, potentially requiring manual intervention.
	//
	// FILE READ OPTIMIZATION CONTEXT: Works in conjunction with S6FileReadChunkSize (1MB)
	// and S6FileReadTimeBuffer (1ms) to optimize file I/O operations, enabling the
	// reduced execution time while maintaining reliability.
	S6ExpectedMaxP95ExecutionTimePerInstance = time.Millisecond * 25
)
