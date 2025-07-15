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
	// S6 Operation Timeouts - Foundation Service (Level 0)
	// S6 is the foundation service with no dependencies
	S6UpdateObservedStateTimeout = 6 * time.Millisecond
	S6RemoveTimeout              = 10 * time.Millisecond
	S6MaxLines                   = 10000

	// S6FileReadTimeBuffer is the minimum time buffer required before attempting to read a file chunk
	// This is half of DefaultTickerTime to ensure graceful early exit from file operations
	// WHY HALF: Provides safety margin to complete current chunk + cleanup before context deadline
	// BUSINESS LOGIC: Prevents timeout failures by returning partial success instead of total failure
	S6FileReadTimeBuffer = time.Millisecond * 1

	// S6FileReadChunkSize is the buffer size used for reading files in chunks
	// Set to 1MB for optimal I/O performance while maintaining reasonable memory usage
	S6FileReadChunkSize = 1024 * 1024
)
