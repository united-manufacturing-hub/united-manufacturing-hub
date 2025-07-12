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
	// Nmap Operation Timeouts - Level 1 Service (depends on S6)
	// NmapUpdateObservedStateTimeout is the timeout for updating the observed state
	NmapUpdateObservedStateTimeout = 10 * time.Millisecond

	// NmapProcessMetricsTimeout is the timeout for processing metrics
	NmapProcessMetricsTimeout = 5 * time.Millisecond // needs to be smaller than NmapUpdateObservedStateTimeout

	// NmapReportTimeout is the timeout for generating the report
	NmapReportTimeout = 10 * time.Second

	// NmapScanTimeout is the timeout for the scan
	NmapScanTimeout = 10 * time.Second
)
