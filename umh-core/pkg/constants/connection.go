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
	// Connection Operation Timeouts - Level 2 Service (depends on Nmap)
	// ConnectionUpdateObservedStateTimeout is the timeout for updating the observed state
	ConnectionUpdateObservedStateTimeout = 20 * time.Millisecond

	// The amount of recent states to keep for flicker detection, if there were at least 2 states in the last 5 states that were differing, the connection is considered degraded
	MaxRecentStates = 5
)
