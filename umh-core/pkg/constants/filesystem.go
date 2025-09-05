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
	// FilesystemSlowReadThreshold defines when a file read operation is considered slow
	// and should be logged for debugging purposes.
	FilesystemSlowReadThreshold = time.Millisecond * 5

	// FilesystemWorkerMultiplier is used to determine the number of worker goroutines
	// for file operations based on CPU count. Since file reading is I/O bound,
	// having more workers than CPU cores can improve throughput.
	FilesystemWorkerMultiplier = 4

	// FilesystemMinWorkers sets a minimum floor for worker count regardless of CPU count.
	FilesystemMinWorkers = 4

	// FilesystemMaxWorkers caps the maximum number of workers to prevent excessive goroutines.
	FilesystemMaxWorkers = 32

	// FilesystemCacheTTL defines how long cached filesystem entries remain valid
	// before requiring revalidation. Short TTL balances freshness vs performance.
	FilesystemCacheTTL = time.Second

	// FilesystemCacheRecheckInterval defines how often to recheck file stats
	// for cached file content to detect external modifications.
	FilesystemCacheRecheckInterval = time.Second
)

// FilesystemCachePrefixes defines which path prefixes should have caching enabled.
// These are high-frequency read paths that benefit from caching.
var FilesystemCachePrefixes = []string{
	"/run/service/", // S6 service definitions
	"/data/",        // Data and configuration files
}
