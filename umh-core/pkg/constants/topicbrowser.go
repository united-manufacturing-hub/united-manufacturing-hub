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
	TopicBrowserServiceName = "topic-browser"
)

const (
	// TopicBrowser Operation Timeouts - Level 2 Service (depends on Redpanda)
	// TopicBrowserUpdateObservedStateTimeout is the timeout for updating the observed state
	TopicBrowserUpdateObservedStateTimeout = 10 * time.Millisecond
)

const (
	// BLOCK_START_MARKER marks the begin of a new data/general block inside the logs.
	BLOCK_START_MARKER = "STARTSTARTSTART"
	// DATA_END_MARKER marks the end of a data block inside the logs.
	DATA_END_MARKER = "ENDDATAENDDATAENDDATA"
	// between DATA_END_MARKER AND BLOCK_END_MARKER sits the timestamp.

	// BLOCK_END_MARKER marks the end of a general block inside the logs.
	BLOCK_END_MARKER = "ENDENDEND"
)
