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

// Nmap monitor constants
const (
	// NmapUpdateObservedStateTimeout is the timeout for updating the observed state
	NmapUpdateObservedStateTimeout = 5 * time.Millisecond

	// Health assessment thresholds
	// NmapScanTimeout is the timeout for a scan to be considered healthy
	// If this is exceeded it means the last result is too old and we should not use it
	NmapScanTimeout = 10 * time.Second

	// NmapExpectedMaxP95ExecutionTimePerInstance means that an instance will not reconcile if not 35ms are left
	// Note: in the intergation test, we defined an alerting threshold of 80% of the max ticker time, which is 100ms
	// So by setting this to 35 ms, we can ensure that an instance will never start if it triggers the alerting threshold
	NmapExpectedMaxP95ExecutionTimePerInstance = time.Millisecond * 35
)
