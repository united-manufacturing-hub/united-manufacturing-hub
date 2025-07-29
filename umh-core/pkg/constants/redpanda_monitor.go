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

// RedpandaMonitor Operation Timeouts - Level 2 Service (depends on Redpanda)
// RedpandaMonitorUpdateObservedStateTimeout is the timeout for updating the observed state
const RedpandaMonitorUpdateObservedStateTimeout = 180 * time.Millisecond

const RedpandaMonitorProcessMetricsTimeout = 120 * time.Millisecond // needs to be smaller than RedpandaMonitorUpdateObservedStateTimeout
