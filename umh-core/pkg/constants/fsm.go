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

// FSM Context Timeouts
// These timeouts ensure critical operations can complete even under CPU throttling

// FSMTransitionTimeout is the timeout for FSM state transitions.
// Set to 200ms to survive CPU throttling via cgroups (which throttle for ~100ms periods).
// This prevents "previous transition did not complete" errors.
// See Linear ticket ENG-3419 for full context.
const FSMTransitionTimeout = 200 * time.Millisecond