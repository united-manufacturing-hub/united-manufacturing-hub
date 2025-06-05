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
	// ProtocolConverterExpectedMaxP95ExecutionTimePerInstance means that an instance will not reconcile if not 50ms are left
	// Note: in the integration test, we defined an alerting threshold of 80% of the max ticker time, which is 100ms
	// So by setting this to 50 ms, we can ensure that an instance will never start if it triggers the alerting threshold

	ProtocolConverterExpectedMaxP95ExecutionTimePerInstance = time.Millisecond * 50 // needs to be higher than DataflowComponentExpectedMaxP95ExecutionTimePerInstance
)

const (
	// Used to set the context timeout for updating the observed state of a ProtocolConverter instance
	ProtocolConverterUpdateObservedStateTimeout = time.Millisecond * 5
)
